# TimescaleDB 集成指南

## 概述

本项目支持使用 TimescaleDB 作为时序数据库存储传感器历史数据，提供高性能的时序数据写入和查询能力。

## 快速开始

### 1. 启动 TimescaleDB

```bash
# 使用 Docker 启动
docker-compose up -d

# 或者手动启动
docker run -d \
  --name sensor-timescaledb \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=sensor123 \
  -e POSTGRES_DB=sensor_db \
  -p 5432:5432 \
  timescale/timescaledb:latest-pg16
```

### 2. 配置后端

编辑 `Data/config/config.yaml`:

```yaml
timescaledb:
  enabled: true
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "sensor123"
  dbname: "sensor_db"
  sslmode: "disable"
```

### 3. 启动后端

```bash
./sensor-backend.exe
```

## 数据库表结构

### sensor_data (时序表)

```sql
CREATE TABLE sensor_data (
    time        TIMESTAMPTZ NOT NULL,
    device_id   TEXT NOT NULL,
    temperature DOUBLE PRECISION,
    humidity    DOUBLE PRECISION,
    pressure    DOUBLE PRECISION,
    light       DOUBLE PRECISION,
    air_quality DOUBLE PRECISION,
    motion      INTEGER,
    rain        INTEGER
);

-- 自动转换为 hypertable
SELECT create_hypertable('sensor_data', 'time');
```

### device_status (设备状态表)

```sql
CREATE TABLE device_status (
    device_id     TEXT PRIMARY KEY,
    online        BOOLEAN DEFAULT FALSE,
    last_seen     TIMESTAMPTZ,
    total_records BIGINT DEFAULT 0,
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);
```

## 特性

### 自动降级

当 TimescaleDB 不可用时，系统会自动降级到文件存储模式，确保服务不中断。

### 数据兼容

- 新数据同时写入 TimescaleDB 和文件存储
- 历史查询优先从 TimescaleDB 获取
- 清理任务同时清理两种存储

### 性能优化

- 连接池管理（默认 25 连接）
- 批量写入支持
- 自动索引优化

## 常用查询

### 查询最近24小时数据

```sql
SELECT * FROM sensor_data
WHERE time > NOW() - INTERVAL '24 hours'
ORDER BY time DESC;
```

### 查询设备统计

```sql
SELECT
    device_id,
    COUNT(*) as record_count,
    AVG(temperature) as avg_temp,
    MIN(time) as first_seen,
    MAX(time) as last_seen
FROM sensor_data
GROUP BY device_id;
```

### 清理旧数据

```sql
-- 删除30天前的数据
DELETE FROM sensor_data WHERE time < NOW() - INTERVAL '30 days';
```

## 监控

查看数据库连接数：

```sql
SELECT count(*) FROM pg_stat_activity WHERE datname = 'sensor_db';
```

查看表大小：

```sql
SELECT pg_size_pretty(pg_total_relation_size('sensor_data'));
```
