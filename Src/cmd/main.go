package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"sensor-backend/Src/internal/alarm"
	"sensor-backend/Src/internal/amqp"
	"sensor-backend/Src/internal/config"
	"sensor-backend/Src/internal/handler"
	"sensor-backend/Src/internal/mqtt"
	"sensor-backend/Src/internal/repository"
	"sensor-backend/Src/internal/service"
	"sensor-backend/Src/pkg/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig("./Data/config/config.yaml")
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 2. 初始化日志
	if err := utils.InitLogger(cfg.Logging.Path, cfg.Logging.Level); err != nil {
		log.Fatalf("日志初始化失败: %v", err)
	}
	defer utils.SyncLogger()

	utils.Logger.Info("=== 传感器数据采集系统启动 ===")

	// 3. 初始化数据存储
	utils.Logger.Info("初始化数据存储...")
	store := repository.NewStorage(cfg.Storage.DataDir)
	utils.Logger.Info("数据存储初始化完成")

	// 4. 启动时清理过期数据
	utils.Logger.Info("检查并清理过期历史数据...", zap.Int("retention_days", cfg.Storage.RetentionDays))
	if err := store.CleanupOldData(cfg.Storage.RetentionDays); err != nil {
		utils.Logger.Error("清理过期数据失败", zap.Error(err))
	} else {
		utils.Logger.Info("过期数据清理完成")
	}

	// 5. 初始化报警引擎
	utils.Logger.Info("初始化报警引擎...")
	alarmEngine := alarm.NewAlarmEngine(store)
	utils.Logger.Info("报警引擎初始化完成")

	// 6. 初始化MQTT客户端（仅用于LED控制，数据通过AMQP接收）
	utils.Logger.Info("创建MQTT客户端...")
	mqttClient := mqtt.NewMQTTClient(cfg.MQTT, store, alarmEngine)
	utils.Logger.Info("MQTT客户端创建完成，尝试连接服务器...")
	if err := mqttClient.Connect(); err != nil {
		utils.Logger.Error("MQTT连接失败，LED控制功能将不可用", zap.Error(err))
	} else {
		utils.Logger.Info("MQTT客户端初始化成功（LED控制）")
	}

	// 7. 初始化AMQP客户端（数据接收）
	utils.Logger.Info("创建AMQP客户端...")
	amqpClient := amqp.NewClient(cfg.AMQP, store, alarmEngine)
	if err := amqpClient.Connect(); err != nil {
		utils.Logger.Error("AMQP连接失败", zap.Error(err))
	} else {
		utils.Logger.Info("AMQP客户端初始化成功（数据接收）")
	}

	// 8. 初始化服务层
	sensorService := service.NewSensorService(store, mqttClient)

	// 9. 初始化HTTP处理器
	sensorHandler := handler.NewSensorHandler(sensorService)
	alarmHandler := handler.NewAlarmHandler(alarmEngine)
	healthHandler := handler.NewHealthHandler(mqttClient, amqpClient, store)

	// 10. 设置Gin路由
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// CORS中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// API路由
	v1 := r.Group("/api/v1")
	{
		// 传感器数据
		v1.GET("/sensors/realtime", sensorHandler.GetRealtimeData)
		v1.GET("/sensors/history", sensorHandler.GetHistoryData)

		// 设备状态
		v1.GET("/devices/status", sensorHandler.GetDeviceStatus)

		// 报警配置
		v1.GET("/alarms", alarmHandler.GetAlarms)
		v1.POST("/alarms", alarmHandler.CreateAlarm)
		v1.PUT("/alarms/:id", alarmHandler.UpdateAlarm)
		v1.DELETE("/alarms/:id", alarmHandler.DeleteAlarm)

		// RGB LED控制
		v1.POST("/led/control", sensorHandler.ControlRGBLED)

		// 健康检查
		v1.GET("/health", healthHandler.HealthCheck)
	}

	// 11. 启动MQTT消息处理
	go mqttClient.Start()

	// 12. 启动HTTP服务器
	addr := ":" + fmt.Sprintf("%d", cfg.Server.Port)
	utils.Logger.Info("HTTP服务器启动", zap.Int("port", cfg.Server.Port))

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := r.Run(addr); err != nil {
			utils.Logger.Error("HTTP服务器异常", zap.Error(err))
		}
	}()

	<-quit
	utils.Logger.Info("系统正在关闭...")

	// 关闭AMQP连接
	amqpClient.Disconnect()

	// 关闭MQTT连接
	mqttClient.Disconnect()

	utils.Logger.Info("=== 系统已关闭 ===")
}
