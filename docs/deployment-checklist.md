# Deployment Checklist

## Pre-Deployment

### Infrastructure
- [ ] Redis cluster provisioned
- [ ] ClickHouse cluster provisioned
- [ ] Network connectivity verified
- [ ] Firewall rules configured
- [ ] SSL/TLS certificates ready (if needed)

### Configuration
- [ ] Environment variables set
- [ ] Configuration file created
- [ ] All parameters within valid ranges
- [ ] Admin credentials configured

### Dependencies
- [ ] Go dependencies downloaded
- [ ] Protobuf compiled
- [ ] Docker images built

---

## Deployment Steps

### 1. Build

```bash
# Build binary
go build -o coordinate-validator ./cmd/server

# Or build Docker
docker build -t coordinate-validator:latest .
```

### 2. Database Setup

```bash
# Verify Redis connection
redis-cli ping
# Expected: PONG

# Verify ClickHouse connection
clickhouse-client --query "SELECT 1"
# Expected: 1
```

### 3. Configuration

```bash
# Set environment variables
export REDIS_ADDR="redis:6379"
export CLICKHOUSE_ADDR="clickhouse:9000"
export SERVER_PORT="50051"
export LOG_LEVEL="info"
```

### 4. Deploy

#### Option A: Docker Compose
```bash
docker-compose up -d
```

#### Option B: Kubernetes
```bash
kubectl apply -f k8s/
```

#### Option C: Binary
```bash
./coordinate-validator
```

### 5. Verify

```bash
# Check health
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
# Expected: {"status":"SERVING"}

# Check config
grpcurl -plaintext localhost:50051 coordinate.Admin/GetConfig
```

---

## Health Checks

### Startup
- [ ] gRPC server started
- [ ] Redis connected
- [ ] ClickHouse connected
- [ ] Tables created

### Runtime
- [ ] Health endpoint responding
- [ ] No errors in logs
- [ ] Memory usage stable
- [ ] CPU usage normal

### Dependencies
- [ ] Redis reachable
- [ ] ClickHouse reachable
- [ ] Network latency acceptable

---

## Rollback Plan

### Quick Rollback
```bash
# Docker
docker-compose down
docker-compose up -d --scale validator=1

# Kubernetes
kubectl rollout undo deployment/validator
```

### Database Rollback
```bash
# No rollback needed - immutable writes
# Point-in-time recovery available if needed
```

---

## Monitoring

### Key Metrics
- [ ] Request rate (RPS)
- [ ] Latency (p50, p95, p99)
- [ ] Error rate
- [ ] Redis connections
- [ ] ClickHouse connections
- [ ] Memory usage
- [ ] CPU usage

### Alerts
- [ ] High error rate (>1%)
- [ ] High latency (>500ms p99)
- [ ] Redis connection failed
- [ ] ClickHouse connection failed

---

## Security

### Network
- [ ] gRPC port not exposed publicly
- [ ] Redis not exposed publicly
- [ ] ClickHouse not exposed publicly
- [ ] Load balancer with TLS

### Authentication
- [ ] gRPC auth configured
- [ ] Admin API protected
- [ ] Metrics endpoint protected

### Secrets
- [ ] Redis password (if used)
- [ ] ClickHouse password (if used)
- [ ] API keys (if used)

---

## Scaling

### Horizontal Scaling
```bash
# Add more instances
docker-compose up -d --scale validator=3

# With load balancer
kubectl scale deployment validator --replicas=5
```

### Vertical Scaling
- [ ] Increase CPU limits
- [ ] Increase memory limits
- [ ] Tune Redis pool size
- [ ] Tune connection pools

---

## Post-Deployment

### Verification
- [ ] Run smoke tests
- [ ] Run integration tests
- [ ] Check logs for errors
- [ ] Verify metrics are reporting

### Documentation
- [ ] Update deployment docs
- [ ] Update runbooks
- [ ] Update contact info

### Notification
- [ ] Notify stakeholders
- [ ] Update status page
- [ ] Set up monitoring dashboards
