# Coordinate Validator 🚕

Microservice for GPS coordinate validation with hybrid microservices architecture.

## Features

- **Coordinate Validation** — Validate GPS coordinates using time, speed, and triangulation
- **Self-Learning** — Companion detection with adaptive confidence
- **High Performance** — ~1000+ RPS throughput
- **Microservices** — Gateway, Refinement API, Learning API, Storage Service
- **Async Storage** — ClickHouse + Kafka for analytics

## Architecture

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
        └──────────────────┬───────────────────────┘
                          │
        ┌─────────────────▼───────────────────────┐
        │     Storage Service (ClickHouse)        │
        │     + Event Producer (Kafka)             │
        └───────────────────────────────────────────┘
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| Gateway | 50050 | API Gateway, routes to Refinement/Learning |
| Refinement API | 50051 | Coordinate validation (time/speed/triangulation) |
| Learning API | 50052 | Companion detection, learning |
| Storage Service | 50053 | Async writes to ClickHouse + Kafka |

## Validation Logic

### Layer 1: Rule-based
- **Time Check** — Timestamp within 0-12 hours
- **Speed Check** — Max 150 km/h (Haversine distance / time)

### Layer 2: Triangulation
- **WiFi** — Confidence boost when BSSID known
- **Cell Towers** — Confidence boost when cell_id + LAC known
- **Bluetooth** — Confidence boost when MAC known

### Result
| Confidence | Result |
|------------|--------|
| ≥ 0.8 | VALID |
| 0.3 - 0.79 | UNCERTAIN |
| < 0.3 | INVALID |

## Quick Start

```bash
# Start all services with Docker
docker-compose up -d

# Or start infrastructure only
docker-compose up -d redis clickhouse kafka

# Start services manually
go run ./cmd/gateway
go run ./cmd/refinement-api
go run ./cmd/learning-api
go run ./cmd/storage-service
```

## Environment Variables

### Gateway
| Variable | Default | Description |
|----------|---------|-------------|
| SERVER_PORT | 50050 | Gateway port |
| REFINEMENT_ADDR | localhost:50051 | Refinement API address |
| LEARNING_ADDR | localhost:50052 | Learning API address |

### Refinement/Learning API
| Variable | Default | Description |
|----------|---------|-------------|
| REDIS_ADDR | localhost:6379 | Redis address |
| MAX_SPEED_KMH | 150 | Max speed (km/h) |
| MAX_TIME_DIFF | 12h | Max time deviation |

### Storage Service
| Variable | Default | Description |
|----------|---------|-------------|
| CLICKHOUSE_ADDR | localhost:9000 | ClickHouse address |
| KAFKA_BROKERS | localhost:9092 | Kafka brokers |
| CLICKHOUSE_BATCH_SIZE | 1000 | Batch size for writes |
| CLICKHOUSE_FLUSH_INTERVAL | 5s | Flush interval |

## gRPC API

### Validate (Refinement)
```bash
grpcurl -plaintext -d '{
  "device_id": "vehicle123",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "accuracy": 10.0,
  "timestamp": 1700000000,
  "wifi": [{"bssid": "AA:BB:CC:DD:EE:FF", "rssi": -50}],
  "cell_towers": [{"cell_id": 12345, "lac": 678, "mcc": 250, "mnc": 99, "rssi": -80}]
}' localhost:50050 coordinate.CoordinateValidator/Validate
```

### Learn (Learning)
```bash
grpcurl -plaintext -d '{
  "object_id": "device123",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "accuracy": 10.0,
  "timestamp": 1700000000
}' localhost:50050 coordinate.LearningService/LearnFromCoordinates
```

## Project Structure

```
cmd/
├── gateway/           # API Gateway
├── refinement-api/    # Validation service
├── learning-api/      # Learning service
└── storage-service/   # Async storage

internal/
├── cache/            # Redis client
├── config/           # Configuration
├── core/
│   ├── validation.go # Validation logic
│   └── learning.go   # Learning logic
├── model/            # Data models
├── queue/            # Kafka producer
└── storage/          # ClickHouse client

docs/
├── architecture.md    # Full architecture docs
├── learning-model.md # Learning algorithm
├── deployment-checklist.md
└── testing-checklist.md
```

## Documentation

- [Architecture](./docs/architecture.md) — Detailed architecture
- [Learning Model](./docs/learning-model.md) — Companion detection algorithm
- [Deployment](./docs/deployment-checklist.md) — Deployment guide
- [Testing](./docs/testing-checklist.md) — Testing checklist

## Tech Stack

- **Go** — Core services
- **gRPC** — API communication
- **Redis** — Cache
- **ClickHouse** — Analytics storage
- **Kafka** — Event streaming
- **Docker** — Containerization

## License

MIT
