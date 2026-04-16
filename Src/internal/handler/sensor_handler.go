package handler

import (
	"strconv"
	"time"

	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/internal/service"
	"sensor-backend/Src/pkg/response"

	"github.com/gin-gonic/gin"
)

// SensorHandler 传感器数据处理器
type SensorHandler struct {
	service *service.SensorService
}

// NewSensorHandler 创建处理器
func NewSensorHandler(service *service.SensorService) *SensorHandler {
	return &SensorHandler{service: service}
}

// GetRealtimeData 获取实时数据
// @Summary 获取实时传感器数据
// @Param device_id query string false "设备ID"
// @Success 200 {object} response.Response
// @Router /api/v1/sensors/realtime [get]
func (h *SensorHandler) GetRealtimeData(c *gin.Context) {
	deviceID := c.Query("device_id")

	var result interface{}

	if deviceID != "" {
		data := h.service.GetRealtimeData(deviceID)
		if data == nil {
			response.NotFound(c, "设备不存在或无数据")
			return
		}
		result = data
	} else {
		// 返回所有设备数据
		allData := h.service.GetAllRealtimeData()
		if len(allData) == 0 {
			response.Success(c, gin.H{"devices": []interface{}{}})
			return
		}
		result = gin.H{"devices": allData}
	}

	response.Success(c, result)
}

// GetHistoryData 获取历史数据
// @Summary 获取历史传感器数据
// @Param device_id query string false "设备ID"
// @Param start_time query int true "开始时间(Unix时间戳)"
// @Param end_time query int true "结束时间(Unix时间戳)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} response.Response
// @Router /api/v1/sensors/history [get]
func (h *SensorHandler) GetHistoryData(c *gin.Context) {
	// 解析参数
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	if startTimeStr == "" || endTimeStr == "" {
		response.InvalidParam(c, "start_time和end_time为必需参数")
		return
	}

	startTime, err := strconv.ParseInt(startTimeStr, 10, 64)
	if err != nil {
		response.InvalidParam(c, "start_time格式错误")
		return
	}

	endTime, err := strconv.ParseInt(endTimeStr, 10, 64)
	if err != nil {
		response.InvalidParam(c, "end_time格式错误")
		return
	}

	if startTime >= endTime {
		response.InvalidParam(c, "start_time必须小于end_time")
		return
	}

	// 检查时间范围最大7天
	if endTime-startTime > 7*24*60*60 {
		response.InvalidParam(c, "时间范围最大7天")
		return
	}

	deviceID := c.Query("device_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	records, total, err := h.service.GetHistoryData(deviceID, startTime, endTime, page, pageSize)
	if err != nil {
		response.InternalError(c, "查询历史数据失败")
		return
	}

	response.Success(c, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"records":   records,
	})
}

// GetDeviceStatus 获取设备状态
// @Summary 获取设备状态
// @Success 200 {object} response.Response
// @Router /api/v1/devices/status [get]
func (h *SensorHandler) GetDeviceStatus(c *gin.Context) {
	statuses := h.service.GetDeviceStatus()

	response.Success(c, gin.H{
		"devices":       statuses,
		"total_online":  countOnline(statuses),
		"total_devices": len(statuses),
	})
}

func countOnline(statuses []model.DeviceStatus) int {
	count := 0
	for _, s := range statuses {
		if s.Online {
			count++
		}
	}
	return count
}

// ControlRGBLED 控制RGB LED
// @Summary 控制RGB LED
// @Param body body model.RGBLEDControl true "RGB控制参数"
// @Success 200 {object} response.Response
// @Router /api/v1/led/control [post]
func (h *SensorHandler) ControlRGBLED(c *gin.Context) {
	var control model.RGBLEDControl
	if err := c.ShouldBindJSON(&control); err != nil {
		response.InvalidParam(c, "参数格式错误")
		return
	}

	// 验证RGB值范围
	if control.Red < 0 || control.Red > 255 ||
		control.Green < 0 || control.Green > 255 ||
		control.Blue < 0 || control.Blue > 255 {
		response.InvalidParam(c, "RGB值必须在0-255之间")
		return
	}

	// 检查MQTT连接
	if !h.service.IsMQTTConnected() {
		response.MQTTError(c, "MQTT未连接")
		return
	}

	deviceID := control.DeviceID
	if deviceID == "" {
		deviceID = "STM32-EnvMonitor-001"
	}

	if err := h.service.ControlRGBLED(deviceID, control.Red, control.Green, control.Blue); err != nil {
		response.MQTTError(c, "发送控制指令失败")
		return
	}

	response.Success(c, gin.H{
		"device_id":    deviceID,
		"red":         control.Red,
		"green":       control.Green,
		"blue":        control.Blue,
		"command_sent": true,
		"timestamp":   time.Now().Format(time.RFC3339),
	})
}
