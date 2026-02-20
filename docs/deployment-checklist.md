# Deployment Checklist

## Pre-Deployment

### Infrastructure
- [ ] Redis cluster provisioned
- [ ] ClickHouse cluster provisioned
- [ ] Kafka cluster provisioned
- [ ] Network connectivity verified
- [ ] Firewall rules configured
- [ ] SSL/TLS certificates ready (if needed)

### Configuration
- [ ] Environment variables set for all services
- [ ] All parameters within valid ranges
- [ ] Service addresses configured (refinement, learning, storage)

### Dependencies
- [ ] Go dependencies downloaded
- [ ] Protobuf compiled
- [ ] Docker images built

---

## Deployment Steps

### 1. Build

```bash
# Build all services
go build -o gateway ./cmd/gateway
go build -o refinement-api ./cmd/refinement-api
go build -o learning-api ./cmd/learning-api
go build -o storage-service ./cmd/storage-service

# Or build Docker
docker-compose build
```

### 2. Infrastructure Setup

```bash
# Start infrastructure
docker-compose up -d redis clickhouse kafka zookeeper

# Verify Redis
redis-cli ping
# Expected: PONG

# Verify ClickHouse
clickhouse-client --query "SELECT 1"
# Expected: 1

# Verify Kafka
kafka-broker-api-versions --bootstrap-server localhost:9092
```

### 3. Configuration

```bash
# Gateway
export SERVER_PORT=50050
export REFINEMENT_ADDR=refinement-api:50051
export LEARNING_ADDR=learning-api:50052

# Refinement/Learning API
export REDIS_ADDR=redis:6379
export MAX_SPEED_KMH=150
export MAX_TIME_DIFF=12h

# Storage Service
export CLICKHOUSE_ADDR=clickhouse:9000
export KAFKA_BROKERS=kafka:9092
```

### 4. Deploy Services

#### Option A: Docker Compose
```bash
docker-compose up -d

# Check status
docker-compose ps
```

#### Option B: Kubernetes
```bash
kubectl apply -f k8s/
```

#### Option C: Binary (local)
```bash
# Terminal 1: Gateway
./gateway

# Terminal 2: Refinement API
./refinement-api

# Terminal 3: Learning API
./learning-api

# Terminal 4: Storage Service
./storage-service
```

### 5. Verify

```bash
# Check Gateway health
grpcurl -plaintext localhost:50050 grpc.health.v1.Health/Check

# Check Refinement API
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

# Check Learning API
grpcurl -plaintext localhost:50052 grpc.health.v1.Health/Check

# Check Storage Service
grpcurl -plaintext localhost:50053 grpc.health.v1.Health/Check
```

---

## Service Ports

| Service | Port | Health Endpoint |
|---------|------|-----------------|
| Gateway | 50050 | localhost:50050/health |
| Refinement API | 50051 | localhost:50051/health |
| Learning API | 50052 | localhost:50052/health |
| Storage Service | 50053 | localhost:50053/health |

---

## Health Checks

### Startup
- [ ] All gRPC servers started
- [ ] Redis connected (Refinement, Learning)
- [ ] ClickHouse connected (Storage)
- [ ] Kafka connected (Storage)
- [ ] Tables created

### Runtime
- [ ] Health endpoints responding
- [ ] No errors in logs
- [ ] Memory usage stable
- [ ] CPU usage normal

### Dependencies
- [ ] Redis reachable from all services
- [ ] ClickHouse reachable from Storage
- [ ] Kafka reachable from Storage
- [ ] Inter-service connectivity

---

## Rollback Plan

### Quick Rollback
```bash
# Docker Compose
docker-compose down
docker-compose up -d

# Kubernetes
kubectl rollout undo deployment/gateway
kubectl rollout undo deployment/refinement-api
kubectl rollout undo deployment/learning-api
kubectl rollout undo deployment/storage-service
```

### Database Rollback
```bash
# No rollback needed - immutable writes
# Point-in-time recovery available in ClickHouse
```

---

## Monitoring

### Key Metrics

**Per Service:**
- Request rate (RPS)
- Latency (p50, p95, p99)
- Error rate
- Memory usage
- CPU usage

**Infrastructure:**
- Redis connections
- ClickHouse batch size / flush latency
- Kafka producer lag

### Alerts
- [ ] High error rate (>1%)
- [ ] High latency (>500ms p99)
- [ ] Redis connection failed
- [ ] ClickHouse connection failed
- [ ] Kafka producer error

---

## Security

### Network
- [ ] gRPC ports not exposed publicly
- [ ] Redis not exposed publicly
- [ ] ClickHouse not exposed publicly
- [ ] Kafka not exposed publicly
- [ ] Load balancer with TLS

### Authentication
- [ ] gRPC auth configured
- [ ] Inter-service auth (mTLS)

### Secrets
- [ ] Redis password (if used)
- [ ] ClickHouse password (if used)
- [ ] Kafka authentication (if used)

---

## Scaling

### Horizontal Scaling

```bash
# Docker Compose
docker-compose up -d --scale refinement-api=3
docker-compose up -d --scale learning-api=2

# Kubernetes
kubectl scale deployment refinement-api --replicas=5
kubectl scale deployment learning-api --replicas=3
```

### Vertical Scaling
- [ ] Increase CPU limits
- [ ] Increase memory limits
- [ ] Tune Redis pool size
- [ ] Tune gRPC connection pools

### Scaling Strategy

| Service | Replicas | Reason |
|---------|----------|--------|
| Gateway | 2-3 | Low CPU, handles routing |
| Refinement API | 5-10 | Hot path, high RPS |
| Learning API | 2-3 | Lower RPS |
| Storage Service | 2 | Async, batch processing |

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
