package service

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PKService struct {
	pool *pgxpool.Pool
}

func NewPKService(pool *pgxpool.Pool) *PKService {
	return &PKService{pool: pool}
}

// StartBattle tạo trận PK ACTIVE. sides[i] là danh sách room thuộc phe i+1 (hỗ trợ 1v1 và team PK).
// Mọi room phải đang LIVE và chưa ở trong trận PK nào khác (DB chặn bằng idx_pk_side_rooms_one_active).
func (s *PKService) StartBattle(ctx context.Context, sides [][]uuid.UUID) (uuid.UUID, error) {
	if len(sides) < 2 {
		return uuid.Nil, fmt.Errorf("%w: a battle needs at least 2 sides", ErrInvalidInput)
	}
	var allRooms []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, side := range sides {
		if len(side) == 0 {
			return uuid.Nil, fmt.Errorf("%w: every side needs at least 1 room", ErrInvalidInput)
		}
		for _, roomID := range side {
			if seen[roomID] {
				return uuid.Nil, fmt.Errorf("%w: room %s appears twice", ErrInvalidInput, roomID)
			}
			seen[roomID] = true
			allRooms = append(allRooms, roomID)
		}
	}
	// Khóa room theo thứ tự id cố định để tránh deadlock giữa 2 lệnh StartBattle chạy song song
	sort.Slice(allRooms, func(i, j int) bool { return bytes.Compare(allRooms[i][:], allRooms[j][:]) < 0 })

	var battleID uuid.UUID
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		hosts := map[uuid.UUID]uuid.UUID{}
		for _, roomID := range allRooms {
			room, err := q.GetRoomForShare(ctx, roomID)
			if isNoRows(err) {
				return fmt.Errorf("%w: room %s", ErrNotFound, roomID)
			}
			if err != nil {
				return fmt.Errorf("get room: %w", err)
			}
			if room.Status != db.RoomStatusLIVE {
				return fmt.Errorf("%w: room %s is not live", ErrConflict, roomID)
			}
			hosts[roomID] = room.HostID
		}

		var err error
		battleID, err = q.CreatePKBattle(ctx)
		if err != nil {
			return fmt.Errorf("create battle: %w", err)
		}
		for i, side := range sides {
			sideID, err := q.CreatePKSide(ctx, db.CreatePKSideParams{BattleID: battleID, SideNumber: int32(i + 1)})
			if err != nil {
				return fmt.Errorf("create side: %w", err)
			}
			for _, roomID := range side {
				err := q.AddPKSideRoom(ctx, db.AddPKSideRoomParams{SideID: sideID, RoomID: roomID, HostID: hosts[roomID]})
				if isUniqueViolation(err, "idx_pk_side_rooms_one_active") {
					return fmt.Errorf("%w: room %s is already in a battle", ErrConflict, roomID)
				}
				if err != nil {
					return fmt.Errorf("add side room: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return battleID, nil
}

type FinishBattleResult struct {
	BattleID     uuid.UUID  `json:"battle_id"`
	WinnerSideID *uuid.UUID `json:"winner_side_id"` // nil = hòa
}

func (s *PKService) FinishBattle(ctx context.Context, battleID uuid.UUID) (FinishBattleResult, error) {
	var winner *uuid.UUID
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		var err error
		winner, err = finishBattleTx(ctx, q, battleID)
		return err
	})
	if err != nil {
		return FinishBattleResult{}, err
	}
	return FinishBattleResult{BattleID: battleID, WinnerSideID: winner}, nil
}

// finishBattleTx kết thúc trận trong transaction có sẵn. Khóa battle FOR UPDATE nên sẽ chờ
// mọi lượt cộng điểm (đang giữ FOR SHARE) hoàn tất, đảm bảo điểm chốt không bị thay đổi sau đó.
func finishBattleTx(ctx context.Context, q *db.Queries, battleID uuid.UUID) (*uuid.UUID, error) {
	battle, err := q.LockPKBattle(ctx, battleID)
	if isNoRows(err) {
		return nil, fmt.Errorf("%w: battle", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("lock battle: %w", err)
	}
	if battle.Status != db.PkStatusACTIVE {
		return nil, fmt.Errorf("%w: battle is %s", ErrConflict, battle.Status)
	}

	sides, err := q.ListPKSidesByScore(ctx, battleID)
	if err != nil {
		return nil, fmt.Errorf("list sides: %w", err)
	}
	var winner *uuid.UUID
	if len(sides) > 0 && (len(sides) == 1 || sides[0].Score > sides[1].Score) {
		winner = &sides[0].ID
	}

	if err := q.FinishPKBattle(ctx, db.FinishPKBattleParams{ID: battleID, WinnerSideID: winner}); err != nil {
		return nil, fmt.Errorf("finish battle: %w", err)
	}
	if err := q.ReleasePKSideRooms(ctx, battleID); err != nil {
		return nil, fmt.Errorf("release side rooms: %w", err)
	}
	return winner, nil
}

// addPKScoreForGift cộng điểm cho phe của room nếu room đang trong trận PK ACTIVE.
// Được gọi bên trong transaction tặng quà, nên quà và điểm PK luôn cùng thành công hoặc cùng thất bại.
func addPKScoreForGift(ctx context.Context, q *db.Queries, roomID, senderID, giftLogID uuid.UUID, points int64) (*uuid.UUID, error) {
	side, err := q.GetActivePKSideByRoomForShare(ctx, roomID)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active pk side: %w", err)
	}
	if points <= 0 {
		return nil, nil
	}

	if err := q.IncrementPKSideScore(ctx, db.IncrementPKSideScoreParams{ID: side.SideID, Points: points}); err != nil {
		return nil, fmt.Errorf("increment pk score: %w", err)
	}
	err = q.CreatePKScoreEvent(ctx, db.CreatePKScoreEventParams{
		BattleID:      side.BattleID,
		SideID:        side.SideID,
		RoomID:        &roomID,
		UserID:        &senderID,
		Source:        db.PkScoreSourceGIFT,
		Points:        points,
		ReferenceType: ptr(giftRefTypeGiftLog),
		ReferenceID:   &giftLogID,
	})
	if err != nil {
		return nil, fmt.Errorf("create pk score event: %w", err)
	}
	return &side.BattleID, nil
}
