package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS cho phép trang web ở origin khác (VD Next.js ở :3000) gọi API (ở :8080) từ trình duyệt.
//
// Đây là cơ chế của TRÌNH DUYỆT: trước một request "không đơn giản" (có Authorization,
// Content-Type JSON, Idempotency-Key...), trình duyệt gửi một request OPTIONS (preflight) hỏi
// "origin này có được gọi không, với method/header này?". Server trả lời bằng các header
// Access-Control-Allow-*. Không có câu trả lời đúng thì trình duyệt chặn, dù server vẫn chạy bình thường.
//
// Chỉ origin trong danh sách mới được phép; origin lạ không nhận header nào nên trình duyệt chặn.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// Response khác nhau theo Origin → báo cho cache/proxy biết đừng dùng chung response
		c.Header("Vary", "Origin")

		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", strings.Join([]string{"Authorization", "Content-Type", headerIdempotencyKey}, ", "))
			c.Header("Access-Control-Max-Age", "600") // trình duyệt nhớ kết quả preflight 10 phút
		}

		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
