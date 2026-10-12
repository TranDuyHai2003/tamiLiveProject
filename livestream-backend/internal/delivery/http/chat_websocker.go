package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	wsWriteWait      = 10 * time.Second
	wsPongWait       = 60 * time.Second
	wsPingPeriod     = wsPongWait * 9 / 10
	wsMaxMessageSize = 4096
)

var upgrader = websocket.Upgrader{
	// Xác thực bằng access token chứ không bằng cookie, nên kết nối từ origin khác không mượn được phiên đăng nhập.
	// Khi có domain chính thức nên giới hạn origin tại đây.
	CheckOrigin: func(r *http.Request) bool { return true },
}

type ChatWebSocketHandler struct {
	chatService *service.ChatService
	roomService *service.RoomService
}

func NewChatWebSocketHandler(chatService *service.ChatService, roomService *service.RoomService) *ChatWebSocketHandler {
	return &ChatWebSocketHandler{chatService: chatService, roomService: roomService}
}

// HandleChatWS: GET /api/v1/ws/chat/:id?access_token=...
// Danh tính lấy từ JWT, username lấy từ DB; client không tự khai user_id/username được nữa.
func (h *ChatWebSocketHandler) HandleChatWS(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	userID := currentUserID(c)

	// Kiểm tra quyền TRƯỚC khi upgrade để còn trả được mã lỗi HTTP chuẩn
	username, err := h.roomService.EnsureCanChat(c.Request.Context(), roomID, userID)
	if err != nil {
		writeError(c, err)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// ctx bị hủy khi handler kết thúc -> goroutine ghi và subscription Redis đều dừng theo
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pubsub := h.chatService.Subscribe(ctx, roomID)
	defer pubsub.Close()

	// Goroutine GHI duy nhất (gorilla/websocket không cho nhiều goroutine ghi đồng thời):
	// chuyển tin từ Redis xuống client và gửi ping giữ kết nối.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer cancel()
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		messages := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Vòng ĐỌC chạy ngay trên goroutine của handler; thoát khi client ngắt hoặc quá hạn pong.
	conn.SetReadLimit(wsMaxMessageSize)
	conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var clientMsg struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(data, &clientMsg); err != nil {
			continue
		}
		if _, err := h.chatService.SendMessage(ctx, roomID, userID, username, clientMsg.Content); err != nil {
			if errors.Is(err, service.ErrForbidden) {
				break // bị kick/ban hoặc phòng đã kết thúc
			}
			if !errors.Is(err, service.ErrInvalidInput) {
				log.Printf("send chat message: %v", err)
			}
		}
	}

	cancel()
	<-writerDone
}
