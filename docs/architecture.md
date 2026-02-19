# Архитектура системы

## Общая схема

```mermaid
flowchart TB
    subgraph Clients
        Device[Device]
        Service[Service]
    end

    subgraph Gateway
        LB[Load Balancer]
        RateLimit[Rate Limiter]
    end

    subgraph Workers
        W1[Worker #1]
        W2[Worker #2]
        W3[Worker #N]
    end

    subgraph Data
        Redis[(Redis Cache)]
        ClickHouse[(ClickHouse)]
        Kafka[Kafka Events]
    end

    Device --> LB
    Service --> LB
    LB --> RateLimit
    RateLimit --> W1
    RateLimit --> W2
    RateLimit --> W3

    W1 <--> Redis
    W2 <--> Redis
    W3 <--> Redis

    W1 --> ClickHouse
    W2 --> ClickHouse
    W3 --> ClickHouse

    W1 -.-> Kafka
    W2 -.-> Kafka
    W3 -.-> Kafka
```

## Flow валидации

```mermaid
flowchart TD
    Start --> TimeCheck
    
    TimeCheck -->|future| TimeInvalid
    TimeCheck -->|old| TimeOld
    TimeCheck -->|OK| SpeedCheck
    
    SpeedCheck -->|no last| WifiCheck
    SpeedCheck -->|has last| CalcSpeed
    
    CalcSpeed --> SpeedFail
    SpeedFail -->|yes| SpeedInvalid
    SpeedFail -->|no| WifiCheck
    
    WifiCheck -->|has data| WifiLookup
    WifiCheck -->|no data| CellCheck
    
    WifiLookup -->|found| WifiBoost
    WifiLookup -->|not found| WifiLearn
    
    WifiBoost --> CellCheck
    WifiLearn --> CellCheck
    
    CellCheck -->|has data| CellLookup
    CellCheck -->|no data| BTCheck
    
    CellLookup -->|found| CellBoost
    CellLookup -->|not found| CellLearn
    
    CellBoost --> BTCheck
    CellLearn --> BTCheck
    
    BTCheck -->|has data| BTLookup
    BTCheck -->|no data| Final
    
    BTLookup -->|found| BTBoost
    BTLookup -->|not found| Final
    
    BTBoost --> Final
    
    Final --> Result
    
    Result -->|INVALID| OutInvalid
    Result -->|high conf| OutValid
    Result -->|low conf| OutUncertain
    
    TimeInvalid --> Save
    TimeOld --> Save
    SpeedInvalid --> Save
    OutInvalid --> Save
    OutValid --> Save
    OutUncertain --> Save
    
    Save --> UpdateCache
    UpdateCache --> End
```

## Flow самообучения

```mermaid
flowchart LR
    NewWifi[New WiFi] --> RedisWifi[Redis]
    NewWifi --> CHWifi[ClickHouse]
    
    NewCell[New Cell] --> RedisCell[Redis]
    NewCell --> CHCell[ClickHouse]
    
    NewBT[New BLE] --> RedisBT[Redis]
    NewBT --> CHBT[ClickHouse]
```

## Структура Redis

```mermaid
erDiagram
    WIFI ||--o{ DEVICE : "links"
    CELL ||--o{ DEVICE : "links"
    BT ||--o{ DEVICE : "links"
    
    WIFI {
        string bssid PK
        float lat
        float lon
        datetime last_seen
    }
    
    CELL {
        uint32 cell_id PK
        uint32 lac PK
        float lat
        float lon
    }
    
    BT {
        string mac PK
        float lat
        float lon
    }
    
    DEVICE {
        string device_id PK
        float lat
        float lon
    }
```

## Структура ClickHouse

```mermaid
erDiagram
    REQUEST ||--|| STATS : "refs"
    
    REQUEST {
        string device_id PK
        float latitude PK
        float longitude PK
        float accuracy
        datetime timestamp PK
        bool has_wifi
        bool has_bt
        bool has_cell
        string result
        float confidence
    }
    
    STATS {
        string type PK
        string point_id PK
        float lat
        float lon
        int obs
    }
```

## Deployment

```mermaid
flowchart TB
    subgraph K8s
        subgraph Svc
            Ingress[gRPC Ingress]
            SVC[Service LB]
        end
        
        subgraph Pods
            Pod1[validator-1]
            Pod2[validator-2]
            Pod3[validator-N]
        end
        
        subgraph Data
            RedisCl[Redis Cluster]
            CH[ClickHouse]
            KafkaCl[Kafka Cluster]
        end
    end
    
    Clients --> Ingress
    Ingress --> SVC
    SVC --> Pod1
    SVC --> Pod2
    SVC --> Pod3
    
    Pod1 --> RedisCl
    Pod2 --> RedisCl
    Pod3 --> RedisCl
    
    Pod1 --> CH
    Pod2 --> CH
    Pod3 --> CH
    
    Pod1 --> KafkaCl
    Pod2 --> KafkaCl
    Pod3 --> KafkaCl
```

## Алгоритм работы

### Flow валидации

```
Input -> Time Check -> Speed Check -> Source Check -> Result
```

### Flow самообучения

```
New Data -> Redis Cache -> ClickHouse Analytics
```

### Проверка скорости

```
Calculate Haversine -> Distance / Time -> Compare to 150 km/h
```

### Проверка времени

```
Timestamp - Now -> Check bounds (0 to 12 hours)
```
