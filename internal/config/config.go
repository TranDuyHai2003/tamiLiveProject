package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	JWTSecret          string
	JWTTTL             time.Duration
	LiveKitAPIKey      string
	LiveKitAPISecret   string
	MiniGameHMACSecret string
	// Tỷ lệ % giá trị COIN của quà được quy đổi thành GEM cho người nhận (0-100)
	GiftGemRatePercent int64
}

// Load đọc cấu hình từ biến môi trường. Mọi secret đều bắt buộc, không có giá trị mặc định trong code.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      os.Getenv("REDIS_PASSWORD"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		LiveKitAPIKey:      os.Getenv("LIVEKIT_API_KEY"),
		LiveKitAPISecret:   os.Getenv("LIVEKIT_API_SECRET"),
		MiniGameHMACSecret: os.Getenv("MINIGAME_HMAC_SECRET"),
	}

	required := map[string]string{
		"DATABASE_URL":         cfg.DatabaseURL,
		"JWT_SECRET":           cfg.JWTSecret,
		"LIVEKIT_API_KEY":      cfg.LiveKitAPIKey,
		"LIVEKIT_API_SECRET":   cfg.LiveKitAPISecret,
		"MINIGAME_HMAC_SECRET": cfg.MiniGameHMACSecret,
	}
	for name, value := range required {
		if value == "" {
			return Config{}, fmt.Errorf("missing required env %s", name)
		}
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}

	ttl, err := time.ParseDuration(getEnv("JWT_TTL", "24h"))
	if err != nil || ttl <= 0 {
		return Config{}, fmt.Errorf("invalid JWT_TTL")
	}
	cfg.JWTTTL = ttl

	rate, err := strconv.ParseInt(getEnv("GIFT_GEM_RATE_PERCENT", "50"), 10, 64)
	if err != nil || rate < 0 || rate > 100 {
		return Config{}, fmt.Errorf("GIFT_GEM_RATE_PERCENT must be an integer in [0, 100]")
	}
	cfg.GiftGemRatePercent = rate

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
