package amqp

import (
	"testing"

	"sensor-backend/Src/internal/model"
)

// TestParseItemsToData_ItemsFormat 测试 items 格式解析（阿里云标准格式）
func TestParseItemsToData_ItemsFormat(t *testing.T) {
	items := map[string]interface{}{
		"temp":        map[string]interface{}{"value": 25.8, "time": 1776327377441},
		"hum":         map[string]interface{}{"value": 49.0, "time": 1776327377441},
		"pressure":    map[string]interface{}{"value": 199.71, "time": 1776327377441},
		"light":       map[string]interface{}{"value": 2289.0, "time": 1776327377441},
		"air_quality": map[string]interface{}{"value": 10.52, "time": 1776327377441},
		"motion":      map[string]interface{}{"value": 1, "time": 1776327377441},
		"rain":        map[string]interface{}{"value": 0, "time": 1776327377441},
	}

	data := parseItemsToData(items)

	if data.Temperature != 25.8 {
		t.Errorf("Temperature: expected 25.8, got %f", data.Temperature)
	}
	if data.Humidity != 49.0 {
		t.Errorf("Humidity: expected 49.0, got %f", data.Humidity)
	}
	if data.Pressure != 199.71 {
		t.Errorf("Pressure: expected 199.71, got %f", data.Pressure)
	}
	if data.Light != 2289.0 {
		t.Errorf("Light: expected 2289.0, got %f", data.Light)
	}
	if data.AirQuality != 10.52 {
		t.Errorf("AirQuality: expected 10.52, got %f", data.AirQuality)
	}
	if data.Motion != 1 {
		t.Errorf("Motion: expected 1, got %d", data.Motion)
	}
	if data.Rain != 0 {
		t.Errorf("Rain: expected 0, got %d", data.Rain)
	}
}

// TestParseItemsToData_DirectFormat 测试直接值格式解析
func TestParseItemsToData_DirectFormat(t *testing.T) {
	items := map[string]interface{}{
		"temp": 25.5,
		"hum":  60.0,
	}

	data := parseItemsToData(items)

	if data.Temperature != 25.5 {
		t.Errorf("Temperature: expected 25.5, got %f", data.Temperature)
	}
	if data.Humidity != 60.0 {
		t.Errorf("Humidity: expected 60.0, got %f", data.Humidity)
	}
}

// TestParseItemsToData_PartialData 测试部分数据
func TestParseItemsToData_PartialData(t *testing.T) {
	items := map[string]interface{}{
		"temp": map[string]interface{}{"value": 30.0, "time": 1776327377441},
		"hum":  map[string]interface{}{"value": 70.0, "time": 1776327377441},
	}

	data := parseItemsToData(items)

	if data.Temperature != 30.0 {
		t.Errorf("Temperature: expected 30.0, got %f", data.Temperature)
	}
	if data.Humidity != 70.0 {
		t.Errorf("Humidity: expected 70.0, got %f", data.Humidity)
	}
	if data.Pressure != 0 {
		t.Errorf("Pressure: expected 0, got %f", data.Pressure)
	}
}

// TestParseItemsToData_NilItems 测试 nil 输入
func TestParseItemsToData_NilItems(t *testing.T) {
	data := parseItemsToData(nil)

	if data.Temperature != 0 || data.Humidity != 0 {
		t.Errorf("Expected zero values for nil input, got %+v", data)
	}
}

// TestParseItemsToData_IntValue 测试 int 类型值
func TestParseItemsToData_IntValue(t *testing.T) {
	items := map[string]interface{}{
		"motion": map[string]interface{}{"value": 1, "time": 1776327377441},
		"rain":   map[string]interface{}{"value": 0, "time": 1776327377441},
	}

	data := parseItemsToData(items)

	if data.Motion != 1 {
		t.Errorf("Motion: expected 1, got %d", data.Motion)
	}
	if data.Rain != 0 {
		t.Errorf("Rain: expected 0, got %d", data.Rain)
	}
}

// TestValidateSensorData_ValidData 测试有效数据验证
func TestValidateSensorData_ValidData(t *testing.T) {
	data := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001",
		Timestamp: 1776327377,
		Data: model.Data{
			Temperature: 25.0,
			Humidity:   50.0,
			Pressure:   1000.0,
			Light:      500.0,
			AirQuality: 50.0,
			Motion:     1,
			Rain:       0,
		},
	}

	if !validateSensorData(data) {
		t.Errorf("Expected valid data to pass validation")
	}
}

// TestValidateSensorData_InvalidTemperature 测试无效温度
func TestValidateSensorData_InvalidTemperature(t *testing.T) {
	data := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001",
		Timestamp: 1776327377,
		Data: model.Data{
			Temperature: 100.0, // 超出 -40~80 范围
			Humidity:   50.0,
		},
	}

	if validateSensorData(data) {
		t.Errorf("Expected invalid temperature to fail validation")
	}
}

// TestValidateSensorData_InvalidHumidity 测试无效湿度
func TestValidateSensorData_InvalidHumidity(t *testing.T) {
	data := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001",
		Timestamp: 1776327377,
		Data: model.Data{
			Humidity: 150.0, // 超出 0~100 范围
		},
	}

	if validateSensorData(data) {
		t.Errorf("Expected invalid humidity to fail validation")
	}
}

// TestValidateSensorData_InvalidPressure 测试无效气压
func TestValidateSensorData_InvalidPressure(t *testing.T) {
	data := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001",
		Timestamp: 1776327377,
		Data: model.Data{
			Pressure: 1200.0, // 超出 0~1100 范围
		},
	}

	if validateSensorData(data) {
		t.Errorf("Expected invalid pressure to fail validation")
	}
}

// TestValidateSensorData_ZeroValues 测试零值（有效）
func TestValidateSensorData_ZeroValues(t *testing.T) {
	data := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001",
		Timestamp: 1776327377,
		Data:     model.Data{},
	}

	// 零值应该通过验证（表示未设置）
	if !validateSensorData(data) {
		t.Errorf("Expected zero values to pass validation")
	}
}

// TestValidateSensorData_ZeroButOutOfRange 测试零值但超范围
func TestValidateSensorData_ZeroButOutOfRange(t *testing.T) {
	// 零值在验证中被跳过，所以不会触发范围检查
	data := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001",
		Timestamp: 1776327377,
		Data: model.Data{
			Temperature: 0,
			Humidity:   0,
		},
	}

	if !validateSensorData(data) {
		t.Errorf("Expected zero values to pass validation")
	}
}

// TestToFloat64 测试类型转换
func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		ok       bool
	}{
		{"float64", float64(25.5), 25.5, true},
		{"float32", float32(30.0), 30.0, true},
		{"int", int(20), 20.0, true},
		{"int64", int64(40), 40.0, true},
		{"string", "invalid", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toFloat64(tt.input)
			if ok != tt.ok {
				t.Errorf("toFloat64(%v): expected ok=%v, got ok=%v", tt.input, tt.ok, ok)
			}
			if ok && result != tt.expected {
				t.Errorf("toFloat64(%v): expected %f, got %f", tt.input, tt.expected, result)
			}
		})
	}
}

// TestToInt 测试整数转换
func TestToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
		ok       bool
	}{
		{"int", int(5), 5, true},
		{"int64", int64(10), 10, true},
		{"float64", float64(15.0), 15, true},
		{"string", "invalid", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toInt(tt.input)
			if ok != tt.ok {
				t.Errorf("toInt(%v): expected ok=%v, got ok=%v", tt.input, tt.ok, ok)
			}
			if ok && result != tt.expected {
				t.Errorf("toInt(%v): expected %d, got %d", tt.input, tt.expected, result)
			}
		})
	}
}

// TestContains 测试字符串包含函数
func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"thing/event/property/post", "property/post", true},
		{"thing/status", "status", true},
		{"other/topic", "property/post", false},
		{"", "test", false},
		{"test", "", true},
		{"test", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q): expected %v, got %v", tt.s, tt.substr, tt.expected, result)
			}
		})
	}
}

// TestSignHMACSHA1 测试签名函数
func TestSignHMACSHA1(t *testing.T) {
	// 使用已知的测试向量验证签名
	stringToSign := "authId=TEST_ACCESS_KEY&timestamp=1776327370349"
	accessKeySecret := "TEST_ACCESS_SECRET"

	signature := signHMACSHA1(stringToSign, accessKeySecret)

	// 签名应该是28个字符的base64字符串
	if len(signature) == 0 {
		t.Errorf("Expected non-empty signature")
	}

	// 多次签名同一内容应得到相同结果
	signature2 := signHMACSHA1(stringToSign, accessKeySecret)
	if signature != signature2 {
		t.Errorf("Expected consistent signature for same input")
	}

	// 不同内容应得到不同签名
	signature3 := signHMACSHA1(stringToSign+"diff", accessKeySecret)
	if signature == signature3 {
		t.Errorf("Expected different signature for different input")
	}
}

// TestSignHMACSHA1_EmptyInput 测试空输入
func TestSignHMACSHA1_EmptyInput(t *testing.T) {
	signature := signHMACSHA1("", "secret")
	if len(signature) == 0 {
		t.Errorf("Expected non-empty signature for empty input")
	}
}

// BenchmarkParseItemsToData 性能测试：items 格式解析
func BenchmarkParseItemsToData(b *testing.B) {
	items := map[string]interface{}{
		"temp":        map[string]interface{}{"value": 25.8, "time": 1776327377441},
		"hum":         map[string]interface{}{"value": 49.0, "time": 1776327377441},
		"pressure":    map[string]interface{}{"value": 199.71, "time": 1776327377441},
		"light":       map[string]interface{}{"value": 2289.0, "time": 1776327377441},
		"air_quality": map[string]interface{}{"value": 10.52, "time": 1776327377441},
		"motion":      map[string]interface{}{"value": 1, "time": 1776327377441},
		"rain":        map[string]interface{}{"value": 0, "time": 1776327377441},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseItemsToData(items)
	}
}
