package handler

import (
	"sensor-backend/Src/internal/alarm"
	"sensor-backend/Src/internal/model"
	"sensor-backend/Src/pkg/response"

	"github.com/gin-gonic/gin"
)

// AlarmHandler 报警配置处理器
type AlarmHandler struct {
	engine *alarm.AlarmEngine
}

// NewAlarmHandler 创建处理器
func NewAlarmHandler(engine *alarm.AlarmEngine) *AlarmHandler {
	return &AlarmHandler{engine: engine}
}

// GetAlarms 获取报警配置列表
func (h *AlarmHandler) GetAlarms(c *gin.Context) {
	alarms := h.engine.GetAlarms()
	response.Success(c, gin.H{
		"alarms": alarms,
		"total":  len(alarms),
	})
}

// CreateAlarm 创建报警配置
func (h *AlarmHandler) CreateAlarm(c *gin.Context) {
	var alarm model.AlarmConfig
	if err := c.ShouldBindJSON(&alarm); err != nil {
		response.InvalidParam(c, "参数格式错误")
		return
	}

	// 验证传感器类型
	validSensors := map[string]bool{
		"temp": true, "hum": true, "pressure": true,
		"light": true, "air_quality": true,
		"motion": true, "rain": true,
	}
	if !validSensors[alarm.Sensor] {
		response.InvalidParam(c, "不支持的传感器类型")
		return
	}

	// 验证操作符
	validOperators := map[string]bool{
		">": true, "<": true, ">=": true, "<=": true, "==": true, "!=": true,
	}
	if !validOperators[alarm.Operator] {
		response.InvalidParam(c, "不支持的操作符")
		return
	}

	// 验证消息长度
	if len(alarm.Message) == 0 || len(alarm.Message) > 200 {
		response.InvalidParam(c, "消息长度必须在1-200之间")
		return
	}

	// 设置默认值
	if alarm.Operator == "" {
		alarm.Operator = ">"
	}
	if !alarm.Enabled {
		alarm.Enabled = true
	}

	result, err := h.engine.CreateAlarm(alarm)
	if err != nil {
		response.InternalError(c, "创建报警配置失败")
		return
	}

	response.Success(c, result)
}

// UpdateAlarm 更新报警配置
func (h *AlarmHandler) UpdateAlarm(c *gin.Context) {
	id := c.Param("id")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.InvalidParam(c, "参数格式错误")
		return
	}

	result, err := h.engine.UpdateAlarm(id, updates)
	if err != nil {
		response.NotFound(c, "报警配置不存在")
		return
	}

	response.Success(c, result)
}

// DeleteAlarm 删除报警配置
func (h *AlarmHandler) DeleteAlarm(c *gin.Context) {
	id := c.Param("id")

	if err := h.engine.DeleteAlarm(id); err != nil {
		response.NotFound(c, "报警配置不存在")
		return
	}

	response.Success(c, gin.H{"deleted_id": id})
}
