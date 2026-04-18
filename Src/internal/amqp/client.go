package amqp

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"sensor-backend/Src/internal/alarm"
	"sensor-backend/Src/internal/config"
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/internal/tsdb"
	"sensor-backend/Src/pkg/utils"

	amqp "pack.ag/amqp"
	"go.uber.org/zap"
)

// Client AMQP客户端（官方SDK版本）
type Client struct {
	config      config.AMQPConfig
	store       *repository.Storage
	tsdbClient  *tsdb.Client
	alarmEngine *alarm.AlarmEngine
	wsHandler   interface{ Broadcast([]byte) }
	connected   bool
	mu          sync.RWMutex
	reconnectCount int32
	cancel      context.CancelFunc
}

// AmqpManager 官方示例结构
type AmqpManager struct {
	address     string
	userName    string
	password    string
	client      *amqp.Client
	session     *amqp.Session
	receiver    *amqp.Receiver
	store       *repository.Storage
	alarmEngine *alarm.AlarmEngine
	wsHandler   interface{ Broadcast([]byte) }
	tsdbClient  *tsdb.Client
}

// NewClient 创建AMQP客户端
func NewClient(cfg config.AMQPConfig, store *repository.Storage, alarmEngine *alarm.AlarmEngine, tsdbCli ...*tsdb.Client) *Client {
	var tc *tsdb.Client
	if len(tsdbCli) > 0 {
		tc = tsdbCli[0]
	}
	return &Client{
		config:      cfg,
		store:       store,
		tsdbClient:  tc,
		alarmEngine: alarmEngine,
		connected:   false,
	}
}

// SetWebSocketHandler 设置 WebSocket 处理器
func (c *Client) SetWebSocketHandler(handler interface{ Broadcast([]byte) }) {
	c.wsHandler = handler
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

		// 创建官方SDK的AMQP管理器
		amqpMgr := &AmqpManager{
			store:       c.store,
			alarmEngine: c.alarmEngine,
			wsHandler:   c.wsHandler,
			tsdbClient:  c.tsdbClient,
		}

		// 设置连接参数
		amqpMgr.address = fmt.Sprintf("amqps://%s:5671", c.config.Host)

		// 生成用户名（与官方示例一致）
		timestamp := time.Now().UnixMilli()
		clientID := fmt.Sprintf("client-%d", timestamp)
		amqpMgr.userName = fmt.Sprintf("%s|authMode=aksign,signMethod=hmacsha1,consumerGroupId=%s,authId=%s,iotInstanceId=%s,timestamp=%d|",
			clientID,
			c.config.ConsumerGroupID,
			c.config.AccessKeyID,
			c.config.IOTInstanceID,
			timestamp)

		// 生成密码（与官方示例一致）
		stringToSign := fmt.Sprintf("authId=%s&timestamp=%d", c.config.AccessKeyID, timestamp)
		hmacKey := hmac.New(sha1.New, []byte(c.config.AccessKeySecret))
		hmacKey.Write([]byte(stringToSign))
		amqpMgr.password = base64.StdEncoding.EncodeToString(hmacKey.Sum(nil))

		utils.Logger.Info("正在连接AMQP服务器（官方SDK）",
			zap.String("host", c.config.Host),
			zap.String("consumer_group", c.config.ConsumerGroupID))

		// 开始接收消息
		if err := amqpMgr.startReceiveMessage(ctx, c); err != nil {
			utils.Logger.Error("AMQP接收失败", zap.Error(err))
			backoff := c.getBackoffTime()
			utils.Logger.Info(fmt.Sprintf("等待 %v 后重连...", backoff))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}
	}
}

// startReceiveMessage 官方示例中的消息接收函数
func (am *AmqpManager) startReceiveMessage(ctx context.Context, c *Client) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 建立连接并重试
	if err := am.generateReceiverWithRetry(childCtx); err != nil {
		return err
	}

	c.setConnected(true)
	atomic.StoreInt32(&c.reconnectCount, 0)
	utils.Logger.Info("AMQP连接成功（官方SDK）")

	messageCount := 0

	// 官方示例中的循环接收逻辑
	for {
		// 阻塞接受消息，如果ctx是background则不会被打断
		message, err := am.receiver.Receive(ctx)

		if err != nil {
			utils.Logger.Error("AMQP接收消息出错", zap.Error(err))

			select {
			case <-childCtx.Done():
				c.setConnected(false)
				return amqp.ErrConnClosed
			default:
			}

			// 非主动取消，重新建立连接
			err := am.generateReceiverWithRetry(childCtx)
			if err != nil {
				c.setConnected(false)
				return err
			}
			continue
		}

		messageCount++
		utils.Logger.Info("收到消息",
			zap.Int("message_count", messageCount),
			zap.String("time", time.Now().Format("15:04:05.000")))

		// 在goroutine中处理消息（与官方示例一致）
		go am.processMessage(message)

		// 确认消息已被处理（关键！与官方示例一致）
		if err := message.Accept(); err != nil {
			utils.Logger.Warn("确认消息失败", zap.Error(err))
		}
	}
}

// processMessage 官方示例中的消息处理函数
func (am *AmqpManager) processMessage(message *amqp.Message) {
	// pack.ag/amqp 的 Data 字段是 [][]byte，需要合并
	var data []byte
	for _, b := range message.Data {
		data = append(data, b...)
	}
	if len(data) == 0 {
		utils.Logger.Warn("收到空消息")
		return
	}

	utils.Logger.Info("处理AMQP消息",
		zap.String("data", string(data)))

	// 解析消息
	var msgData struct {
		DeviceName string                 `json:"deviceName"`
		ProductKey string                 `json:"productKey"`
		Timestamp int64                  `json:"timestamp"`
		Items     map[string]interface{} `json:"items"`
	}

	if err := json.Unmarshal(data, &msgData); err != nil {
		utils.Logger.Error("解析AMQP消息失败", zap.Error(err))
		return
	}

	deviceID := msgData.DeviceName
	if deviceID == "" {
		deviceID = "STM32-EnvMonitor-001"
	}

	timestamp := msgData.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().UnixMilli() / 1000
	}

	sensorData := &model.SensorData{
		DeviceID:  deviceID,
		Timestamp: timestamp,
		Data:      parseItemsToData(msgData.Items),
	}

	if !validateSensorData(sensorData) {
		utils.Logger.Warn("传感器数据验证失败")
		return
	}

	// 更新实时数据
	am.store.UpdateRealtime(sensorData)
	utils.Logger.Info("实时数据已更新",
		zap.String("device_id", deviceID),
		zap.Float64("temp", sensorData.Data.Temperature),
		zap.Float64("hum", sensorData.Data.Humidity))

	// 保存历史数据
	if err := am.store.SaveHistory(sensorData); err != nil {
		utils.Logger.Error("保存历史数据失败", zap.Error(err))
	}

		// 保存到 TimescaleDB
		if am.tsdbClient != nil {
			if err := am.tsdbClient.InsertSensorData(sensorData); err != nil {
				utils.Logger.Error("写入TimescaleDB失败", zap.Error(err))
			}
		}

	// 检查报警
	am.alarmEngine.CheckAlarms(sensorData)

	// 通过 WebSocket 广播给前端
	if am.wsHandler != nil {
		sensorJSON, _ := json.Marshal(sensorData)
		am.wsHandler.Broadcast(sensorJSON)
	}
}

// generateReceiverWithRetry 官方示例中的连接重试函数
func (am *AmqpManager) generateReceiverWithRetry(ctx context.Context) error {
	duration := 10 * time.Millisecond
	maxDuration := 20000 * time.Millisecond
	times := 1

	// 异常情况，退避重连
	for {
		select {
		case <-ctx.Done():
			return amqp.ErrConnClosed
		default:
		}

		err := am.generateReceiver()
		if err != nil {
			utils.Logger.Warn("AMQP连接重试",
				zap.Int("times", times),
				zap.Duration("duration", duration),
				zap.Error(err))

			time.Sleep(duration)
			if duration < maxDuration {
				duration *= 2
			}
			times++
		} else {
			utils.Logger.Info("AMQP连接初始化成功")
			return nil
		}
	}
}

// generateReceiver 官方示例中的建立连接函数
func (am *AmqpManager) generateReceiver() error {
	// 清理上一个连接
	if am.client != nil {
		am.client.Close()
	}

	// 建立连接
	client, err := amqp.Dial(am.address, amqp.ConnSASLPlain(am.userName, am.password))
	if err != nil {
		return fmt.Errorf("AMQP拨号失败: %w", err)
	}
	am.client = client

	// 创建会话
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("AMQP创建会话失败: %w", err)
	}
	am.session = session

	// 创建接收者 - 地址格式与官方示例一致
	receiverAddress := fmt.Sprintf("/%s/%s", "iot-06z00i44a0e6h4j", "VNDTFVa24p2nyTp1yHE8000100")
	receiver, err := session.NewReceiver(
		amqp.LinkSourceAddress(receiverAddress),
		amqp.LinkCredit(20), // 与官方示例一致
	)
	if err != nil {
		return fmt.Errorf("AMQP创建接收者失败: %w", err)
	}
	am.receiver = receiver

	return nil
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

	if d.Temperature != 0 && (d.Temperature < -40 || d.Temperature > 80) {
		return false
	}
	if d.Humidity != 0 && (d.Humidity < 0 || d.Humidity > 100) {
		return false
	}
	if d.Pressure != 0 && (d.Pressure < 0 || d.Pressure > 1100) {
		return false
	}
	if d.Light != 0 && (d.Light < 0 || d.Light > 65535) {
		return false
	}
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
