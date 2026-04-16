package amqp

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"sensor-backend/Src/internal/alarm"
	"sensor-backend/Src/internal/config"
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/pkg/utils"

	amqp "github.com/Azure/go-amqp"
	"go.uber.org/zap"
)

// Client AMQP客户端
type Client struct {
	config      config.AMQPConfig
	store       *repository.Storage
	alarmEngine *alarm.AlarmEngine
	connected   bool
	mu          sync.RWMutex
	reconnectCount int32
	cancel      context.CancelFunc
}

// NewClient 创建AMQP客户端
func NewClient(cfg config.AMQPConfig, store *repository.Storage, alarmEngine *alarm.AlarmEngine) *Client {
	return &Client{
		config:      cfg,
		store:       store,
		alarmEngine: alarmEngine,
		connected:   false,
	}
}

// Connect 启动AMQP连接和消费循环（非阻塞）
func (c *Client) Connect() error {
	if !c.config.Enabled {
		utils.Logger.Info("AMQP已禁用，跳过连接")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	go c.run(ctx)
	utils.Logger.Info("AMQP客户端已启动")
	return nil
}

func (c *Client) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			utils.Logger.Info("AMQP客户端退出")
			c.setConnected(false)
			return
		default:
		}

		conn, session, err := c.connectAMQP(ctx)
		if err != nil {
			utils.Logger.Error("AMQP连接失败", zap.Error(err))
			backoff := c.getBackoffTime()
			utils.Logger.Info(fmt.Sprintf("等待 %v 后重连...", backoff))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}

		c.setConnected(true)
		atomic.StoreInt32(&c.reconnectCount, 0)
		utils.Logger.Info("AMQP连接成功")

		err = c.consumeMessages(ctx, session, conn)
		if ctx.Err() != nil {
			c.setConnected(false)
			return
		}

		if err != nil {
			c.setConnected(false)
			utils.Logger.Warn("消息消费中断，准备重连", zap.Error(err))
		}

		c.cleanup(conn, session)
	}
}

func (c *Client) connectAMQP(ctx context.Context) (*amqp.Conn, *amqp.Session, error) {
	clientID := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())

	authParams := fmt.Sprintf("iotInstanceId=%s,authMode=aksign,signMethod=hmacsha1,consumerGroupId=%s,authId=%s,timestamp=%s",
		c.config.IOTInstanceID,
		c.config.ConsumerGroupID,
		c.config.AccessKeyID,
		timestamp)

	username := fmt.Sprintf("%s|%s|", clientID, authParams)
	stringToSign := fmt.Sprintf("authId=%s&timestamp=%s", c.config.AccessKeyID, timestamp)
	password := signHMACSHA1(stringToSign, c.config.AccessKeySecret)

	u := url.URL{
		Scheme: "amqps",
		Host:   fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		User:   url.UserPassword(username, password),
	}

	opts := &amqp.ConnOptions{
		ContainerID: clientID,
		IdleTimeout: 60 * time.Second,
	}

	conn, err := amqp.Dial(ctx, u.String(), opts)
	if err != nil {
		return nil, nil, fmt.Errorf("拨号失败: %w", err)
	}

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("创建会话失败: %w", err)
	}

	return conn, session, nil
}

func (c *Client) consumeMessages(ctx context.Context, session *amqp.Session, conn *amqp.Conn) error {
	addr := fmt.Sprintf("/%s/%s", c.config.IOTInstanceID, c.config.ConsumerGroupID)

	receiver, err := session.NewReceiver(
		ctx,
		addr,
		&amqp.ReceiverOptions{
			Credit: 10,
		},
	)
	if err != nil {
		return fmt.Errorf("创建接收者失败: %w", err)
	}
	defer receiver.Close(ctx)

	utils.Logger.Info("开始消费AMQP消息...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		recvCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		msg, err := receiver.Receive(recvCtx, nil)
		cancel()

		if err != nil {
			if recvCtx.Err() == context.DeadlineExceeded {
				// 超时是正常的，继续等待
				continue
			}
			if ctx.Err() != nil {
				continue
			}
			utils.Logger.Warn("接收消息出错", zap.Error(err))
			continue
		}

		utils.Logger.Info("收到消息，准备处理",
			zap.Int("data_len", len(msg.Data)),
			zap.String("time", time.Now().Format("15:04:05.000")))
		c.handleMessage(msg)
	}
}

func (c *Client) handleMessage(msg *amqp.Message) {
	topic := ""
	if len(msg.ApplicationProperties) > 0 {
		if t, ok := msg.ApplicationProperties["topic"]; ok {
			if ts, ok := t.(string); ok {
				topic = ts
			}
		}
	}

	utils.Logger.Info("收到AMQP消息", zap.String("topic", topic))

	switch {
	case contains(topic, "thing/event/property/post"):
		c.handlePropertyPost(msg)
	case contains(topic, "thing/status"):
		c.handleStatusChange(msg)
	default:
		utils.Logger.Info("未识别的消息类型", zap.String("topic", topic))
	}
}

func (c *Client) handlePropertyPost(msg *amqp.Message) {
	payload := c.getPayload(msg)
	if payload == nil {
		return
	}

	utils.Logger.Info("原始AMQP消息内容", zap.String("payload", string(payload)))

	var data struct {
		DeviceName string                 `json:"deviceName"`
		ProductKey string                 `json:"productKey"`
		Timestamp  int64                  `json:"timestamp"`
		Items      map[string]interface{} `json:"items"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		utils.Logger.Error("解析设备属性上报失败", zap.Error(err), zap.String("raw_payload", string(payload)))
		return
	}

	deviceID := data.DeviceName
	if deviceID == "" {
		deviceID = "STM32-EnvMonitor-001"
	}

	timestamp := data.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().UnixMilli() / 1000
	}

	sensorData := &model.SensorData{
		DeviceID:  deviceID,
		Timestamp: timestamp,
		Data:      parseItemsToData(data.Items),
	}

	if !validateSensorData(sensorData) {
		utils.Logger.Warn("传感器数据验证失败", zap.Any("data", sensorData))
		return
	}

	// 更新实时数据
	c.store.UpdateRealtime(sensorData)
	utils.Logger.Info("实时数据已更新",
		zap.String("device_id", deviceID),
		zap.Float64("temp", sensorData.Data.Temperature),
		zap.Float64("hum", sensorData.Data.Humidity),
		zap.String("time", time.Now().Format("15:04:05.000")))

	// 保存历史数据
	if err := c.store.SaveHistory(sensorData); err != nil {
		utils.Logger.Error("保存历史数据失败", zap.Error(err))
	} else {
		utils.Logger.Info("成功保存传感器数据",
			zap.String("device_id", sensorData.DeviceID),
			zap.Float64("temperature", sensorData.Data.Temperature),
			zap.Float64("humidity", sensorData.Data.Humidity))
	}

	// 检查报警
	c.alarmEngine.CheckAlarms(sensorData)
}

func (c *Client) handleStatusChange(msg *amqp.Message) {
	payload := c.getPayload(msg)
	if payload == nil {
		return
	}

	var data struct {
		DeviceName string `json:"deviceName"`
		ProductKey string `json:"productKey"`
		Status     string `json:"status"`
		Timestamp  int64  `json:"timestamp"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		utils.Logger.Error("解析设备状态变化失败", zap.Error(err))
		return
	}

	utils.Logger.Info("=== 设备状态变化 ===",
		zap.String("device", fmt.Sprintf("%s/%s", data.ProductKey, data.DeviceName)),
		zap.String("status", data.Status),
		zap.Int64("timestamp", data.Timestamp))
}

func (c *Client) getPayload(msg *amqp.Message) []byte {
	if msg.Data == nil || len(msg.Data) == 0 {
		return nil
	}
	return msg.Data[0]
}

func (c *Client) setConnected(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = v
}

// IsConnected 检查连接状态
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Disconnect 断开连接
func (c *Client) Disconnect() {
	if c.cancel != nil {
		c.cancel()
	}
	c.setConnected(false)
	utils.Logger.Info("AMQP连接已断开")
}

func (c *Client) getBackoffTime() time.Duration {
	count := atomic.AddInt32(&c.reconnectCount, 1) - 1
	backoff := time.Duration(1<<uint(count)) * time.Second
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}
	return backoff
}

func (c *Client) cleanup(conn *amqp.Conn, session *amqp.Session) {
	if session != nil {
		session.Close(context.Background())
	}
	if conn != nil {
		conn.Close()
	}
}

// parseItemsToData 将阿里云 items 格式解析为内部 Data 结构
func parseItemsToData(items map[string]interface{}) model.Data {
	data := model.Data{}
	if items == nil {
		return data
	}

	extractValue := func(key string) (interface{}, bool) {
		v, ok := items[key]
		if !ok {
			return nil, false
		}
		if item, ok := v.(map[string]interface{}); ok {
			if value, ok := item["value"]; ok {
				return value, true
			}
		}
		return v, true
	}

	if v, ok := extractValue("temp"); ok {
		if f, ok := toFloat64(v); ok {
			data.Temperature = f
		}
	}
	if v, ok := extractValue("hum"); ok {
		if f, ok := toFloat64(v); ok {
			data.Humidity = f
		}
	}
	if v, ok := extractValue("pressure"); ok {
		if f, ok := toFloat64(v); ok {
			data.Pressure = f
		}
	}
	if v, ok := extractValue("light"); ok {
		if f, ok := toFloat64(v); ok {
			data.Light = f
		}
	}
	if v, ok := extractValue("air_quality"); ok {
		if f, ok := toFloat64(v); ok {
			data.AirQuality = f
		}
	}
	if v, ok := extractValue("motion"); ok {
		if i, ok := toInt(v); ok {
			data.Motion = i
		}
	}
	if v, ok := extractValue("rain"); ok {
		if i, ok := toInt(v); ok {
			data.Rain = i
		}
	}

	return data
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

	// 大气压力：0~1100 hPa（放宽下限以兼容设备实际值）
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

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func signHMACSHA1(stringToSign, accessKeySecret string) string {
	h := hmac.New(sha1.New, []byte(accessKeySecret))
	h.Write([]byte(stringToSign))
	signature := h.Sum(nil)
	return base64.StdEncoding.EncodeToString(signature)
}
