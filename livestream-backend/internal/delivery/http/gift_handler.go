package http

import (
	"net/http"

	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// headerIdempotencyKey: client sinh một UUID cho MỖI lần bấm "tặng" và gửi lại y nguyên khi retry.
// Nằm ở header (theo chuẩn phổ biến, như Stripe) để mọi endpoint ghi tiền dùng chung một cơ chế.
const headerIdempotencyKey = "Idempotency-Key"

type GiftHandler struct {
	giftService *service.GiftService
}

func NewGiftHandler(giftService *service.GiftService) *GiftHandler {
	return &GiftHandler{giftService: giftService}
}

// Người gửi luôn là user đang đăng nhập (lấy từ JWT); phòng lấy từ đường dẫn.
type SendGiftRequest struct {
	GiftID     uuid.UUID  `json:"gift_id" binding:"required"`
	ReceiverID *uuid.UUID `json:"receiver_id"`
	Quantity   int64      `json:"quantity" binding:"required,min=1"`
}

// SendGift: POST /rooms/:id/gifts
func (h *GiftHandler) SendGift(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req SendGiftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBindError(c, err)
		return
	}

	result, err := h.giftService.SendGift(c.Request.Context(), service.SendGiftInput{
		SenderID:       currentUserID(c),
		RoomID:         roomID,
		GiftID:         req.GiftID,
		ReceiverID:     req.ReceiverID,
		Quantity:       req.Quantity,
		IdempotencyKey: c.GetHeader(headerIdempotencyKey),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
