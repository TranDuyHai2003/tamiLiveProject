package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// healthz (liveness): tiến trình còn sống và trả lời được không. Không kiểm tra DB:
// DB chết thì khởi động lại server cũng không giúp gì, nên không được làm server bị khởi động lại.
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readyz (readiness): server đã sẵn sàng nhận request chưa (kết nối được Postgres, Redis...).
// Load balancer chỉ gửi traffic tới instance trả 200.
func readyz(ready func(context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				log.Printf("readiness check failed: %v", err)
				abortWithError(c, http.StatusServiceUnavailable, errorDetail{Code: CodeNotReady, Message: "dependencies are not ready"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
