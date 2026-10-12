package http

import (
	"context"
	"net/http"
	"sync"

	"livestream-backend/api"
	"livestream-backend/internal/auth"
	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
)

type Services struct {
	Tokens   *auth.TokenIssuer
	User     *service.UserService
	Gift     *service.GiftService
	Room     *service.RoomService
	Chat     *service.ChatService
	MiniGame *service.MiniGameService
	// ServeAPIDocs bật /docs (Swagger UI) và /openapi.yaml. Tắt ở production.
	ServeAPIDocs bool
	// CORSAllowedOrigins: origin của web app được gọi API từ trình duyệt.
	CORSAllowedOrigins []string
	// Ready kiểm tra kết nối tới các phụ thuộc (Postgres, Redis) cho /readyz.
	Ready func(context.Context) error
}

var setupValidatorOnce sync.Once

// SetupRouter khai báo mọi route. Mỗi route ở đây PHẢI có trong api/openapi.yaml
// (test TestRoutesMatchOpenAPISpec chặn việc quên cập nhật hợp đồng).
func SetupRouter(s Services) *gin.Engine {
	setupValidatorOnce.Do(useJSONFieldNames)

	r := gin.New()
	r.HandleMethodNotAllowed = true
	// Không ghi query string vào log vì WebSocket truyền access_token qua query
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipQueryString: true}), gin.CustomRecovery(recovery), CORS(s.CORSAllowedOrigins))
	r.NoRoute(notFound)
	r.NoMethod(methodNotAllowed)

	r.GET("/healthz", healthz)
	r.GET("/readyz", readyz(s.Ready))

	if s.ServeAPIDocs {
		r.GET("/openapi.yaml", func(c *gin.Context) { c.Data(http.StatusOK, "application/yaml", api.OpenAPISpec) })
		r.GET("/docs", func(c *gin.Context) { c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(api.DocsHTML)) })
	}

	userHandler := NewUserHandler(s.User)
	giftHandler := NewGiftHandler(s.Gift)
	roomHandler := NewRoomHandler(s.Room)
	chatHandler := NewChatWebSocketHandler(s.Chat, s.Room)
	gameHandler := NewMiniGameHandler(s.MiniGame)

	v1 := r.Group("/api/v1")

	// Public
	v1.POST("/auth/register", userHandler.Register)
	v1.POST("/auth/login", userHandler.Login)
	v1.POST("/games/results", gameHandler.SubmitResult) // xác thực bằng HMAC
	v1.GET("/rooms", roomHandler.ListRooms)

	// Cần đăng nhập
	authed := v1.Group("", AuthMiddleware(s.Tokens))
	authed.GET("/me", userHandler.Me)

	authed.POST("/rooms", roomHandler.CreateRoom)
	authed.GET("/rooms/:id", roomHandler.GetRoom)
	authed.POST("/rooms/:id/join", roomHandler.JoinRoom)
	authed.POST("/rooms/:id/leave", roomHandler.LeaveRoom)
	authed.POST("/rooms/:id/end", roomHandler.EndRoom)
	authed.POST("/rooms/:id/gifts", giftHandler.SendGift)
	authed.POST("/rooms/:id/games", gameHandler.CreateSession)

	authed.POST("/games/sessions/:id/join", gameHandler.JoinSession)
	authed.POST("/games/sessions/:id/start", gameHandler.StartSession)
	authed.POST("/games/sessions/:id/end", gameHandler.EndSession)

	authed.GET("/ws/chat/:id", chatHandler.HandleChatWS)

	return r
}
