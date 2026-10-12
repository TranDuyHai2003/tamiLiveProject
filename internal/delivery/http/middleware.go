package http

import (
	"errors"
	"net/http"
	"strings"

	"livestream-backend/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const ctxUserIDKey = "auth.user_id"

// AuthMiddleware xác thực JWT từ header "Authorization: Bearer <token>".
// Riêng WebSocket (trình duyệt không gửi được header tùy ý) cho phép truyền qua query ?access_token=.
//
// Ba mã lỗi để client phản ứng khác nhau:
//   - AUTH_REQUIRED: không gửi token → về màn đăng nhập
//   - TOKEN_EXPIRED: token hết hạn → làm mới token (bước 8) rồi gọi lại
//   - TOKEN_INVALID: token sai hoặc bị sửa → về màn đăng nhập
func AuthMiddleware(tokens *auth.TokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ""
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if c.IsWebsocket() {
			token = c.Query("access_token")
		}
		if token == "" {
			abortWithError(c, http.StatusUnauthorized, errorDetail{Code: CodeAuthRequired, Message: "missing access token"})
			return
		}

		userID, err := tokens.Parse(token)
		if errors.Is(err, auth.ErrTokenExpired) {
			abortWithError(c, http.StatusUnauthorized, errorDetail{Code: CodeTokenExpired, Message: "access token expired"})
			return
		}
		if err != nil {
			abortWithError(c, http.StatusUnauthorized, errorDetail{Code: CodeTokenInvalid, Message: "invalid access token"})
			return
		}
		c.Set(ctxUserIDKey, userID)
		c.Next()
	}
}

func currentUserID(c *gin.Context) uuid.UUID {
	return c.MustGet(ctxUserIDKey).(uuid.UUID)
}

// pathUUID đọc tham số UUID trên URL; sai định dạng thì tự trả 400 MALFORMED_REQUEST và trả về false.
func pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		abortWithError(c, http.StatusBadRequest, errorDetail{Code: CodeMalformedRequest, Message: name + " must be a valid UUID"})
		return uuid.Nil, false
	}
	return id, true
}
