package http

import (
	"net/http"

	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
)

type RoomHandler struct {
	roomService *service.RoomService
}

func NewRoomHandler(roomService *service.RoomService) *RoomHandler {
	return &RoomHandler{roomService: roomService}
}

// Host luôn là user đang đăng nhập, không nhận host_id từ body.
type CreateRoomRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func (h *RoomHandler) CreateRoom(c *gin.Context) {
	var req CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	room, err := h.roomService.CreateRoom(c.Request.Context(), currentUserID(c), req.Title, req.Description, req.Category)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": room})
}

func (h *RoomHandler) GetRoom(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	room, err := h.roomService.GetRoom(c.Request.Context(), roomID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": room})
}

// JoinRoom thay cho endpoint /rooms/token cũ: quyền publish do server tính từ role, không còn cờ is_host từ client.
func (h *RoomHandler) JoinRoom(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.roomService.JoinRoom(c.Request.Context(), roomID, currentUserID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *RoomHandler) LeaveRoom(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.roomService.LeaveRoom(c.Request.Context(), roomID, currentUserID(c)); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *RoomHandler) EndRoom(c *gin.Context) {
	roomID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	room, err := h.roomService.EndRoom(c.Request.Context(), roomID, currentUserID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": room})
}
