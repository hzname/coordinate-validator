# Архитектура системы

## Общая схема

```mermaid
flowchart TB
    subgraph Clients["Источники данных"]
        Device[Устройство]
        Service[Другой сервис]
    end

    subgraph Gateway["gRPC Gateway"]
        LB[Load Balancer]
        RateLimit[Rate Limiter]
    end

    subgraph Workers["Validator Workers"]
        W1[Worker #1]
        W2[Worker #2]
        W3[Worker #N]
    end

    subgraph Data["Слой данных"]
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
    Start([Получил координаты]) --> TimeCheck{Проверка времени}
    
    TimeCheck -->|timestamp в будущем| TimeInvalid[INVALID: будущее время]
    TimeCheck -->|timestamp старше 12ч| TimeOld[INVALID: старое время]
    TimeCheck -->|OK| SpeedCheck
    
    SpeedCheck --> LastLoc{Есть последняя<br/>координата?}
    
    LastLoc -->|Нет| WifiCheck
    LastLoc -->|Да| CalcSpeed[Расчёт скорости]
    
    CalcSpeed --> SpeedValid{Скорость<br/>> 150 км/ч?}
    
    SpeedValid -->|Да| SpeedInvalid[INVALID: невозможная скорость]
    SpeedValid -->|Нет| WifiCheck
    
    WifiCheck --> WifiData{Есть WiFi<br/>данные?}
    
    WifiData -->|Да| WifiKnown{Известные<br/>точки?}
    WifiData -->|Нет| CellCheck
    
    WifiKnown -->|Да| WifiBoost[ confidence +0.3]
    WifiKnown -->|Нет| WifiRecord[Записать<br/>на обучение]
    
    WifiBoost --> CellCheck
    WifiRecord --> CellCheck
    
    CellCheck --> CellData{Есть Cell<br/>данные?}
    
    CellData -->|Да| CellKnown{Известная<br/>вышка?}
    CellData -->|Нет| BTCheck
    
    CellKnown -->|Да| CellBoost[ confidence +0.2]
    CellKnown -->|Нет| CellRecord[Записать<br/>на обучение]
    
    CellBoost --> BTCheck
    CellRecord --> BTCheck
    
    BTCheck --> BTData{Есть BT<br/>данные?}
    
    BTData -->|Да| BTKnown{Известные<br/>устройства?}
    BTData -->|Нет| Final
    
    BTKnown -->|Да| BTBoost[ confidence +0.1]
    BTKnown -->|Нет| Final
    
    BTBoost --> Final
    
    Final --> Result{Итог}
    
    Result -->|INVALID| OutInvalid[INVALID]
    Result -->|confidence > 0.7| OutValid[VALID]
    Result -->|confidence <= 0.7| OutUncertain[UNCERTAIN]
    
    TimeInvalid --> Save[Сохранить в ClickHouse]
    TimeOld --> Save
    SpeedInvalid --> Save
    OutInvalid --> Save
    OutValid --> Save
    OutUncertain --> Save
    
    Save --> UpdateCache[Обновить кэш]
    UpdateCache --> End([Ответ: VALID/INVALID/UNCERTAIN])
```

## Структура Redis

```mermaid
erDiagram
    WIFI_POINT ||--o{ DEVICE_LAST_KNOWN : "linked"
    CELL_POINT ||--o{ DEVICE_LAST_KNOWN : "linked"
    BT_POINT ||--o{ DEVICE_LAST_KNOWN : "linked"
    
    WIFI_POINT {
        string bssid PK
        float lat
        float lon
        datetime last_seen
        int count
    }
    
    CELL_POINT {
        string cell_id PK
        int lac PK
        float lat
        float lon
        datetime last_seen
    }
    
    BT_POINT {
        string mac PK
        float lat
        float lon
        datetime last_seen
    }
    
    DEVICE_LAST_KNOWN {
        string device_id PK
        float lat
        float lon
        datetime time
    }
```

## Структура ClickHouse

```mermaid
erDiagram
    COORDINATE_REQUESTS ||--|| POINT_STATS : "references"
    
    COORDINATE_REQUESTS {
        string device_id PK
        float latitude PK
        float longitude PK
        float accuracy
        datetime timestamp PK
        bool has_wifi
        bool has_bluetooth
        bool has_cell
        enum validation_result
        float confidence
        datetime created_at
    }
    
    POINT_STATS {
        enum point_type PK
        string point_id PK
        float latitude
        float longitude
        int observations
        datetime last_observed
        float accuracy
    }
```

## Алгоритм работы

### Flow валидации

```mermaid
flowchart TD
    subgraph Input["Входные данные (EGTS)"]
        Coord[Координаты: lat, lon, time]
        Wifi[WiFi: BSSID, RSSI]
        Cell[Cell: CID, LAC, MCC, MNC, RSSI]
        BT[BLE: MAC, RSSI]
    end

    subgraph Validation["Валидация"]
        TimeCheck{Время<br/>валидно?}
        TimeCheck -->|Нет| TimeFail[INVALID:<br/>будущее/старое]
        TimeCheck -->|Да| SpeedCheck
        
        SpeedCheck{Есть<br/>предыдущая<br/>координата?}
        SpeedCheck -->|Нет| WifiCheck
        SpeedCheck -->|Да| CalcSpeed[Расчёт<br/>скорости]
        
        CalcSpeed --> SpeedFail{>150<br/>км/ч?}
        SpeedFail -->|Да| SpeedFailRes[INVALID:<br/>невозможная<br/>скорость]
        SpeedFail -->|Нет| WifiCheck
        
        WifiCheck{WiFi<br/>данные<br/>есть?}
        WifiCheck -->|Да| WifiLookup[Поиск в Redis<br/>wifi:{BSSID}]
        WifiCheck -->|Нет| CellCheck
        
        WifiLookup --> WifiFound{Найден?}
        WifiFound -->|Да| WifiBoost[ confidence +0.3]
        WifiFound -->|Нет| WifiLearn[Записать на<br/>обучение]
        WifiBoost --> CellCheck
        WifiLearn --> CellCheck
        
        CellCheck{Cell towers<br/>есть?}
        CellCheck -->|Да| CellLookup[Поиск в Redis<br/>cell:{CID}:{LAC}]
        CellCheck -->|Нет| BTCheck
        
        CellLookup --> CellFound{Найден?}
        CellFound -->|Да| CellBoost[ confidence +0.2]
        CellFound -->|Нет| CellLearn[Записать на<br/>обучение]
        CellBoost --> BTCheck
        CellLearn --> BTCheck
        
        BTCheck{BLE<br/>данные<br/>есть?}
        BTCheck -->|Да| BTLookup[Поиск в Redis<br/>bt:{MAC}]
        BTCheck -->|Нет| FinalCheck
        
        BTLookup --> BTFound{Найден?}
        BTFound -->|Да| BTBoost[ confidence +0.1]
        BTFound -->|Нет| FinalCheck
        BTBoost --> FinalCheck
    end

    subgraph Decision["Итоговое решение"]
        FinalCheck{confidence<br/>> 0.5?}
        FinalCheck -->|Да| Valid[VALID]
        FinalCheck -->|Нет| Uncertain[UNCERTAIN]
        
        TimeFail --> Result[INVALID]
        SpeedFailRes --> Result
        Valid --> Result
        Uncertain --> Result
    end

    subgraph Output["Выход"]
        Result --> SaveHist[Сохранить в<br/>ClickHouse]
        Result --> UpdateCache[Обновить<br/>кеш]
        UpdateCache --> Response[Ответ:<br/>VALID/INVALID/UNCERTAIN]
    end
```

### Flow самообучения

```mermaid
flowchart LR
    subgraph NewData["Новые данные"]
        NewWifi[Новый WiFi<br/>BSSID: AA:BB:CC...]
        NewCell[Новая сота<br/>CID: 12345, LAC: 678]
        NewBT[Новый BLE<br/>MAC: 11:22:33...]
    end

    subgraph Redis["Redis Cache"]
        WifiKey["wifi:AA:BB:CC..."]
        CellKey["cell:12345:678"]
        BTKey["bt:11:22:33..."]
    end

    subgraph ClickHouse["ClickHouse Analytics"]
        CH_Wifi["point_stats<br/>(wifi)"]
        CH_Cell["point_stats<br/>(cell)"]
        CH_BT["point_stats<br/>(bt)"]
    end

    NewWifi -->|SET if not exists| WifiKey
    NewWifi -->|INSERT| CH_Wifi
    
    NewCell -->|SET if not exists| CellKey
    NewCell -->|INSERT| CH_Cell
    
    NewBT -->|SET if not exists| BTKey
    NewBT -->|INSERT| CH_BT
```

### Детализация: Проверка по источникам

```mermaid
flowchart TB
    subgraph WiFiFlow["Проверка WiFi"]
        W1[Получил BSSID]
        W2[Запрос в Redis<br/>GET wifi:{BSSID}]
        W3{Ключ<br/>существует?}
        W3 -->|Да| W4[confidence += 0.3]
        W3 -->|Нет| W5[SET wifi:{BSSID}<br/>{lat, lon, time, count:1}]
        W5 --> W6[INSERT point_stats<br/>type: wifi]
        W4 --> W7[Результат]
    end

    subgraph CellFlow["Проверка Cell Tower"]
        C1[Получил CID, LAC]
        C2[Запрос в Redis<br/>GET cell:{CID}:{LAC}]
        C3{Ключ<br/>существует?}
        C3 -->|Да| C4[confidence += 0.2]
        C3 -->|Нет| C5[SET cell:{CID}:{LAC}<br/>{lat, lon, time, MCC, MNC}]
        C5 --> C6[INSERT point_stats<br/>type: cell]
        C4 --> C7[Результат]
    end

    subgraph BTFlow["Проверка Bluetooth"]
        B1[Получил MAC]
        B2[Запрос в Redis<br/>GET bt:{MAC}]
        B3{Ключ<br/>существует?}
        B3 -->|Да| B4[confidence += 0.1]
        B3 -->|Нет| B5[SET bt:{MAC}<br/>{lat, lon, time}]
        B5 --> B6[INSERT point_stats<br/>type: bt]
        B4 --> B7[Результат]
    end
```

### Проверка скорости

```mermaid
flowchart TD
    Start[Получил координаты] --> HasLast{Есть<br/>последняя<br/>координата?}
    
    HasLast -->|Да| Calc[Расчёт расстояния<br/>Haversine]
    HasLast -->|Нет| SkipSpeed[Пропустить]
    
    Calc --> Distance[Расстояние в км]
    Distance --> TimeDiff[Время в часах]
    TimeDiff --> Speed[Скорость =<br/>Расстояние/Время]
    
    Speed --> CheckSpeed{Скорость<br/>> 150<br/>км/ч?}
    
    CheckSpeed -->|Да| InvalidSpeed[INVALID:<br/>impossible speed]
    CheckSpeed -->|Нет| OKSpeed[OK]
```

### Проверка времени

```mermaid
flowchart TD
    Start[Получил timestamp] --> Now[Текущее время]
    
    Now --> Diff{timestamp - Now}
    
    Diff -->|Отрицательное| Future[В будущем]
    Future --> InvalidFuture[INVALID:<br/>timestamp in future]
    
    Diff -->|Положительное| CheckOld{> 12 часов?}
    
    CheckOld -->|Да| TooOld[Старше 12ч]
    TooOld --> InvalidOld[INVALID:<br/>timestamp too old]
    
    CheckOld -->|Нет| ValidTime[OK]
```

## Deployment

```mermaid
flowchart TB
    subgraph Kubernetes["Kubernetes Cluster"]
        subgraph Services["Services"]
            Ingress[gRPC Ingress]
            SVC[Service LB]
        end
        
        subgraph Pods["Pods"]
            Pod1[validator-1]
            Pod2[validator-2]
            Pod3[validator-N]
        end
        
        subgraph Data["Data Layer"]
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
