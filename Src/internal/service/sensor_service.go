package service

import (
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/mqtt"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/internal/tsdb"
)

// SensorService 传感器服务
type SensorService struct {
	store      *repository.Storage
	mqttClient *mqtt.MQTTClient
	tsdbClient *tsdb.Client
}

// NewSensorService 创建服务实例
func NewSensorService(store *repository.Storage, mqttClient *mqtt.MQTTClient, tsdbClient *tsdb.Client) *SensorService {
	return &SensorService{
		store:      store,
		mqttClient: mqttClient,
		tsdbClient: tsdbClient,
	}
}

// GetRealtimeData 获取实时数据
func (s *SensorService) GetRealtimeData(deviceID string) *model.SensorData {
	// 优先从 TimescaleDB 获取最新数据
	if s.tsdbClient != nil {
		data, err := s.tsdbClient.GetLatestData(deviceID)
		if err == nil && data != nil {
			return data
		}
	}

	// 降级到文件存储
	if deviceID == "" {
		all := s.store.GetAllRealtime()
		for _, data := range all {
			return data
		}
		return nil
	}
	return s.store.GetRealtime(deviceID)
}

// GetAllRealtimeData 获取所有设备实时数据
func (s *SensorService) GetAllRealtimeData() map[string]*model.SensorData {
	// 优先从 TimescaleDB 获取所有设备最新数据
	if s.tsdbClient != nil {
		data, err := s.tsdbClient.GetAllLatestData()
		if err == nil && len(data) > 0 {
			return data
		}
	}

	// 降级到文件存储
	return s.store.GetAllRealtime()
}

// GetHistoryData 获取历史数据
func (s *SensorService) GetHistoryData(deviceID string, startTime, endTime int64, page, pageSize int) ([]model.HistoryRecord, int, error) {
	// 优先从 TimescaleDB 查询历史数据
	if s.tsdbClient != nil {
		records, total, err := s.tsdbClient.QueryHistory(deviceID, startTime, endTime, page, pageSize)
		if err == nil {
			return records, total, nil
		}
	}

	// 降级到文件存储
	return s.store.QueryHistory(deviceID, startTime, endTime, page, pageSize)
}

// GetDeviceStatus 获取设备状态
func (s *SensorService) GetDeviceStatus() []model.DeviceStatus {
	// 优先从 TimescaleDB 获取设备状态
	if s.tsdbClient != nil {
		statuses, err := s.tsdbClient.GetDeviceStatus()
		if err == nil && len(statuses) > 0 {
			return statuses
		}
	}

	// 降级到文件存储
	allData := s.store.GetAllRealtime()
	statuses := make([]model.DeviceStatus, 0, len(allData))

	for deviceID := range allData {
		status := model.DeviceStatus{
			DeviceID:     deviceID,
			Online:       s.store.IsDeviceOnline(deviceID),
			LastSeen:     s.store.GetDeviceLastSeen(deviceID),
			TotalRecords: s.store.GetTotalRecords(deviceID),
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// ControlRGBLED 控制RGB LED
func (s *SensorService) ControlRGBLED(deviceID string, red, green, blue int) error {
	return s.mqttClient.PublishControl(deviceID, red, green, blue)
}

// IsMQTTConnected 检查MQTT连接状态
func (s *SensorService) IsMQTTConnected() bool {
	return s.mqttClient.IsConnected()
}

// GetDataDir 获取数据目录
func (s *SensorService) GetDataDir() string {
	return s.store.GetDataDir()
}

// IsTimescaleDBEnabled 检查 TimescaleDB 是否启用
func (s *SensorService) IsTimescaleDBEnabled() bool {
	return s.tsdbClient != nil
}
