package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"livestream-backend/internal/auth"
	"livestream-backend/internal/config"
	database "livestream-backend/internal/db"
	httpdelivery "livestream-backend/internal/delivery/http"
	"livestream-backend/internal/pkg"
	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// ctx bị hủy khi nhận Ctrl+C / SIGTERM để tắt server êm
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1. PostgreSQL
	pool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	// 2. Redis (Chat Pub/Sub)
	rdb, err := pkg.NewRedisClient(ctx, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	// 3. Services
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	tokens := auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTTTL)
	router := httpdelivery.SetupRouter(httpdelivery.Services{
		Tokens:   tokens,
		User:     service.NewUserService(pool, tokens),
		Gift:     service.NewGiftService(pool, cfg.GiftGemRatePercent),
		Room:     service.NewRoomService(pool, cfg.LiveKitAPIKey, cfg.LiveKitAPISecret),
		Chat:     service.NewChatService(pool, rdb),
		MiniGame: service.NewMiniGameService(pool, cfg.MiniGameHMACSecret),
		// Swagger UI tại /docs, tắt ở production
		ServeAPIDocs: !cfg.IsProduction(),
	})

	// 4. HTTP server
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("Server is running on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
	}
}
