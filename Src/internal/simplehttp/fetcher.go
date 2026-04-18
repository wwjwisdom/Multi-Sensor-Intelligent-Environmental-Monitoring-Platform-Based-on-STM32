package simplehttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/pkg/utils"

	"go.uber.org/zap"
)

// SimpleHTTPFetcher 简单的HTTP获取器
type SimpleHTTPFetcher struct {
	deviceName  string
	productKey  string
	accessKeyID string
	accessKeySecret string
	region      string
	client      *http.Client
}

// DeviceDataResponse 阿里云IoT设备数据响应
type DeviceDataResponse struct {
	Code    int `json:"code"`
	Data    struct {
		DeviceName string                 `json:"deviceName"`
		ProductKey string                 `json:"productKey"`
		Time       string                 `json:"time"`
		Properties map[string]interface{} `json:"properties"`
	} `json:"data"`
	Message string `json:"message"`
}

// NewSimpleHTTPFetcher 创建简单的HTTP获取器
func NewSimpleHTTPFetcher(deviceName, productKey, accessKeyID, accessKeySecret, region string) *SimpleHTTPFetcher {
	return &SimpleHTTPFetcher{
		deviceName:       deviceName,
		productKey:       productKey,
		accessKeyID:      accessKeyID,
		accessKeySecret:  accessKeySecret,
		region:          region,
		client:          &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchDeviceData 获取设备数据（简化版本，先返回模拟数据进行测试）
func (f *SimpleHTTPFetcher) FetchDeviceData() (*model.SensorData, error) {
	utils.Logger.Info("尝试获取设备数据", zap.String("device", f.deviceName))

	// 暂时返回模拟数据进行测试
	sensorData := &model.SensorData{
		DeviceID:  f.deviceName,
		Timestamp: time.Now().Unix(),
		Data: model.Data{
			Temperature: 26.0 + (float64(time.Now().Unix()%30) / 10.0), // 26.0-29.0之间波动
			Humidity:    40.0 + (float64(time.Now().Unix()%20) / 10.0),  // 40.0-42.0之间波动
			Pressure:    1000.0 + (float64(time.Now().Unix()%50) / 10.0), // 1000.0-1005.0之间波动
			Light:       float64(1000 + time.Now().Unix()%500),             // 1000-1500之间波动
			AirQuality:  10.0 + (float64(time.Now().Unix()%10) / 10.0),   // 10.0-11.0之间波动
			Motion:      int(time.Now().Unix() % 2),                      // 0或1
			Rain:        int(time.Now().Unix() % 2),                       // 0或1
		},
	}

	utils.Logger.Info("生成模拟数据",
		zap.String("device_id", sensorData.DeviceID),
		zap.Float64("temp", sensorData.Data.Temperature),
		zap.Float64("hum", sensorData.Data.Humidity))

	return sensorData, nil
}

// FetchFromAliyunAPI 从阿里云API获取真实数据（待实现）
func (f *SimpleHTTPFetcher) FetchFromAliyunAPI() (*model.SensorData, error) {
	// 阿里云IoT API URL
	apiURL := fmt.Sprintf("https://iot-%s.%s.aliyuncs.com/thing/property/get",
		"iot-06z00i44a0e6h4j", f.region)

	// 创建HTTP请求
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置查询参数
	q := req.URL.Query()
	q.Set("ProductKey", f.productKey)
	q.Set("DeviceName", f.deviceName)
	req.URL.RawQuery = q.Encode()

	// 发送请求
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var result DeviceDataResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("API返回错误: %s", result.Message)
	}

	// 转换为SensorData
	sensorData := &model.SensorData{
		DeviceID:  result.Data.DeviceName,
		Timestamp: time.Now().Unix(),
		Data: model.Data{
			Temperature: getFloatProperty(result.Data.Properties, "temp"),
			Humidity:    getFloatProperty(result.Data.Properties, "hum"),
			Pressure:    getFloatProperty(result.Data.Properties, "pressure"),
			Light:       getFloatProperty(result.Data.Properties, "light"),
			AirQuality:  getFloatProperty(result.Data.Properties, "air_quality"),
			Motion:      getIntProperty(result.Data.Properties, "motion"),
			Rain:        getIntProperty(result.Data.Properties, "rain"),
		},
	}

	return sensorData, nil
}

func getFloatProperty(props map[string]interface{}, key string) float64 {
	if val, ok := props[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

func getIntProperty(props map[string]interface{}, key string) int {
	if val, ok := props[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case bool:
			if v {
				return 1
			}
			return 0
		}
	}
	return 0
}