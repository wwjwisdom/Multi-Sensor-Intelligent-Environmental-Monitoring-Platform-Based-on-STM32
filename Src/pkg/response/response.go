package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 错误码定义
const (
	CodeSuccess         = 0
	CodeInvalidParam    = 10001
	CodeNotFound        = 10002
	CodeAlreadyExists   = 10003
	CodeInternalError   = 20001
	CodeMQTTError       = 20002
	CodeStorageError    = 20003
)

// 错误消息映射
var errorMessages = map[int]string{
	CodeSuccess:       "success",
	CodeInvalidParam:  "参数错误",
	CodeNotFound:      "数据未找到",
	CodeAlreadyExists: "数据已存在",
	CodeInternalError: "服务器内部错误",
	CodeMQTTError:     "MQTT连接失败",
	CodeStorageError:  "数据存储失败",
}

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Success 返回成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	})
}

// Error 返回错误响应
func Error(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// InvalidParam 参数错误
func InvalidParam(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, CodeInvalidParam, message)
}

// NotFound 未找到
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, CodeNotFound, message)
}

// AlreadyExists 已存在
func AlreadyExists(c *gin.Context, message string) {
	Error(c, http.StatusConflict, CodeAlreadyExists, message)
}

// InternalError 服务器错误
func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, CodeInternalError, message)
}

// MQTTError MQTT错误
func MQTTError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, CodeMQTTError, message)
}

// StorageError 存储错误
func StorageError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, CodeStorageError, message)
}
