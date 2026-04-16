# Multi-Sensor-Intelligent-Environmental-Monitoring-Platform-Based-on-STM32

## 概述

本系统基于 Go + Gin 框架开发，通过 AMQP 协议接收阿里云 IoT 平台设备数据，提供 HTTP REST API 给前端使用。

---

## 部署步骤

### 1. 克隆代码

```bash
git clone https://github.com/wwjwisdom/Multi-Sensor-Intelligent-Environmental-Monitoring-Platform-Based-on-STM32.git
cd Multi-Sensor-Intelligent-Environmental-Monitoring-Platform-Based-on-STM32
```

### 2. 配置

```bash
# 复制配置模板
cp Data/config/config.yaml.example Data/config/config.yaml

# 编辑配置文件，填入阿里云 AccessKey
vi Data/config/config.yaml
```

### 3. 编译运行

```bash
cd Src
go build -o sensor-backend.exe ./cmd/main.go
./sensor-backend.exe
```

服务将在 `http://localhost:8080` 启动。

---

## 快速开始（已配置环境）

```bash
cd Src
./sensor-backend.exe
```

---

## API 文档

### 基础信息

- **Base URL**: `http://localhost:8080/api/v1`
- **数据格式**: JSON
- **时区**: 北京时间 (UTC+8)

---

### 1. 传感器数据

#### 获取实时数据

```
GET /sensors/realtime
```

参数: `device_id` (可选，不填返回所有设备)

响应:
```json
{
  "code": 0,
  "data": {
    "devices": {
      "STM32-EnvMonitor-001": {
        "device_id": "STM32-EnvMonitor-001",
        "timestamp": 1776337169,
        "data": {
          "temp": 26.2,
          "hum": 53,
          "pressure": 199.79,
          "light": 9,
          "air_quality": 9.63,
          "motion": 1,
          "rain": 0
        }
      }
    }
  }
}
```

字段: temp(温度℃), hum(湿度%RH), pressure(气压hPa), light(光照lux), air_quality(空气质量), motion(人体感应0/1), rain(雨滴0/1)

#### 查询历史数据

```
GET /sensors/history
```

参数:
- `start_time`: 开始时间 (Unix时间戳, 必需)
- `end_time`: 结束时间 (Unix时间戳, 必需)
- `device_id`: 设备ID (可选)
- `page`: 页码 (默认1)
- `page_size`: 每页数量 (默认20, 最大100)

注意: 时间范围最大支持7天

---

### 2. 设备状态

```
GET /devices/status
```

返回所有设备在线状态和最后活跃时间。

---

### 3. 报警管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /alarms | 获取报警列表 |
| POST | /alarms | 创建报警规则 |
| PUT | /alarms/:id | 更新报警规则 |
| DELETE | /alarms/:id | 删除报警规则 |

---

### 4. LED 控制

```
POST /led/control
Body: {"device_id": "xxx", "red": 255, "green": 0, "blue": 0}
```

RGB 值范围 0-255

---

### 5. 健康检查

```
GET /health
```

返回系统各组件状态 (AMQP/HTTP/MQTT/Storage)

---

## 配置

配置文件: `Data/config/config.yaml`

- `amqp.enabled`: 启用 AMQP 数据接收
- `storage.retention_days`: 历史数据保留天数 (默认180天)
- `alarm.enabled`: 启用报警功能

---

## 数据存储

- **实时数据**: 内存存储
- **历史数据**: `Data/history/` 目录，按日期分文件
- **保留期限**: 180天，自动清理

---

## 代码统计

| 模块 | 行数 |
|------|------|
| internal/amqp (AMQP客户端) | 817 |
| internal/mqtt (MQTT客户端) | 376 |
| internal/handler (HTTP处理器) | 436 |
| internal/repository (数据存储) | 230 |
| internal/alarm (报警引擎) | 267 |
| cmd/main.go | 151 |
| 其他 | 398 |
| **总计** | **2675** |
