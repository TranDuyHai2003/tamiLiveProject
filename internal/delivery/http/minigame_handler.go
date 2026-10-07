package http

import (
	"encoding/json"
	"net/http"

	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MiniGameHandler struct {
	miniGameService *service.MiniGameService
}

func NewMiniGameHandler(miniGameService *service.MiniGameService) *MiniGameHandler {
	return &MiniGameHandler{miniGameService: miniGameService}
}

type CreateGameSessionRequest struct {
	GameID     uuid.UUID `json:"game_id" binding:"required"`
	MaxPlayers int32     `json:"max_players"`
}

func (h *MiniGameHandler) CreateSession(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req CreateGameSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := h.miniGameService.CreateSession(c.Request.Context(), currentUserID(c), roomID, req.GameID, req.MaxPlayers)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": session})
}

func (h *MiniGameHandler) JoinSession(c *gin.Context) {
	sessionID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	player, err := h.miniGameService.JoinSession(c.Request.Context(), sessionID, currentUserID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": player})
}

func (h *MiniGameHandler) StartSession(c *gin.Context) {
	sessionID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.miniGameService.StartSession(c.Request.Context(), sessionID, currentUserID(c)); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *MiniGameHandler) EndSession(c *gin.Context) {
	sessionID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.miniGameService.EndSession(c.Request.Context(), sessionID, currentUserID(c)); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Callback server-to-server từ game provider, xác thực bằng chữ ký HMAC thay cho JWT.
type SubmitGameResultRequest struct {
	SessionID  uuid.UUID       `json:"session_id" binding:"required"`
	UserID     uuid.UUID       `json:"user_id" binding:"required"`
	Score      int64           `json:"score" binding:"min=0"`
	RewardCoin int64           `json:"reward_coin" binding:"min=0"`
	Payload    json.RawMessage `json:"payload"`
	Signature  string          `json:"signature" binding:"required"`
}

func (h *MiniGameHandler) SubmitResult(c *gin.Context) {
	var req SubmitGameResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.miniGameService.SubmitResult(c.Request.Context(), service.GameResultInput{
		SessionID:  req.SessionID,
		UserID:     req.UserID,
		Score:      req.Score,
		RewardCoin: req.RewardCoin,
		Payload:    req.Payload,
		Signature:  req.Signature,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
