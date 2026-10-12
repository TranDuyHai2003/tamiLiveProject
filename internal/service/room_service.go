package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	livekit "github.com/livekit/protocol/auth"
)

const liveKitTokenTTL = 6 * time.Hour

type RoomService struct {
	pool      *pgxpool.Pool
	queries   *db.Queries
	apiKey    string
	apiSecret string
}

func NewRoomService(pool *pgxpool.Pool, apiKey, apiSecret string) *RoomService {
	return &RoomService{
		pool:      pool,
		queries:   db.New(pool),
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

// CreateRoom mở phòng LIVE mới và ghi host vào room_members với role HOST.
func (s *RoomService) CreateRoom(ctx context.Context, hostID uuid.UUID, title, description, category string) (db.Room, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 255 {
		return db.Room{}, invalidField("title", "length", "title must be 1-255 chars")
	}

	var room db.Room
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		var err error
		room, err = q.CreateLiveRoom(ctx, db.CreateLiveRoomParams{
			HostID:          hostID,
			Title:           title,
			Description:     optionalString(description),
			Category:        optionalString(category),
			LivekitRoomName: ptr("room-" + uuid.NewString()),
		})
		if err != nil {
			return err
		}
		_, err = q.JoinRoomMember(ctx, db.JoinRoomMemberParams{RoomID: room.ID, UserID: hostID, Role: db.RoomMemberRoleHOST})
		return err
	})
	if isUniqueViolation(err, "idx_rooms_one_live_per_host") {
		return db.Room{}, newError(ErrConflict, CodeAlreadyLive, "you already have a live room")
	}
	if err != nil {
		return db.Room{}, fmt.Errorf("create room: %w", err)
	}
	return room, nil
}

func (s *RoomService) GetRoom(ctx context.Context, roomID uuid.UUID) (db.Room, error) {
	room, err := s.queries.GetRoom(ctx, roomID)
	if isNoRows(err) {
		return db.Room{}, fmt.Errorf("%w: room", ErrNotFound)
	}
	if err != nil {
		return db.Room{}, fmt.Errorf("get room: %w", err)
	}
	return room, nil
}

type JoinRoomResult struct {
	Room         db.Room           `json:"room"`
	Role         db.RoomMemberRole `json:"role"`
	LiveKitToken string            `json:"livekit_token"`
}

// JoinRoom ghi nhận thành viên và cấp LiveKit token. Quyền publish do server quyết định
// dựa trên role trong phòng, không tin bất kỳ cờ nào client gửi lên.
func (s *RoomService) JoinRoom(ctx context.Context, roomID, userID uuid.UUID) (JoinRoomResult, error) {
	user, err := s.queries.GetUserByID(ctx, userID)
	if isNoRows(err) {
		return JoinRoomResult{}, fmt.Errorf("%w: user", ErrNotFound)
	}
	if err != nil {
		return JoinRoomResult{}, fmt.Errorf("get user: %w", err)
	}
	if user.Status != db.UserStatusACTIVE {
		return JoinRoomResult{}, fmt.Errorf("%w: account is not active", ErrForbidden)
	}

	var result JoinRoomResult
	err = withTx(ctx, s.pool, func(q *db.Queries) error {
		room, err := q.GetRoomForShare(ctx, roomID)
		if isNoRows(err) {
			return fmt.Errorf("%w: room", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("get room: %w", err)
		}
		if room.Status != db.RoomStatusLIVE || room.LivekitRoomName == nil {
			return errRoomNotLive()
		}

		role := db.RoomMemberRoleVIEWER
		if room.HostID == userID {
			role = db.RoomMemberRoleHOST
		}
		member, err := q.JoinRoomMember(ctx, db.JoinRoomMemberParams{RoomID: roomID, UserID: userID, Role: role})
		if isNoRows(err) {
			return newError(ErrForbidden, CodeBannedFromRoom, "you were kicked or banned from this room")
		}
		if err != nil {
			return fmt.Errorf("join room: %w", err)
		}

		canPublish := member.Role == db.RoomMemberRoleHOST || member.Role == db.RoomMemberRoleCOHOST
		token, err := s.liveKitToken(*room.LivekitRoomName, userID.String(), user.Username, canPublish)
		if err != nil {
			return err
		}
		result = JoinRoomResult{Room: room, Role: member.Role, LiveKitToken: token}
		return nil
	})
	if err != nil {
		return JoinRoomResult{}, err
	}
	return result, nil
}

func (s *RoomService) LeaveRoom(ctx context.Context, roomID, userID uuid.UUID) error {
	return s.queries.LeaveRoom(ctx, db.LeaveRoomParams{RoomID: roomID, UserID: userID})
}

// EndRoom kết thúc phòng (chỉ host), đồng thời đóng mọi thứ còn treo trong phòng:
// thành viên, game session đang mở, và trận PK đang diễn ra.
func (s *RoomService) EndRoom(ctx context.Context, roomID, hostID uuid.UUID) (db.Room, error) {
	var room db.Room
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		var err error
		room, err = q.EndRoom(ctx, db.EndRoomParams{ID: roomID, HostID: hostID})
		if isNoRows(err) {
			return explainEndRoomFailure(ctx, q, roomID, hostID)
		}
		if err != nil {
			return fmt.Errorf("end room: %w", err)
		}

		if err := q.LeaveAllRoomMembers(ctx, roomID); err != nil {
			return fmt.Errorf("leave members: %w", err)
		}

		sessionIDs, err := q.CancelOpenGameSessionsByRoom(ctx, roomID)
		if err != nil {
			return fmt.Errorf("cancel game sessions: %w", err)
		}
		for _, sessionID := range sessionIDs {
			if err := q.MarkGamePlayersFinished(ctx, sessionID); err != nil {
				return fmt.Errorf("finish game players: %w", err)
			}
		}

		battleID, err := q.GetOpenPKBattleIDByRoom(ctx, roomID)
		if err == nil {
			// Trận có thể vừa được kết thúc bởi request khác -> ErrConflict, bỏ qua
			if _, err := finishBattleTx(ctx, q, battleID); err != nil && !errors.Is(err, ErrConflict) {
				return err
			}
		} else if !isNoRows(err) {
			return fmt.Errorf("get pk battle: %w", err)
		}
		return nil
	})
	if err != nil {
		return db.Room{}, err
	}
	return room, nil
}

// EnsureCanChat kiểm tra phòng LIVE và user đang JOINED; trả về username để hiển thị.
func (s *RoomService) EnsureCanChat(ctx context.Context, roomID, userID uuid.UUID) (string, error) {
	room, err := s.GetRoom(ctx, roomID)
	if err != nil {
		return "", err
	}
	if room.Status != db.RoomStatusLIVE {
		return "", errRoomNotLive()
	}
	member, err := s.queries.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: roomID, UserID: userID})
	if isNoRows(err) || (err == nil && member.Status != db.RoomMemberStatusJOINED) {
		return "", fmt.Errorf("%w: join the room before chatting", ErrForbidden)
	}
	if err != nil {
		return "", fmt.Errorf("get member: %w", err)
	}
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}
	return user.Username, nil
}

func (s *RoomService) liveKitToken(roomName, identity, name string, canPublish bool) (string, error) {
	grant := &livekit.VideoGrant{RoomJoin: true, Room: roomName}
	grant.SetCanPublish(canPublish)
	// Data message của LiveKit không đi qua backend (không kiểm tra kick/ban/độ dài),
	// nên viewer không được gửi; chat của viewer phải đi qua WebSocket.
	grant.SetCanPublishData(canPublish)
	grant.SetCanSubscribe(true)

	token, err := livekit.NewAccessToken(s.apiKey, s.apiSecret).
		SetIdentity(identity).
		SetName(name).
		SetValidFor(liveKitTokenTTL).
		SetVideoGrant(grant).
		ToJWT()
	if err != nil {
		return "", fmt.Errorf("generate livekit token: %w", err)
	}
	return token, nil
}

func explainEndRoomFailure(ctx context.Context, q *db.Queries, roomID, hostID uuid.UUID) error {
	room, err := q.GetRoom(ctx, roomID)
	if isNoRows(err) {
		return fmt.Errorf("%w: room", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("get room: %w", err)
	}
	if room.HostID != hostID {
		return fmt.Errorf("%w: only the host can end the room", ErrForbidden)
	}
	return errRoomNotLive()
}

func optionalString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
