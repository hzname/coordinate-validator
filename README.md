# Coordinate Validator

Микросервис для валидации GPS координат с использованием gRPC, Go, Redis и ClickHouse.

## Возможности

- Валидация координат по скорости перемещения (max 150 км/ч)
- Проверка временной метки (max 12 часов)
- Проверка по известным WiFi точкам
- Проверка по известным вышкам сотовой связи
- Проверка по известным Bluetooth устройствам
- Самообучение (запись новых точек доступа)
- Асинхронное сохранение в ClickHouse
- Поддержка EGTS протокола (подписки 91, 92)

## EGTS Протокол

Система поддерживает данные из EGTS (ЕГТС) протокола:

### Подписка 91: EGTS_ENVELOPE_HIGHT
Данные о видимых базовых станциях сотовой сети.

| Поле | Описание |
|------|----------|
| CID | ID базовой станции |
| LAC | Код локальной зоны |
| MCC | Код страны |
| MNC | Код оператора |
| RSSI | Уровень сигнала (+128 offset) |
| EID | Инвертированный RSSI (*-1) |

### Подписка 92: EGTS_ENVELOPE_LOW
Данные о видимых источниках WiFi / BLE.

| Поле | Описание |
|------|----------|
| ENVTYPE | Тип: 0 = WiFi, 1 = BLE |
| CID | MAC-адрес источника |
| EID | Инвертированный уровень сигнала |

## Архитектура

```
[EGTS Client] → [gRPC] → [Validator Service] → [Redis (cache)]
                                              ↓
                                        [ClickHouse (storage)]
```

## Быстрый старт

### Требования

- Go 1.21+
- Docker & Docker Compose

### Запуск

```bash
git clone https://github.com/hzname/coordinate-validator.git
cd coordinate-validator
docker-compose up -d redis clickhouse
go run ./cmd/server
```

## gRPC API

```protobuf
message CoordinateRequest {
  string device_id = 1;
  double latitude = 2;
  double longitude = 3;
  float accuracy = 4;
  int64 timestamp = 5;
  
  // EGTS_ENVELOPE_LOW (92)
  repeated WifiAccessPoint wifi = 6;
  repeated BluetoothDevice bluetooth = 7;
  
  // EGTS_ENVELOPE_HIGHT (91)
  repeated CellTower cell_towers = 8;
}

message CoordinateResponse {
  ValidationResult result = 1;   // VALID, INVALID, UNCERTAIN
  float confidence = 2;          // 0.0 - 1.0
  float estimated_accuracy = 3;
  string reason = 4;
}
```

### Пример вызова

```bash
grpcurl -plaintext -d '{
  "device_id": "vehicle123",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "accuracy": 10.0,
  "timestamp": 1700000000,
  "wifi": [{"bssid": "AA:BB:CC:DD:EE:FF", "ssid": "MyWiFi", "rssi": -50}],
  "cell_towers": [{"cell_id": 12345, "lac": 678, "mcc": 250, "mnc": 99, "rssi": -80}]
}' localhost:50051 coordinate.CoordinateValidator/Validate
```

## Конфигурация

| Переменная | Описание | По умолчанию |
|-----------|----------|--------------|
| SERVER_PORT | Порт gRPC сервера | 50051 |
| REDIS_ADDR | Адрес Redis | localhost:6379 |
| CLICKHOUSE_ADDR | Адрес ClickHouse | localhost:9000 |
| CLICKHOUSE_DB | База данных | coordinates |

## Лимиты валидации

- Максимальная скорость: 150 км/ч
- Максимальное время отклонения: 12 часов
- Веса источников данных:
  - WiFi: 0.4
  - Cell: 0.3
  - Bluetooth: 0.3

## Документация

- [Архитектура](docs/architecture.md) - Диаграммы

## Производительность

- Пропускная способность: ~1000+ RPS
- Задержка: <10ms

## Лицензия

MIT
