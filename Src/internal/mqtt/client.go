package mqtt

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"sensor-backend/Src/internal/alarm"
	"sensor-backend/Src/internal/config"
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/pkg/utils"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// MQTTClient MQTT客户端
type MQTTClient struct {
	config      config.MQTTConfig
	client      mqtt.Client
	store       *repository.Storage
	alarmEngine *alarm.AlarmEngine
	connected   bool
	mu          sync.RWMutex
}

// NewMQTTClient 创建MQTT客户端
func NewMQTTClient(cfg config.MQTTConfig, store *repository.Storage, alarmEngine *alarm.AlarmEngine) *MQTTClient {
	return &MQTTClient{
		config:      cfg,
		store:       store,
		alarmEngine: alarmEngine,
		connected:   false,
	}
}

// Connect 连接到MQTT Broker
func (m *MQTTClient) Connect() error {
	broker := fmt.Sprintf("tcp://%s:%d", m.config.Broker, m.config.Port)
	utils.Logger.Info("开始连接MQTT", zap.String("broker", broker))

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(m.config.ClientID)
	opts.SetUsername(m.config.Username)
	opts.SetPassword(m.config.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)

	// 设置连接重试间隔
	reconnectInterval := 5 * time.Second
	if m.config.MaxReconnectInterval > 0 {
		reconnectInterval = m.config.MaxReconnectInterval
	}
	opts.SetConnectRetryInterval(reconnectInterval)

	// 设置心跳包参数
	keepAlive := 60 * time.Second
	if m.config.KeepAlive > 0 {
		keepAlive = m.config.KeepAlive
	}
	opts.SetKeepAlive(keepAlive)

	// 设置Ping超时
	pingTimeout := 10 * time.Second
	if m.config.PingTimeout > 0 {
		pingTimeout = m.config.PingTimeout
	}
	opts.SetPingTimeout(pingTimeout)

	// 设置消息清理
	opts.SetCleanSession(true)

	opts.SetDefaultPublishHandler(m.messageHandler)
	opts.SetOnConnectHandler(m.onConnect)
	opts.SetConnectionLostHandler(m.onConnectionLost)

	client := mqtt.NewClient(opts)
	m.client = client

	utils.Logger.Info("开始异步连接MQTT...")

	// 异步连接，避免阻塞主程序
	go func() {
		defer func() {
			if r := recover(); r != nil {
				utils.Logger.Error("MQTT连接goroutine异常", zap.Any("panic", r))
			}
		}()

		utils.Logger.Info("MQTT连接goroutine已启动")
		token := client.Connect()
		utils.Logger.Info("Connect方法已调用，开始等待连接结果...")

		// 使用带超时的等待
		connectionDone := make(chan bool, 1)
		go func() {
			token.Wait()
			connectionDone <- true
		}()

		select {
		case <-connectionDone:
			if token.Error() != nil {
				utils.Logger.Error("MQTT异步连接失败",
					zap.Error(token.Error()),
					zap.String("broker", broker))
			} else {
				utils.Logger.Info("MQTT异步连接成功",
					zap.String("broker", broker),
					zap.String("client_id", m.config.ClientID))
			}
		case <-time.After(15 * time.Second):
			utils.Logger.Error("MQTT连接超时（15秒）",
				zap.String("broker", broker))
		}
	}()

	// 标记为已连接（实际连接状态由MQTT库管理）
	m.mu.Lock()
	m.connected = false // 实际连接状态尚未确定
	m.mu.Unlock()

	utils.Logger.Info("MQTT客户端已启动连接流程")
	return nil
}

// onConnect 连接成功回调
func (m *MQTTClient) onConnect(client mqtt.Client) {
	m.mu.Lock()
	m.connected = true
	m.mu.Unlock()
	utils.Logger.Info("MQTT连接已建立")

	// 订阅规则引擎转发的用户主题
	topics := []string{
		m.config.SubscribeTopic,         // 具体设备的传感器数据
		m.config.SubscribeTopicWildcard, // 所有设备的传感器数据（通配符）
	}

	for _, topic := range topics {
		if topic == "" {
			continue
		}
		token := client.Subscribe(topic, m.config.QoS, m.onMessage)
		if token.Wait() && token.Error() != nil {
			utils.Logger.Error("订阅主题失败", zap.String("topic", topic), zap.Error(token.Error()))
		} else {
			utils.Logger.Info("订阅主题成功", zap.String("topic", topic))
		}
	}
}

// onConnectionLost 连接断开回调
func (m *MQTTClient) onConnectionLost(client mqtt.Client, err error) {
	m.mu.Lock()
	m.connected = false
	m.mu.Unlock()
	utils.Logger.Error("MQTT连接断开",
		zap.Error(err),
		zap.String("reason", err.Error()))
}

// onMessage 消息回调
func (m *MQTTClient) onMessage(client mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	utils.Logger.Info("收到MQTT消息",
		zap.String("topic", topic),
		zap.String("payload", string(payload)),
		zap.Int("length", len(payload)))

	// 根据不同主题处理不同类型的消息
	switch topic {
	case m.config.SubscribeTopic, m.config.SubscribeTopicWildcard:
		// 处理设备数据上报消息
		var aliCloudData model.AliCloudData
		if err := json.Unmarshal(payload, &aliCloudData); err != nil {
			utils.Logger.Error("解析MQTT消息失败", zap.Error(err), zap.String("raw_payload", string(payload)))
			return
		}

		// 转换为内部数据格式
		sensorData := m.parseAliCloudData(&aliCloudData)
		if sensorData == nil {
			utils.Logger.Warn("解析阿里云数据失败或数据为空")
			return
		}

		// 验证数据
		if !m.validateSensorData(sensorData) {
			utils.Logger.Warn("传感器数据验证失败", zap.Any("data", sensorData))
			return
		}

		// 更新实时数据
		m.store.UpdateRealtime(sensorData)

		// 保存历史数据
		if err := m.store.SaveHistory(sensorData); err != nil {
			utils.Logger.Error("保存历史数据失败", zap.Error(err))
		} else {
			utils.Logger.Info("成功保存传感器数据",
				zap.String("device_id", sensorData.DeviceID),
				zap.Float64("temperature", sensorData.Data.Temperature),
				zap.Float64("humidity", sensorData.Data.Humidity))
		}

		// 检查报警
		m.alarmEngine.CheckAlarms(sensorData)

	default:
		utils.Logger.Debug("收到未订阅主题的消息", zap.String("topic", topic))
	}
}

// parseAliCloudData 解析阿里云数据格式
func (m *MQTTClient) parseAliCloudData(aliData *model.AliCloudData) *model.SensorData {
	if aliData.Params == nil {
		return nil
	}

	sensorData := &model.SensorData{
		DeviceID:  "STM32-EnvMonitor-001", // 从配置或消息中获取
		Timestamp: time.Now().Unix(),
		Data:      model.Data{},
	}

	// 解析每个参数
	for key, param := range aliData.Params {
		switch key {
		case "temp":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.Temperature = v
			}
			if param.Time > 0 {
				sensorData.Timestamp = param.Time
			}
		case "hum":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.Humidity = v
			}
		case "pressure":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.Pressure = v
			}
		case "light":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.Light = v
			}
		case "air_quality":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.AirQuality = v
			}
		case "motion":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.Motion = int(v)
			} else if v, ok := param.Value.(int); ok {
				sensorData.Data.Motion = v
			}
		case "rain":
			if v, ok := param.Value.(float64); ok {
				sensorData.Data.Rain = int(v)
			} else if v, ok := param.Value.(int); ok {
				sensorData.Data.Rain = v
			}
		}
	}

	return sensorData
}

// validateSensorData 验证传感器数据
func (m *MQTTClient) validateSensorData(data *model.SensorData) bool {
	d := data.Data

	// 温度：-40~80℃
	if d.Temperature < -40 || d.Temperature > 80 {
		utils.Logger.Warn("温度数据超出范围", zap.Float64("value", d.Temperature))
		return false
	}

	// 湿度：0~100%RH
	if d.Humidity < 0 || d.Humidity > 100 {
		utils.Logger.Warn("湿度数据超出范围", zap.Float64("value", d.Humidity))
		return false
	}

	// 大气压力：300~1100 hPa
	if d.Pressure < 300 || d.Pressure > 1100 {
		utils.Logger.Warn("大气压力数据超出范围", zap.Float64("value", d.Pressure))
		return false
	}

	// 光照：0~65535 lux
	if d.Light < 0 || d.Light > 65535 {
		utils.Logger.Warn("光照数据超出范围", zap.Float64("value", d.Light))
		return false
	}

	// 空气质量：0~500 AQI
	if d.AirQuality < 0 || d.AirQuality > 500 {
		utils.Logger.Warn("空气质量数据超出范围", zap.Float64("value", d.AirQuality))
		return false
	}

	return true
}

// messageHandler 默认消息处理器
func (m *MQTTClient) messageHandler(client mqtt.Client, msg mqtt.Message) {
	utils.Logger.Debug("默认消息处理", zap.String("topic", msg.Topic()))
}

// Start 启动MQTT客户端
func (m *MQTTClient) Start() {
	utils.Logger.Info("MQTT客户端已启动")
	// 保持运行，等待消息
	for {
		time.Sleep(1 * time.Second)
	}
}

// Disconnect 断开连接
func (m *MQTTClient) Disconnect() {
	if m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(5000)
		utils.Logger.Info("MQTT连接已断开")
	}
}

// IsConnected 检查连接状态
func (m *MQTTClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected && m.client != nil && m.client.IsConnected()
}

// PublishControl 发布控制指令
func (m *MQTTClient) PublishControl(deviceID string, red, green, blue int) error {
	if !m.IsConnected() {
		return fmt.Errorf("MQTT未连接")
	}

	// 构造阿里云物模型格式的控制指令
	params := map[string]interface{}{
		"rgb_red":   red,
		"rgb_green": green,
		"rgb_blue":  blue,
	}

	payload := map[string]interface{}{
		"id":      fmt.Sprintf("%d", time.Now().UnixNano()),
		"version": "1.0",
		"params":  params,
		"method":  "thing.event.property.set",
	}

	content, _ := json.Marshal(payload)

	token := m.client.Publish(m.config.PublishTopic, m.config.QoS, false, content)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	utils.Logger.Info("发布控制指令成功",
		zap.String("device_id", deviceID),
		zap.Int("red", red),
		zap.Int("green", green),
		zap.Int("blue", blue))

	return nil
}
