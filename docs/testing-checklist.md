# Testing Checklist

## Unit Tests

### Config
- [ ] Load configuration from file
- [ ] Load configuration from environment variables
- [ ] Validate parameter ranges
- [ ] Reset to defaults

### Cache (Redis)
- [ ] Connect to Redis
- [ ] Get/Set WiFi point
- [ ] Get/Set Cell tower
- [ ] Get/Set BLE device
- [ ] Get/Set device last location
- [ ] TTL expiration
- [ ] Error handling (connection lost)

### Storage (ClickHouse)
- [ ] Connect to ClickHouse
- [ ] Create tables
- [ ] Insert coordinate record
- [ ] Query coordinate history
- [ ] Update point statistics
- [ ] Error handling

### Validator Service
- [ ] Time validation - future timestamp
- [ ] Time validation - old timestamp (>12h)
- [ ] Time validation - valid timestamp
- [ ] Speed validation - no previous location
- [ ] Speed validation - valid speed
- [ ] Speed validation - impossible speed (>150km/h)
- [ ] WiFi validation - known point
- [ ] WiFi validation - unknown point
- [ ] Cell validation - known tower
- [ ] Cell validation - unknown tower
- [ ] BLE validation - known device
- [ ] BLE validation - unknown device
- [ ] Confidence calculation
- [ ] UNCERTAIN result when confidence < 0.5
- [ ] Save to history on validation

### Learning Service
- [ ] New source - first observation
- [ ] Random source - insufficient observations
- [ ] Stationary detection - low variance
- [ ] Stationary detection - high variance
- [ ] Update CALCULATED coordinates
- [ ] Do NOT learn from ABSOLUTE sources

### Companion/Stationary Detection
- [ ] First appearance = NEW status
- [ ] After MIN observations - check variance
- [ ] Low variance = STATIONARY
- [ ] High variance = RANDOM
- [ ] Do NOT learn RANDOM sources

### Triangulation
- [ ] Intersection of two circles
- [ ] Intersection of three circles
- [ ] No intersection - fallback to weighted average
- [ ] Single source - use as-is

### Admin Service
- [ ] Get all config
- [ ] Update single parameter
- [ ] Validate parameter range on update
- [ ] Reset to defaults
- [ ] Config history tracking

---

## Integration Tests

### gRPC API - Validate
- [ ] Valid request returns VALID
- [ ] Invalid timestamp returns INVALID
- [ ] Invalid speed returns INVALID
- [ ] Unknown sources returns UNCERTAIN
- [ ] Batch validation works

### gRPC API - Learn
- [ ] New source returns NEED_MORE_DATA
- [ ] Stationary source returns LEARNED
- [ ] Random source returns EXCLUDED

### gRPC API - Admin
- [ ] Get config returns all parameters
- [ ] Update config persists change
- [ ] Reset config restores defaults

### End-to-End
- [ ] Validate coordinates with known WiFi
- [ ] Validate coordinates with unknown WiFi
- [ ] Learn from stationary source
- [ ] Do NOT learn from random source
- [ ] Full flow: Validate → Learn → Validate

---

## Performance Tests

- [ ] 100 RPS - latency < 50ms
- [ ] 500 RPS - latency < 100ms
- [ ] 1000 RPS - latency < 200ms
- [ ] Memory usage < 200MB under load
- [ ] Redis connection pool handling

---

## Error Handling Tests

- [ ] Redis connection lost - graceful degradation
- [ ] ClickHouse connection lost - queue writes
- [ ] Invalid request data - proper error response
- [ ] Timeout handling

---

## Edge Cases

- [ ] Empty WiFi/Cell/BLE list
- [ ] Very old timestamp
- [ ] Very high speed (impossible)
- [ ] Duplicate requests (idempotency)
- [ ] Concurrent learning requests
- [ ] Large batch of sources
