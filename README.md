# Coordinate Validator

Microservice for GPS coordinate validation with hybrid microservices architecture.

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

## Quick Start

```bash
# Start all services
docker-compose up -d

# Or with Redis and ClickHouse
docker-compose up -d redis clickhouse kafka
go run ./cmd/gateway
go run ./cmd/refinement-api
go run ./cmd/learning-api
go run ./cmd/storage-service
```

## Environment Variables

### Gateway
- `SERVER_PORT` - Gateway port (default: 50050)
- `REFINEMENT_ADDR` - Refinement API address
- `LEARNING_ADDR` - Learning API address

### Refinement/Learning API
- `REDIS_ADDR` - Redis address (default: localhost:6379)
- `MAX_SPEED_KMH` - Max speed in km/h (default: 150)
- `MAX_TIME_DIFF` - Max time difference (default: 12h)

### Storage Service
- `CLICKHOUSE_ADDR` - ClickHouse address (default: localhost:9000)
- `KAFKA_BROKERS` - Kafka brokers

## gRPC API

### Refinement API
```bash
grpcurl -plaintext -d '{
  "device_id": "vehicle123",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "accuracy": 10.0,
  "timestamp": 1700000000
}' localhost:50051 coordinate.CoordinateValidator/Validate
```

### Learning API
```bash
grpcurl -plaintext -d '{
  "object_id": "device123",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "accuracy": 10.0,
  "timestamp": 1700000000
}' localhost:50052 coordinate.LearningService/LearnFromCoordinates
```

## License

MIT
