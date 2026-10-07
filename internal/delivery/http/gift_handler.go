package http

import (
	"net/http"

	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GiftHandler struct {
	giftService *service.GiftService
}

func NewGiftHandler(giftService *service.GiftService) *GiftHandler {
	return &GiftHandler{giftService: giftService}
}

// Người gửi luôn là user đang đăng nhập (lấy từ JWT), không nhận sender_id từ body.
type SendGiftRequest struct {
	RoomID         uuid.UUID  `json:"room_id" binding:"required"`
	GiftID         uuid.UUID  `json:"gift_id" binding:"required"`
	ReceiverID     *uuid.UUID `json:"receiver_id"`
	Quantity       int64      `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string     `json:"idempotency_key" binding:"required,max=255"`
}

func (h *GiftHandler) SendGift(c *gin.Context) {
	var req SendGiftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.giftService.SendGift(c.Request.Context(), service.SendGiftInput{
		SenderID:       currentUserID(c),
		RoomID:         req.RoomID,
		GiftID:         req.GiftID,
		ReceiverID:     req.ReceiverID,
		Quantity:       req.Quantity,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
