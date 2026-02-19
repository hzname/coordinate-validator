# Coordinate Validator

Microservice for GPS coordinate validation using gRPC, Go, Redis and ClickHouse.

## Features

- Coordinate validation by speed (max 150 km/h)
- Timestamp validation (max 12 hours)
- Triangulation using WiFi, Cell towers, Bluetooth
- Self-learning with companion detection
- Asynchronous storage in ClickHouse
- EGTS protocol support (subrecords 91, 92)
- Two-stream model: refinement vs learning

## Two Data Streams

### 1. Refinement (Validation)
- Incoming data for validation
- Used ONLY for checking coordinates
- Does NOT participate in learning

### 2. Learning
- Separate data stream
- Only "companion" sources
- Updates CALCULATED coordinates

## EGTS Protocol

System supports EGTS (Egts Telematics) protocol data:

### Subrecord 91: EGTS_ENVELOPE_HIGHT
Cell tower data.

| Field | Description |
|-------|-------------|
| CID | Base station ID |
| LAC | Local Area Code |
| MCC | Mobile Country Code |
| MNC | Mobile Network Code |
| RSSI | Signal level (+128 offset) |
| EID | Inverted RSSI (*-1) |

### Subrecord 92: EGTS_ENVELOPE_LOW
WiFi / BLE data.

| Field | Description |
|-------|-------------|
| ENVTYPE | Type: 0 = WiFi, 1 = BLE |
| CID | MAC address |
| EID | Inverted signal level |

## Architecture

```
[Client] → [gRPC] → [Validator Service] → [Redis (cache)]
                                              ↓
                                        [ClickHouse (storage)]
```

## Quick Start

### Requirements

- Go 1.21+
- Docker & Docker Compose

### Run

```bash
git clone https://github.com/hzname/coordinate-validator.git
cd coordinate-validator
docker-compose up -d redis clickhouse
go run ./cmd/server
```

## gRPC API

### Validation (Refinement - NO learning)

```protobuf
rpc Validate(CoordinateRequest) returns (CoordinateResponse);
rpc ValidateBatch(stream CoordinateRequest) returns (stream CoordinateResponse);
```

### Learning (separate stream)

```protobuf
rpc LearnFromCoordinates(LearnRequest) returns (LearnResponse);
rpc GetCompanionSources(GetCompanionsRequest) returns (GetCompanionsResponse);
```

### Request/Response

```protobuf
message CoordinateRequest {
  string device_id = 1;
  double latitude = 2;
  double longitude = 3;
  float accuracy = 4;
  int64 timestamp = 5;
  
  // WiFi / BLE
  repeated WifiAccessPoint wifi = 6;
  repeated BluetoothDevice bluetooth = 7;
  
  // Cell towers
  repeated CellTower cell_towers = 8;
}

message CoordinateResponse {
  ValidationResult result = 1;   // VALID, INVALID, UNCERTAIN
  float confidence = 2;          // 0.0 - 1.0
  float estimated_accuracy = 3;
  string reason = 4;
}
```

### Example

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

## Configuration

| Variable | Description | Default |
|---------|-------------|---------|
| SERVER_PORT | gRPC server port | 50051 |
| REDIS_ADDR | Redis address | localhost:6379 |
| CLICKHOUSE_ADDR | ClickHouse address | localhost:9000 |
| CLICKHOUSE_DB | Database | coordinates |

## Validation Limits

- Max speed: 150 km/h
- Max time deviation: 12 hours
- Triangulation: intersection of source areas
- Companion detection: co-occurrence analysis

## Documentation

- [Architecture](docs/architecture.md) - System diagrams
- [Learning Model](docs/learning-model.md) - Two-stream model, companion filter

## Performance

- Throughput: ~1000+ RPS
- Latency: <10ms

## License

MIT
