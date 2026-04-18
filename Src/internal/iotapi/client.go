package iotapi

import (
	"context"
	"time"

	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/pkg/utils"

	"go.uber.org/zap"
)

// Client 阿里云IoT HTTP API客户端（模拟数据版本）
type Client struct {
	deviceName  string
	productKey  string
	region      string
}

// NewClient 创建HTTP API客户端
func NewClient(accessKeyID, accessKeySecret, region, instanceID, productKey, deviceName string) *Client {
	return &Client{
		deviceName: deviceName,
		productKey: productKey,
		region:    region,
	}
}

// GetDevicePropertyData 获取设备属性数据（模拟数据版本）
func (c *Client) GetDevicePropertyData(ctx context.Context) (*model.SensorData, error) {
	utils.Logger.Info("获取设备属性数据", zap.String("device", c.deviceName))

	// 生成模拟数��
	sensorData := &model.SensorData{
		DeviceID:  c.deviceName,
		Timestamp: time.Now().Unix(),
		Data: model.Data{
			Temperature: 25.0 + (float64(time.Now().Unix()%40) / 10.0), // 25.0-29.0
			Humidity:    45.0 + (float64(time.Now().Unix()%15) / 10.0), // 45.0-46.5
			Pressure:    1010.0 + (float64(time.Now().Unix()%30) / 10.0), // 1010.0-1013.0
			Light:       float64(2000 + time.Now().Unix()%1000),        // 2000-3000
			AirQuality:  15.0 + (float64(time.Now().Unix()%20) / 10.0),  // 15.0-17.0
			Motion:      int(time.Now().Unix() % 2),                    // 0或1
			Rain:        int(time.Now().Unix() % 2),                     // 0或1
		},
	}

	utils.Logger.Info("生成模拟设备数据",
		zap.String("device_id", sensorData.DeviceID),
		zap.Float64("temp", sensorData.Data.Temperature),
		zap.Float64("hum", sensorData.Data.Humidity),
		zap.String("time", time.Now().Format("15:04:05.000")))

	return sensorData, nil
}