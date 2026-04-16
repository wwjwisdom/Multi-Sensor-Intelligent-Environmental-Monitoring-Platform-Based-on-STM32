package service

import (
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/mqtt"
	"sensor-backend/Src/internal/repository"
)

// SensorService 传感器服务
type SensorService struct {
	store      *repository.Storage
	mqttClient *mqtt.MQTTClient
}

// NewSensorService 创建服务实例
func NewSensorService(store *repository.Storage, mqttClient *mqtt.MQTTClient) *SensorService {
	return &SensorService{
		store:      store,
		mqttClient: mqttClient,
	}
}

// GetRealtimeData 获取实时数据
func (s *SensorService) GetRealtimeData(deviceID string) *model.SensorData {
	if deviceID == "" {
		// 返回第一个设备的实时数据
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
	return s.store.GetAllRealtime()
}

// GetHistoryData 获取历史数据
func (s *SensorService) GetHistoryData(deviceID string, startTime, endTime int64, page, pageSize int) ([]model.HistoryRecord, int, error) {
	return s.store.QueryHistory(deviceID, startTime, endTime, page, pageSize)
}

// GetDeviceStatus 获取设备状态
func (s *SensorService) GetDeviceStatus() []model.DeviceStatus {
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
