package iotapi

import (
	"context"
	"encoding/json"
	"time"

	"sensor-backend/Src/internal/alarm"
	"sensor-backend/Src/internal/config"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/pkg/utils"

	"go.uber.org/zap"
)

// Poller HTTP API轮询器
type Poller struct {
	client      *Client
	store       *repository.Storage
	alarmEngine *alarm.AlarmEngine
	wsHandler   interface{ Broadcast([]byte) }
	interval    time.Duration
	enabled     bool
	cancel      context.CancelFunc
}

// NewPoller 创建HTTP API轮询器
func NewPoller(cfg config.IOTAPIConfig, store *repository.Storage, alarmEngine *alarm.AlarmEngine) *Poller {
	client := NewClient(
		"dummy_access_key",  // 模拟版本不需要真实密钥
		"dummy_secret",
		cfg.Region,
		"dummy_instance",
		cfg.ProductKey,
		cfg.DeviceName,
	)

	return &Poller{
		client:      client,
		store:       store,
		alarmEngine: alarmEngine,
		interval:    time.Duration(cfg.PollInterval) * time.Second,
		enabled:     cfg.Enabled,
	}
}

// SetWebSocketHandler 设置WebSocket处理器
func (p *Poller) SetWebSocketHandler(handler interface{ Broadcast([]byte) }) {
	p.wsHandler = handler
}

// Start 启动轮询
func (p *Poller) Start() error {
	if !p.enabled {
		utils.Logger.Info("HTTP API轮询已禁用，跳过启动")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go p.pollLoop(ctx)

	utils.Logger.Info("HTTP API轮询器已启动",
		zap.Duration("interval", p.interval),
		zap.String("device", p.client.deviceName))

	return nil
}

// Stop 停止轮询
func (p *Poller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	utils.Logger.Info("HTTP API轮询器已停止")
}

// pollLoop 轮询循环
func (p *Poller) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// 启动时立即获取一次
	p.fetchAndProcessData(ctx)

	for {
		select {
		case <-ctx.Done():
			utils.Logger.Info("HTTP API轮询器退出")
			return
		case <-ticker.C:
			p.fetchAndProcessData(ctx)
		}
	}
}

// fetchAndProcessData 获取并处理数据
func (p *Poller) fetchAndProcessData(ctx context.Context) {
	sensorData, err := p.client.GetDevicePropertyData(ctx)
	if err != nil {
		utils.Logger.Error("HTTP API获取数据失败",
			zap.Error(err),
			zap.String("device", p.client.deviceName))
		return
	}

	if sensorData == nil {
		utils.Logger.Warn("HTTP API返回空数据")
		return
	}

	// 验证数据
	if !validateSensorData(sensorData) {
		utils.Logger.Warn("传感器数据验证失败", zap.Any("data", sensorData))
		return
	}

	// 更新实时数据
	p.store.UpdateRealtime(sensorData)
	utils.Logger.Info("HTTP API数据已更新",
		zap.String("device_id", sensorData.DeviceID),
		zap.Float64("temp", sensorData.Data.Temperature),
		zap.Float64("hum", sensorData.Data.Humidity),
		zap.String("time", time.Now().Format("15:04:05.000")))

	// 保存历史数据
	if err := p.store.SaveHistory(sensorData); err != nil {
		utils.Logger.Error("保存历史数据失败", zap.Error(err))
	}

	// 检查报警
	p.alarmEngine.CheckAlarms(sensorData)

	// 通过WebSocket广播给前端
	if p.wsHandler != nil {
		sensorJSON, _ := json.Marshal(sensorData)
		p.wsHandler.Broadcast(sensorJSON)
	}
}

// validateSensorData 验证传感器数据
func validateSensorData(data *model.SensorData) bool {
	d := data.Data

	// 温度：-40~80℃
	if d.Temperature != 0 && (d.Temperature < -40 || d.Temperature > 80) {
		return false
	}

	// 湿度：0~100%RH
	if d.Humidity != 0 && (d.Humidity < 0 || d.Humidity > 100) {
		return false
	}

	// 大气压力：0~1100 hPa
	if d.Pressure != 0 && (d.Pressure < 0 || d.Pressure > 1100) {
		return false
	}

	// 光照：0~65535 lux
	if d.Light != 0 && (d.Light < 0 || d.Light > 65535) {
		return false
	}

	// 空气质量：0~500 AQI
	if d.AirQuality != 0 && (d.AirQuality < 0 || d.AirQuality > 500) {
		return false
	}

	return true
}