# Новая архитектура системы

## Общая схема (гибридная)

```
┌─────────────────────────────────────────────────────────┐
│                    API Gateway                          │
│              (gRPC Ingress + Rate Limit)               │
└─────────────────┬───────────────────────┬───────────────┘
                  │                       │
        ┌─────────▼─────────┐   ┌────────▼────────┐
        │  Refinement API   │   │   Learning API    │
        │  (Validate/Batch) │   │ (LearnFromCoords)│
        └─────────┬─────────┘   └────────┬─────────┘
                  │                       │
        ┌─────────▼─────────┐   ┌────────▼────────┐
        │  Validation Core  │   │  Learning Core   │
        │  (Time/Speed/Tri) │   │  (Companion det) │
        └─────────┬─────────┘   └────────┬─────────┘
                  │                       │
        ┌─────────▼──────────────────────▼────────┐
        │           Shared Cache (Redis)          │
        │    WiFi / Cell / BT / Device positions  │
        └──────────────────┬───────────────────────┘
                          │
        ┌─────────────────▼───────────────────────┐
        │     Storage Service (ClickHouse)        │
        │     + Event Producer (Kafka)             │
        └───────────────────────────────────────────┘
```

## Микросервисы

### 1. API Gateway
- Входная точка (gRPC)
- Rate limiting
- Роутинг запросов к нужному сервису

### 2. Refinement API
- Эндпоинты: `Validate`, `ValidateBatch`
- **НЕ участвует в обучении**
- Только валидация координат
- Вызывает Validation Core

### 3. Learning API
- Эндпоинты: `LearnFromCoordinates`, `GetCompanionSources`
- Обучение на "companion" источниках
- Обновляет координаты в кэше

### 4. Validation Core
- Проверка времени (max 12 часов)
- Проверка скорости (max 150 км/ч)
- Триангуляция (WiFi, Cell, BT)

### 5. Learning Core
- Определение companion устройств
- Анализ co-occurrence
- Обновление CALCULATED координат

### 6. Storage Service
- Асинхронная запись в ClickHouse
- Producer событий в Kafka

## Потоки данных

### Refinement (валидация)
```
Client → Gateway → Refinement API → Validation Core → Redis (read) → Response
                                    ↓
                              Storage Service (async write)
```

### Learning (обучение)
```
Client → Gateway → Learning API → Learning Core → Redis (write) → Response
                                   ↓
                             Storage Service (async write)
```

## Структура Redis (общая)

```
WIFI { bssid → lat, lon, last_seen }
CELL { cell_id + lac → lat, lon }
BT   { mac → lat, lon }
DEVICE { device_id → last_lat, last_lon, last_time }
```

## Разделение по потокам

| Поток | API | Операции | Обучение |
|-------|-----|----------|----------|
| Refinement | Validate, ValidateBatch | Только чтение из кэша | Нет |
| Learning | LearnFromCoordinates | Чтение + запись в кэш | Да |

## Преимущества гибридной архитектуры

1. **Изоляция потоков** — Refinement и Learning разделены (как в оригинале)
2. **Общий кэш** — не нужно синхронизировать данные между сервисами
3. **Асинхронный storage** — не блокирует API
4. **Масштабируемость** — можно реплицировать только горячий путь (валидация)
5. **Гибкость** — каждый микросервис можно развивать отдельно

## Deployment

```
                    ┌──────────────┐
                    │   Ingress   │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │    Gateway   │
                    └──────┬───────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
    ┌──────▼──────┐ ┌─────▼─────┐ ┌──────▼──────┐
    │ Refinement  │ │  Learning │ │  Storage    │
    │    API      │ │    API    │ │  Service    │
    └──────┬──────┘ └─────┬─────┘ └──────┬──────┘
           │               │               │
           └───────────────┼───────────────┘
                           │
                    ┌──────▼───────┐
                    │    Redis     │
                    │   (Cache)    │
                    └──────────────┘
```
