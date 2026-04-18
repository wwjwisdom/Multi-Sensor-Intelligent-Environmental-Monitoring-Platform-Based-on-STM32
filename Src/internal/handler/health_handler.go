package handler

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"sensor-backend/Src/internal/amqp"
	"sensor-backend/Src/internal/iotapi"
	"sensor-backend/Src/internal/mqtt"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/internal/tsdb"
	"sensor-backend/Src/pkg/response"

	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	mqttClient *mqtt.MQTTClient
	amqpClient *amqp.Client
	iotPoller  *iotapi.Poller
	store      *repository.Storage
	tsdbClient *tsdb.Client
}

// NewHealthHandler 创建处理器
func NewHealthHandler(mqttClient *mqtt.MQTTClient, amqpClient *amqp.Client, iotPoller *iotapi.Poller, store *repository.Storage, tsdbClient *tsdb.Client) *HealthHandler {
	return &HealthHandler{
		mqttClient: mqttClient,
		amqpClient: amqpClient,
		iotPoller:  iotPoller,
		store:      store,
		tsdbClient: tsdbClient,
	}
}

// HealthCheck 健康检查
func (h *HealthHandler) HealthCheck(c *gin.Context) {
	components := make(map[string]string)
	overallStatus := "healthy"

	// 检查HTTP
	components["http"] = "ok"

	// 检查 TimescaleDB
	if h.tsdbClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := h.tsdbClient.Ping(ctx); err != nil {
			components["timescaledb"] = "error"
			overallStatus = "degraded"
		} else {
			components["timescaledb"] = "connected"
		}
		cancel()
	} else {
		components["timescaledb"] = "disabled"
	}

	// 检查AMQP（数据接收）
	if h.amqpClient.IsConnected() {
		components["amqp"] = "connected"
	} else {
		components["amqp"] = "disconnected"
		overallStatus = "degraded"
	}

	// 检查MQTT（LED控制）
	if h.mqttClient.IsConnected() {
		components["mqtt"] = "connected"
	} else {
		components["mqtt"] = "disconnected"
		// MQTT断开不影响整体状态（LED控制不可用但数据接收正常）
		if overallStatus == "healthy" {
			overallStatus = "degraded"
		}
	}

	// 检查存储
	dataDir := h.store.GetDataDir()
	if _, err := os.Stat(dataDir); err == nil {
		components["storage"] = "ok"
	} else {
		components["storage"] = "error"
		overallStatus = "unhealthy"
	}

	// 检查磁盘空间
	if free, err := getDiskFreeSpace(dataDir); err == nil {
		if free < 100 {
			components["disk"] = "critical (less than 100MB)"
			overallStatus = "unhealthy"
		} else if free < 1024 {
			components["disk"] = "warning (less than 1GB)"
			if overallStatus == "healthy" {
				overallStatus = "degraded"
			}
		} else {
			components["disk"] = "ok (" + formatBytes(uint64(free*1024*1024)) + " free)"
		}
	} else {
		components["disk"] = "unknown"
	}

	// 获取指标
	metrics := make(map[string]interface{})

	// 从 TimescaleDB 或文件存储获取总记录数
	if h.tsdbClient != nil {
		if total, err := h.tsdbClient.GetTotalRecords(""); err == nil {
			metrics["total_records"] = total
		}
	}
	if _, ok := metrics["total_records"]; !ok {
		metrics["total_records"] = h.store.GetTotalRecords("")
	}

	// 获取最后数据时间
	if h.tsdbClient != nil {
		if data, err := h.tsdbClient.GetAllLatestData(); err == nil && len(data) > 0 {
			for _, data := range data {
				lastTime := time.Unix(data.Timestamp, 0)
				metrics["last_data_time"] = lastTime.Format(time.RFC3339)
				if time.Since(lastTime) > 60*time.Second {
					overallStatus = "degraded"
				}
				break
			}
		}
	}
	if _, ok := metrics["last_data_time"]; !ok {
		allDataRaw := h.store.GetAllRealtime()
		if len(allDataRaw) > 0 {
			for _, data := range allDataRaw {
				lastTime := time.Unix(data.Timestamp, 0)
				metrics["last_data_time"] = lastTime.Format(time.RFC3339)
				if time.Since(lastTime) > 60*time.Second {
					overallStatus = "degraded"
				}
				break
			}
		}
	}

	// 获取运行时统计
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	metrics["memory_used"] = formatBytes(memStats.Alloc)
	metrics["goroutines"] = runtime.NumGoroutine()

	data := gin.H{
		"status":     overallStatus,
		"components": components,
		"metrics":    metrics,
	}

	// 根据状态返回不同HTTP状态码
	if overallStatus == "unhealthy" {
		response.Error(c, 500, 20001, "unhealthy")
		return
	}

	response.Success(c, data)
}

// getDiskFreeSpace 获取磁盘剩余空间(MB)
func getDiskFreeSpace(path string) (int64, error) {
	// 简化实现，实际生产环境需要更完善的实现
	// 这里返回固定值，实际使用时请根据平台实现
	return 10240, nil // 默认返回10GB
}

// formatBytes 格式化字节大小
func formatBytes(bytes uint64) string {
	const unit = 1024.0
	if bytes < uint64(unit) {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / uint64(unit); n >= uint64(unit); n /= uint64(unit) {
		div *= uint64(unit)
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
