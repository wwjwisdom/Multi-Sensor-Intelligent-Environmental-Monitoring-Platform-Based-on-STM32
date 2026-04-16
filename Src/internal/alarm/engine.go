package alarm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/pkg/utils"

	"go.uber.org/zap"
)

// AlarmEngine 报警引擎
type AlarmEngine struct {
	store       *repository.Storage
	configPath  string
	alarms      []model.AlarmConfig
	mu          sync.RWMutex
	alarmNotify chan *model.AlarmConfig // 报警通知通道
}

// NewAlarmEngine 创建报警引擎
func NewAlarmEngine(store *repository.Storage) *AlarmEngine {
	configPath := "./Data/config/alarms.json"
	engine := &AlarmEngine{
		store:       store,
		configPath:  configPath,
		alarms:      make([]model.AlarmConfig, 0),
		alarmNotify: make(chan *model.AlarmConfig, 100),
	}
	engine.loadAlarms()
	return engine
}

// loadAlarms 加载报警配置
func (e *AlarmEngine) loadAlarms() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 确保目录存在
	os.MkdirAll(filepath.Dir(e.configPath), 0755)

	if content, err := os.ReadFile(e.configPath); err == nil {
		var config model.AlarmConfigFile
		if err := json.Unmarshal(content, &config); err == nil {
			e.alarms = config.Alarms
			utils.Logger.Info("加载报警配置成功", zap.Int("count", len(e.alarms)))
			return
		}
	}

	// 初始化默认报警配置
	e.alarms = []model.AlarmConfig{
		{
			ID:        "alarm_001",
			Sensor:    "temp",
			Operator:  ">",
			Threshold: 30,
			Message:   "温度过高警告",
			Enabled:   true,
		},
		{
			ID:        "alarm_002",
			Sensor:    "hum",
			Operator:  ">",
			Threshold: 80,
			Message:   "湿度过高警告",
			Enabled:   true,
		},
		{
			ID:        "alarm_003",
			Sensor:    "rain",
			Operator:  "==",
			Threshold: 1,
			Message:   "检测到雨滴",
			Enabled:   true,
		},
	}
	e.saveAlarms()
}

// saveAlarms 保存报警配置
func (e *AlarmEngine) saveAlarms() {
	config := model.AlarmConfigFile{Alarms: e.alarms}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		utils.Logger.Error("序列化报警配置失败", zap.Error(err))
		return
	}

	tmpPath := e.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		utils.Logger.Error("保存报警配置失败", zap.Error(err))
		return
	}

	os.Rename(tmpPath, e.configPath)
	utils.Logger.Debug("报警配置已保存")
}

// CheckAlarms 检查报警
func (e *AlarmEngine) CheckAlarms(data *model.SensorData) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, alarm := range e.alarms {
		if !alarm.Enabled {
			continue
		}

		value := e.getSensorValue(&data.Data, alarm.Sensor)
		if e.checkCondition(value, alarm.Operator, alarm.Threshold) {
			// 触发报警
			utils.Logger.Warn("报警触发",
				zap.String("alarm_id", alarm.ID),
				zap.String("device_id", data.DeviceID),
				zap.String("sensor", alarm.Sensor),
				zap.Float64("value", value),
				zap.Float64("threshold", alarm.Threshold),
				zap.String("message", alarm.Message))

			// 发送报警通知
			select {
			case e.alarmNotify <- &alarm:
			default:
			}
		}
	}
}

// getSensorValue 获取传感器值
func (e *AlarmEngine) getSensorValue(data *model.Data, sensor string) float64 {
	switch sensor {
	case "temp":
		return data.Temperature
	case "hum":
		return data.Humidity
	case "pressure":
		return data.Pressure
	case "light":
		return data.Light
	case "air_quality":
		return data.AirQuality
	case "motion":
		return float64(data.Motion)
	case "rain":
		return float64(data.Rain)
	default:
		return 0
	}
}

// checkCondition 检查条件
func (e *AlarmEngine) checkCondition(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

// GetAlarms 获取所有报警配置
func (e *AlarmEngine) GetAlarms() []model.AlarmConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]model.AlarmConfig, len(e.alarms))
	copy(result, e.alarms)
	return result
}

// GetAlarm 获取单个报警配置
func (e *AlarmEngine) GetAlarm(id string) *model.AlarmConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for i := range e.alarms {
		if e.alarms[i].ID == id {
			return &e.alarms[i]
		}
	}
	return nil
}

// CreateAlarm 创建报警配置
func (e *AlarmEngine) CreateAlarm(alarm model.AlarmConfig) (*model.AlarmConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 生成ID
	alarm.ID = fmt.Sprintf("alarm_%03d", len(e.alarms)+1)
	e.alarms = append(e.alarms, alarm)
	e.saveAlarms()

	return &alarm, nil
}

// UpdateAlarm 更新报警配置
func (e *AlarmEngine) UpdateAlarm(id string, updates map[string]interface{}) (*model.AlarmConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.alarms {
		if e.alarms[i].ID == id {
			// 应用更新
			if v, ok := updates["threshold"]; ok {
				if f, ok := v.(float64); ok {
					e.alarms[i].Threshold = f
				}
			}
			if v, ok := updates["message"]; ok {
				if s, ok := v.(string); ok {
					e.alarms[i].Message = s
				}
			}
			if v, ok := updates["enabled"]; ok {
				if b, ok := v.(bool); ok {
					e.alarms[i].Enabled = b
				}
			}
			if v, ok := updates["operator"]; ok {
				if s, ok := v.(string); ok {
					e.alarms[i].Operator = s
				}
			}

			e.saveAlarms()
			return &e.alarms[i], nil
		}
	}

	return nil, fmt.Errorf("报警配置不存在")
}

// DeleteAlarm 删除报警配置
func (e *AlarmEngine) DeleteAlarm(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.alarms {
		if e.alarms[i].ID == id {
			e.alarms = append(e.alarms[:i], e.alarms[i+1:]...)
			e.saveAlarms()
			return nil
		}
	}

	return fmt.Errorf("报警配置不存在")
}

// GetAlarmNotifyChannel 获取报警通知通道
func (e *AlarmEngine) GetAlarmNotifyChannel() <-chan *model.AlarmConfig {
	return e.alarmNotify
}
