package handler

import (
	"net/http"
	"sync"

	"sensor-backend/Src/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocketHandler WebSocket 处理器
type WebSocketHandler struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

// NewWebSocketHandler 创建 WebSocket 处理器
func NewWebSocketHandler() *WebSocketHandler {
	h := &WebSocketHandler{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
	go h.run()
	return h
}

// run 处理客户端注册/注销和广播
func (h *WebSocketHandler) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			utils.Logger.Info("WebSocket 客户端连接", zap.Int("total", len(h.clients)))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mu.Unlock()
			utils.Logger.Info("WebSocket 客户端断开", zap.Int("total", len(h.clients)))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					h.unregister <- client
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast 广播消息给所有客户端
func (h *WebSocketHandler) Broadcast(data []byte) {
	select {
	case h.broadcast <- data:
	default:
		utils.Logger.Warn("WebSocket 广播队列已满，丢弃消息")
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		utils.Logger.Error("WebSocket 升级失败", zap.Error(err))
		return
	}

	h.register <- conn

	// 保持连接直到客户端断开
	// 读取客户端消息（可选，用于心跳检测）
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			h.unregister <- conn
			break
		}
	}
}

// ClientCount 获取当前客户端数量
func (h *WebSocketHandler) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
