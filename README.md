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

## Архитектура

```
[Client] → [gRPC] → [Validator Service] → [Redis (cache)]
                                    ↓
                              [ClickHouse (storage)]
```

## Быстрый старт

### Требования

- Go 1.21+
- Docker & Docker Compose

### Запуск

```bash
# Клонировать репозиторий
git clone https://github.com/hzname/coordinate-validator.git
cd coordinate-validator

# Запустить Redis и ClickHouse
docker-compose up -d redis clickhouse

# Запустить приложение
go run ./cmd/server
```

### Docker

```bash
docker-compose up --build
```

## gRPC API

### Validate

Валидирует координаты одного устройства.

```protobuf
message CoordinateRequest {
  string device_id = 1;
  double latitude = 2;
  double longitude = 3;
  float accuracy = 4;
  int64 timestamp = 5;
  
  repeated WifiAccessPoint wifi = 6;
  repeated BluetoothDevice bluetooth = 7;
  CellTower cell_tower = 8;
}

message CoordinateResponse {
  ValidationResult result = 1;  // VALID, INVALID, UNCERTAIN
  float confidence = 2;          // 0.0 - 1.0
  float estimated_accuracy = 3;
  string reason = 4;
}
```

### Пример вызова

```bash
grpcurl -plaintext -d '{
  "device_id": "device123",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "accuracy": 10.0,
  "timestamp": 1700000000
}' localhost:50051 coordinate.CoordinateValidator/Validate
```

## Конфигурация

Переменные окружения:

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

- [Архитектура](docs/architecture.md) - Диаграммы архитектуры, flow валидации, структуры данных

## Разработка

```bash
# Генерировать protobuf код
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/coordinate.proto

# Запустить тесты
go test ./...

# Собрать бинарник
go build -o coordinate-validator ./cmd/server
```

## Производительность

- Пропускная способность: ~1000+ запросов/сек на одном ядре
- Задержка: <10ms (без обращения к Redis/ClickHouse)
- Память: ~50MB базовый объём

## Лицензия

MIT
