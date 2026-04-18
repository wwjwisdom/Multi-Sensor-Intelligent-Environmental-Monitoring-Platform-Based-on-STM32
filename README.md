# Multi-Sensor-Intelligent-Environmental-Monitoring-Platform-Based-on-STM32

基于 STM32 + 阿里云 IoT 的多传感器智能环境监测平台后端服务。

---

## 技术栈

### 后端

| 分类 | 技术 | 说明 |
|------|------|------|
| **语言** | Go 1.21+ | 高性能并发语言 |
| **框架** | Gin | 轻量级 Web 框架 |
| **协议** | AMQP | 接收阿里云 IoT 平台推送的设备数据 |
| **数据库** | TimescaleDB | 时序数据库，基于 PostgreSQL，用于存储历史传感器数据 |
| **容器** | Docker | 数据库等服务的容器化部署 |
| **日志** | Zap | Uber 开源的高性能日志库 |

### 前端

| 分类 | 技术 | 说明 |
|------|------|------|
| **语言** | TypeScript | 类型安全的 JavaScript 超集 |
| **框架** | Vue 3 | 渐进式 JavaScript 框架 |
| **构建** | Vite | 下一代前端构建工具 |
| **UI 组件** | Element Plus | 基于 Vue 3 的组件库 |
| **图表** | ECharts / vue-echarts | 数据可视化图表库 |
| **状态管理** | Pinia | Vue 3 官方推荐的状态管理库 |
| **样式** | CSS3 | 自定义赛博朋克/工业风格主题 |

### 部署环境

| 组件 | 说明 |
|------|------|
| Docker Desktop | 容器化运行 TimescaleDB |
| TimescaleDB | 时序数据库服务 |
| 阿里云 IoT | 设备接入平台 |

---

## 系统架构

```
STM32 传感器设备
        │
        ▼
  阿里云 IoT 平台
        │
        ▼
  AMQP 协议推送
        │
        ▼
   Go 后端服务
        │
   ┌────┴────┐
   ▼         ▼
TimescaleDB  文件存储
   │         │
   └────┬────┘
        ▼
   前端展示 (Vue 3)
```

---

## 功能特性

- 实时传感器数据接收与展示
- 历史数据查询与可视化
- 多传感器支持：温度、湿度、气压、光照、空气质量、人体感应、雨量
- 报警规则配置与触发
- RGB LED 远程控制
- WebSocket 实时推送

---

## API 文档

### 基础信息

- **Base URL**: `http://localhost:8080/api/v1`
- **数据格式**: JSON
- **时区**: 北京时间 (UTC+8)

### 端点列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/sensors/realtime` | 获取实时传感器数据 |
| GET | `/sensors/history` | 查询历史数据 |
| GET | `/devices/status` | 获取设备状态 |
| GET | `/alarms` | 获取报警列表 |
| POST | `/alarms` | 创建报警规则 |
| PUT | `/alarms/:id` | 更新报警规则 |
| DELETE | `/alarms/:id` | 删除报警规则 |
| POST | `/led/control` | 控制 RGB LED |
| GET | `/health` | 健康检查 |

---

## 配置

配置文件: `Data/config/config.yaml`

主要配置项：
- `amqp` - 阿里云 AMQP 连接配置
- `timescaledb` - TimescaleDB 连接配置
- `alarm` - 报警规则配置
- `storage` - 历史数据存储配置

---

## 数据字段

| 字段 | 类型 | 说明 | 单位 |
|------|------|------|------|
| temperature | float64 | 温度 | ℃ |
| humidity | float64 | 湿度 | %RH |
| pressure | float64 | 气压 | hPa |
| light | int | 光照强度 | lux |
| air_quality | float64 | 空气质量 | AQI |
| motion | int | 人体感应 | 0/1 |
| rain | int | 雨滴检测 | 0/1 |

---

## 快速开始

详见 [STARTUP_GUIDE.md](./STARTUP_GUIDE.md)

```powershell
# 1. 启动数据库
docker-compose up -d

# 2. 启动后端
cd Src
./main.exe

# 3. 启动前端
cd frontend
npm run dev
```

---

## 项目结构

```
├── Data/              # 数据存储
│   ├── config/       # 配置文件
│   └── history/      # 历史数据文件
├── Src/              # Go 源代码
│   ├── cmd/          # 程序入口
│   ├── internal/     # 内部包
│   └── pkg/         # 公共工具
├── docker-compose.yml # Docker 配置
└── README.md
```

---

## 前端仓库

https://github.com/wwjwisdom/stm32-iot-frontend
