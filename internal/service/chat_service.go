package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const maxChatMessageRunes = 500

type ChatMessage struct {
	ID       uuid.UUID `json:"id"`
	RoomID   uuid.UUID `json:"room_id"`
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Content  string    `json:"content"`
	SentAt   time.Time `json:"sent_at"`
}

type ChatService struct {
	queries *db.Queries
	rdb     *redis.Client
}

func NewChatService(pool *pgxpool.Pool, rdb *redis.Client) *ChatService {
	return &ChatService{queries: db.New(pool), rdb: rdb}
}

// SendMessage lưu tin nhắn vào DB rồi broadcast qua Redis Pub/Sub tới mọi server.
// Câu INSERT tự kiểm tra phòng LIVE + người gửi JOINED, nên user bị kick/ban giữa chừng không chat tiếp được.
func (s *ChatService) SendMessage(ctx context.Context, roomID, userID uuid.UUID, username, content string) (ChatMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > maxChatMessageRunes {
		return ChatMessage{}, fmt.Errorf("%w: message must be 1-%d characters", ErrInvalidInput, maxChatMessageRunes)
	}

	row, err := s.queries.CreateChatMessage(ctx, db.CreateChatMessageParams{RoomID: roomID, UserID: userID, Message: content})
	if isNoRows(err) {
		return ChatMessage{}, fmt.Errorf("%w: you cannot chat in this room", ErrForbidden)
	}
	if err != nil {
		return ChatMessage{}, fmt.Errorf("save chat message: %w", err)
	}

	msg := ChatMessage{
		ID:       row.ID,
		RoomID:   roomID,
		UserID:   userID,
		Username: username,
		Content:  content,
		SentAt:   row.CreatedAt,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("marshal chat message: %w", err)
	}
	if err := s.rdb.Publish(ctx, roomChannel(roomID), payload).Err(); err != nil {
		return ChatMessage{}, fmt.Errorf("publish chat message: %w", err)
	}
	return msg, nil
}

// Subscribe lắng nghe kênh chat của một phòng. Người gọi phải Close() PubSub khi xong.
func (s *ChatService) Subscribe(ctx context.Context, roomID uuid.UUID) *redis.PubSub {
	return s.rdb.Subscribe(ctx, roomChannel(roomID))
}

func roomChannel(roomID uuid.UUID) string {
	return "chat:room:" + roomID.String()
}
