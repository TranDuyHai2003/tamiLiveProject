package http

import (
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
}

func SetupRouter(s Services) *gin.Engine {
	r := gin.New()
	// Không ghi query string vào log vì WebSocket truyền access_token qua query
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipQueryString: true}), gin.Recovery())

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

	// Cần đăng nhập
	authed := v1.Group("", AuthMiddleware(s.Tokens))
	authed.GET("/me", userHandler.Me)

	authed.POST("/gifts/send", giftHandler.SendGift)

	authed.POST("/rooms", roomHandler.CreateRoom)
	authed.GET("/rooms/:id", roomHandler.GetRoom)
	authed.POST("/rooms/:id/join", roomHandler.JoinRoom)
	authed.POST("/rooms/:id/leave", roomHandler.LeaveRoom)
	authed.POST("/rooms/:id/end", roomHandler.EndRoom)
	authed.POST("/rooms/:id/games", gameHandler.CreateSession)

	authed.POST("/games/sessions/:id/join", gameHandler.JoinSession)
	authed.POST("/games/sessions/:id/start", gameHandler.StartSession)
	authed.POST("/games/sessions/:id/end", gameHandler.EndSession)

	authed.GET("/ws/chat/:id", chatHandler.HandleChatWS)

	return r
}
