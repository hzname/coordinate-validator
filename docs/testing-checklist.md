# Testing Checklist

## Unit Tests

### Config
- [ ] Load configuration from environment variables
- [ ] Validate parameter ranges
- [ ] Service-specific config (Gateway, Refinement, Learning, Storage)

### Cache (Redis)
- [ ] Connect to Redis
- [ ] Get/Set WiFi point
- [ ] Get/Set Cell tower
- [ ] Get/Set BLE device
- [ ] Get/Set device last location
- [ ] Get/Set companions
- [ ] TTL expiration
- [ ] Error handling (connection lost)

### Storage (ClickHouse)
- [ ] Connect to ClickHouse
- [ ] Create tables
- [ ] Batch insert coordinate records
- [ ] Query coordinate history
- [ ] Error handling

### Kafka Producer
- [ ] Connect to Kafka
- [ ] Send refinement event
- [ ] Send learning event
- [ ] Batch send
- [ ] Error handling (broker unavailable)

### Validation Core
- [ ] Time validation - future timestamp → INVALID
- [ ] Time validation - old timestamp (>12h) → INVALID
- [ ] Time validation - valid timestamp → OK
- [ ] Speed validation - no previous location → OK
- [ ] Speed validation - valid speed → OK
- [ ] Speed validation - impossible speed (>150km/h) → INVALID
- [ ] WiFi triangulation - known point → confidence boost
- [ ] WiFi triangulation - unknown point → no boost
- [ ] Cell triangulation - known tower → confidence boost
- [ ] Cell triangulation - unknown tower → no boost
- [ ] BLE triangulation - known device → confidence boost
- [ ] Confidence calculation
- [ ] UNCERTAIN result when confidence 0.3-0.79
- [ ] INVALID result when confidence < 0.3
- [ ] VALID result when confidence ≥ 0.8

### Learning Core
- [ ] New source - first observation
- [ ] Companion detection - co-occurrence
- [ ] Stationary update - weight 0.2
- [ ] Random update - weight 0.1
- [ ] Confidence calculation (logarithmic growth)
- [ ] Version increment on update

### Gateway Routing
- [ ] Route Validate to Refinement API
- [ ] Route ValidateBatch to Refinement API
- [ ] Route LearnFromCoordinates to Learning API
- [ ] Route GetCompanionSources to Learning API
- [ ] Error handling (service unavailable)

---

## Integration Tests

### Gateway API
- [ ] GET /health returns 200
- [ ] Validate forwards to Refinement API
- [ ] Learn forwards to Learning API
- [ ] Error propagation from backend services

### Refinement API (port 50051)
- [ ] Valid request returns VALID
- [ ] Invalid timestamp returns INVALID
- [ ] Invalid speed returns INVALID
- [ ] Unknown sources returns UNCERTAIN
- [ ] Batch validation works
- [ ] Device position updated after validation

### Learning API (port 50052)
- [ ] New source returns NEED_MORE_DATA
- [ ] Stationary source returns LEARNED
- [ ] Random source returns RANDOM_EXCLUDED
- [ ] Companions updated in Redis

### Storage Service (port 50053)
- [ ] Queue validation record
- [ ] Batch flush to ClickHouse
- [ ] Send Kafka event
- [ ] Async write doesn't block API

### End-to-End Flows
- [ ] Validate → Storage → ClickHouse
- [ ] Learn → Storage → ClickHouse + Kafka
- [ ] Full flow: Validate → Learn → Validate (improved confidence)

---

## Performance Tests

### Gateway
- [ ] 1000 RPS routing latency < 10ms
- [ ] 5000 RPS routing latency < 50ms

### Refinement API
- [ ] 100 RPS - latency < 50ms
- [ ] 500 RPS - latency < 100ms
- [ ] 1000 RPS - latency < 200ms

### Learning API
- [ ] 50 RPS - latency < 100ms
- [ ] 100 RPS - latency < 200ms

### Storage Service
- [ ] Batch 1000 records - flush < 1s
- [ ] Kafka produce < 10ms

### Resource Usage
- [ ] Memory < 200MB per service under load
- [ ] Redis connections < pool size
- [ ] No connection leaks

---

## Error Handling Tests

### Redis
- [ ] Connection lost → graceful degradation
- [ ] Timeout → retry with backoff

### ClickHouse
- [ ] Connection lost → queue writes
- [ ] Batch failure → retry

### Kafka
- [ ] Broker unavailable → buffer in memory
- [ ] Produce timeout → error returned

### gRPC
- [ ] Backend service unavailable → error to client
- [ ] Timeout → context deadline exceeded

---

## Microservices Tests

### Service Discovery
- [ ] Gateway finds Refinement API
- [ ] Gateway finds Learning API
- [ ] Services find Redis
- [ ] Storage finds ClickHouse
- [ ] Storage finds Kafka

### Circuit Breaker
- [ ] Refinement API down → Gateway returns error
- [ ] Learning API down → Gateway returns error
- [ ] Recovery after timeout

### Rate Limiting
- [ ] Requests throttled at limit
- [ ] 429 returned when limit exceeded

---

## Edge Cases

### Validation
- [ ] Empty WiFi/Cell/BLE list
- [ ] Very old timestamp (>12h)
- [ ] Very high speed (impossible)
- [ ] Invalid coordinates (lat > 90)
- [ ] Duplicate requests (idempotency)

### Learning
- [ ] No sources in request
- [ ] All new sources
- [ ] All known sources
- [ ] Concurrent learning requests
- [ ] Large batch of sources

### Infrastructure
- [ ] Redis full → eviction policy
- [ ] ClickHouse write fails → retry
- [ ] Kafka topic not exist → auto-create
