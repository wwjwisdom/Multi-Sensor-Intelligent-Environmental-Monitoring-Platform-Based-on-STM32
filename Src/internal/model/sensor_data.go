package model

import "time"

// SensorData 传感器数据结构
type SensorData struct {
	DeviceID  string `json:"device_id"`
	Timestamp int64  `json:"timestamp"`
	Data      Data   `json:"data"`
}

// Data 传感器数据详情
type Data struct {
	Temperature float64 `json:"temp"`        // 温度（℃）
	Humidity    float64 `json:"hum"`         // 湿度（%RH）
	Pressure    float64 `json:"pressure"`   // 大气压力（hPa）
	Light       float64 `json:"light"`       // 光照强度（lux）
	AirQuality  float64 `json:"air_quality"` // 空气质量指数
	Motion      int     `json:"motion"`      // 人体感应（0/1）
	Rain        int     `json:"rain"`        // 雨滴检测（0/1）
}

// HistoryRecord 历史记录
type HistoryRecord struct {
	Timestamp int64      `json:"timestamp"`
	DeviceID string     `json:"device_id"`
	Data     Data       `json:"data"`
}

// HistoryFile 历史数据文件
type HistoryFile struct {
	Date    string          `json:"date"`
	Records []HistoryRecord `json:"records"`
}

// DeviceStatus 设备状态
type DeviceStatus struct {
	DeviceID     string    `json:"device_id"`
	Online       bool      `json:"online"`
	LastSeen     time.Time `json:"last_seen"`
	TotalRecords int       `json:"total_records"`
}

// AlarmConfig 报警配置
type AlarmConfig struct {
	ID        string  `json:"id"`
	Sensor    string  `json:"sensor"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	Message   string  `json:"message"`
	Enabled   bool    `json:"enabled"`
}

// AlarmConfigFile 报警配置文件
type AlarmConfigFile struct {
	Alarms []AlarmConfig `json:"alarms"`
}

// RGBLEDControl RGB LED控制
type RGBLEDControl struct {
	DeviceID string `json:"device_id"`
	Red      int    `json:"red"`
	Green    int    `json:"green"`
	Blue     int    `json:"blue"`
}

// AliCloudData 阿里云IoT平台数据格式
type AliCloudData struct {
	ID      string                 `json:"id"`
	Version string                 `json:"version"`
	Params  map[string]ParamValue `json:"params"`
	Method  string                 `json:"method"`
}

// ParamValue 参数值（阿里云格式）
type ParamValue struct {
	Value interface{} `json:"value"`
	Time  int64       `json:"time,omitempty"`
}
