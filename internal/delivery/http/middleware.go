package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"livestream-backend/internal/auth"
	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const ctxUserIDKey = "auth.user_id"

// AuthMiddleware xác thực JWT từ header "Authorization: Bearer <token>".
// Riêng WebSocket (trình duyệt không gửi được header tùy ý) cho phép truyền qua query ?access_token=.
func AuthMiddleware(tokens *auth.TokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ""
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if c.IsWebsocket() {
			token = c.Query("access_token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing access token"})
			return
		}

		userID, err := tokens.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.Set(ctxUserIDKey, userID)
		c.Next()
	}
}

func currentUserID(c *gin.Context) uuid.UUID {
	return c.MustGet(ctxUserIDKey).(uuid.UUID)
}

// pathUUID đọc tham số UUID trên URL; tự trả 400 và trả về false nếu sai định dạng.
func pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": name + " must be a valid UUID"})
		return uuid.Nil, false
	}
	return id, true
}

// writeError map lỗi nghiệp vụ sang HTTP status. Lỗi không xác định chỉ được log,
// client nhận thông báo chung để không lộ chi tiết nội bộ (câu SQL, tên bảng...).
func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, service.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, service.ErrInsufficientBalance):
		status = http.StatusPaymentRequired
	case errors.Is(err, service.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, service.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, service.ErrConflict):
		status = http.StatusConflict
	}

	if status == http.StatusInternalServerError {
		log.Printf("internal error on %s %s: %v", c.Request.Method, c.FullPath(), err)
		c.JSON(status, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
