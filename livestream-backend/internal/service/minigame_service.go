package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMaxPlayers = 4
	maxMaxPlayers     = 16
)

type MiniGameService struct {
	pool      *pgxpool.Pool
	secretKey []byte // Khóa bí mật dùng chung với game provider để ký kết quả
}

func NewMiniGameService(pool *pgxpool.Pool, secretKey string) *MiniGameService {
	return &MiniGameService{pool: pool, secretKey: []byte(secretKey)}
}

// CreateSession: host mở một phiên game trong phòng đang LIVE của mình (mỗi phòng tối đa 1 phiên đang mở).
func (s *MiniGameService) CreateSession(ctx context.Context, hostID, roomID, gameID uuid.UUID, maxPlayers int32) (db.GameSession, error) {
	if maxPlayers == 0 {
		maxPlayers = defaultMaxPlayers
	}
	if maxPlayers < 1 || maxPlayers > maxMaxPlayers {
		return db.GameSession{}, invalidField("max_players", "range", fmt.Sprintf("max_players must be in [1, %d]", maxMaxPlayers))
	}

	var session db.GameSession
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		room, err := q.GetRoomForShare(ctx, roomID)
		if isNoRows(err) {
			return fmt.Errorf("%w: room", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("get room: %w", err)
		}
		if room.HostID != hostID {
			return fmt.Errorf("%w: only the host can start a game", ErrForbidden)
		}
		if room.Status != db.RoomStatusLIVE {
			return errRoomNotLive()
		}
		if _, err := q.GetActiveGame(ctx, gameID); isNoRows(err) {
			return fmt.Errorf("%w: game not found or inactive", ErrNotFound)
		} else if err != nil {
			return fmt.Errorf("get game: %w", err)
		}

		session, err = q.CreateGameSession(ctx, db.CreateGameSessionParams{
			RoomID:     roomID,
			GameID:     gameID,
			HostID:     hostID,
			MaxPlayers: maxPlayers,
		})
		if isUniqueViolation(err, "idx_game_sessions_one_open_per_room") {
			return fmt.Errorf("%w: room already has an open game session", ErrConflict)
		}
		return err
	})
	if err != nil {
		return db.GameSession{}, err
	}
	return session, nil
}

// JoinSession: viewer đang JOINED trong phòng xin vào chơi; server tự xếp ghế trống nhỏ nhất.
func (s *MiniGameService) JoinSession(ctx context.Context, sessionID, userID uuid.UUID) (db.GamePlayer, error) {
	var player db.GamePlayer
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		// FOR UPDATE: tuần tự hóa các lượt join để không vượt quá max_players
		session, err := lockSession(ctx, q, sessionID)
		if err != nil {
			return err
		}
		if session.Status != db.GameSessionStatusWAITING {
			return fmt.Errorf("%w: session is not accepting players", ErrConflict)
		}

		member, err := q.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: session.RoomID, UserID: userID})
		if isNoRows(err) || (err == nil && member.Status != db.RoomMemberStatusJOINED) {
			return fmt.Errorf("%w: join the room first", ErrForbidden)
		}
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		taken, err := q.ListTakenSeats(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("list seats: %w", err)
		}
		seat := firstFreeSeat(taken, session.MaxPlayers)
		if seat == 0 {
			return fmt.Errorf("%w: session is full", ErrConflict)
		}

		player, err = q.CreateGamePlayer(ctx, db.CreateGamePlayerParams{SessionID: sessionID, UserID: userID, SeatNumber: &seat})
		if isUniqueViolation(err, "unique_session_user") {
			return fmt.Errorf("%w: already joined this session", ErrConflict)
		}
		return err
	})
	if err != nil {
		return db.GamePlayer{}, err
	}
	return player, nil
}

// StartSession: host chuyển WAITING -> RUNNING, mọi người chơi đang chờ chuyển sang PLAYING.
func (s *MiniGameService) StartSession(ctx context.Context, sessionID, hostID uuid.UUID) error {
	return withTx(ctx, s.pool, func(q *db.Queries) error {
		session, err := lockOwnedSession(ctx, q, sessionID, hostID)
		if err != nil {
			return err
		}
		if session.Status != db.GameSessionStatusWAITING {
			return fmt.Errorf("%w: session is %s", ErrConflict, session.Status)
		}
		taken, err := q.ListTakenSeats(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("list seats: %w", err)
		}
		if len(taken) == 0 {
			return fmt.Errorf("%w: no players joined", ErrConflict)
		}
		if err := q.StartGameSession(ctx, sessionID); err != nil {
			return fmt.Errorf("start session: %w", err)
		}
		return q.MarkGamePlayersPlaying(ctx, sessionID)
	})
}

// EndSession: host đóng phiên. WAITING -> CANCELLED, RUNNING -> FINISHED.
func (s *MiniGameService) EndSession(ctx context.Context, sessionID, hostID uuid.UUID) error {
	return withTx(ctx, s.pool, func(q *db.Queries) error {
		session, err := lockOwnedSession(ctx, q, sessionID, hostID)
		if err != nil {
			return err
		}
		var next db.GameSessionStatus
		switch session.Status {
		case db.GameSessionStatusWAITING:
			next = db.GameSessionStatusCANCELLED
		case db.GameSessionStatusRUNNING:
			next = db.GameSessionStatusFINISHED
		default:
			return fmt.Errorf("%w: session is already %s", ErrConflict, session.Status)
		}
		if err := q.CloseGameSession(ctx, db.CloseGameSessionParams{ID: sessionID, Status: next}); err != nil {
			return fmt.Errorf("close session: %w", err)
		}
		return q.MarkGamePlayersFinished(ctx, sessionID)
	})
}

type GameResultInput struct {
	SessionID  uuid.UUID
	UserID     uuid.UUID
	Score      int64
	RewardCoin int64
	Payload    json.RawMessage
	Signature  string
}

type GameResultOutput struct {
	ResultID      uuid.UUID  `json:"result_id"`
	RewardCoin    int64      `json:"reward_coin"`
	TransactionID *uuid.UUID `json:"transaction_id,omitempty"`
}

// SignGameResult là chuỗi mà game provider phải ký: hex(HMAC-SHA256(secret, "session_id|user_id|score|reward_coin")).
// Chữ ký bao trùm cả session, user, điểm và số thưởng, nên không thể đổi số thưởng hay dùng lại cho người khác.
func SignGameResult(secret []byte, sessionID, userID uuid.UUID, score, rewardCoin int64) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%d|%d", sessionID, userID, score, rewardCoin)
	return hex.EncodeToString(mac.Sum(nil))
}

// SubmitResult nhận kết quả từ game provider, xác thực chữ ký, lưu kết quả và trả thưởng COIN
// từ ví SYSTEM_REWARD. Mỗi player chỉ có 1 kết quả (unique_game_result_player) nên không thể nhận thưởng 2 lần.
func (s *MiniGameService) SubmitResult(ctx context.Context, in GameResultInput) (GameResultOutput, error) {
	if in.Score < 0 || in.RewardCoin < 0 {
		return GameResultOutput{}, fmt.Errorf("%w: score and reward_coin must be >= 0", ErrInvalidInput)
	}
	expected := SignGameResult(s.secretKey, in.SessionID, in.UserID, in.Score, in.RewardCoin)
	if !hmac.Equal([]byte(in.Signature), []byte(expected)) {
		return GameResultOutput{}, fmt.Errorf("%w: invalid signature", ErrUnauthorized)
	}

	var out GameResultOutput
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		session, err := lockSession(ctx, q, in.SessionID)
		if err != nil {
			return err
		}
		if session.Status != db.GameSessionStatusRUNNING {
			return fmt.Errorf("%w: session is not running", ErrConflict)
		}

		player, err := q.GetGamePlayer(ctx, db.GetGamePlayerParams{SessionID: in.SessionID, UserID: in.UserID})
		if isNoRows(err) {
			return fmt.Errorf("%w: user is not a player of this session", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("get player: %w", err)
		}
		if player.Status != db.GamePlayerStatusPLAYING {
			return fmt.Errorf("%w: player is %s", ErrConflict, player.Status)
		}

		var payload []byte
		if len(in.Payload) > 0 {
			payload = in.Payload
		}
		resultID, err := q.CreateGameResult(ctx, db.CreateGameResultParams{
			SessionID:     in.SessionID,
			PlayerID:      player.ID,
			UserID:        in.UserID,
			Score:         in.Score,
			RewardCoin:    in.RewardCoin,
			ResultPayload: payload,
			Signature:     in.Signature,
		})
		if isUniqueViolation(err, "unique_game_result_player") {
			return fmt.Errorf("%w: result already submitted for this player", ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("create game result: %w", err)
		}
		out = GameResultOutput{ResultID: resultID, RewardCoin: in.RewardCoin}

		if in.RewardCoin == 0 {
			return nil
		}
		userWallet, err := userWalletID(ctx, q, in.UserID)
		if err != nil {
			return err
		}
		rewardWallet, err := systemWalletID(ctx, q, walletSystemReward)
		if err != nil {
			return err
		}
		txID, err := q.CreateEconomyTransaction(ctx, db.CreateEconomyTransactionParams{
			Type:          db.EconomyTransactionTypeGAMEREWARD,
			ReferenceType: ptr("GAME_RESULT"),
			ReferenceID:   &resultID,
		})
		if err != nil {
			return fmt.Errorf("create economy transaction: %w", err)
		}
		_, err = postLedger(ctx, q, txID, []ledgerLeg{
			{WalletID: rewardWallet, Currency: db.CurrencyTypeCOIN, Delta: -in.RewardCoin, Description: "game reward issued"},
			{WalletID: userWallet, Currency: db.CurrencyTypeCOIN, Delta: in.RewardCoin, Description: "game reward"},
		})
		if errors.Is(err, ErrInsufficientBalance) {
			// Ví bị trừ duy nhất là SYSTEM_REWARD (không được âm): hết ngân sách thưởng.
			// Rollback cả kết quả, nên provider gửi lại được sau khi admin nạp thêm ngân sách.
			return newError(ErrInsufficientBalance, CodeRewardBudgetExhausted, "game reward budget exhausted")
		}
		if err != nil {
			return err
		}
		out.TransactionID = &txID
		return q.MarkGameResultRewarded(ctx, db.MarkGameResultRewardedParams{ID: resultID, EconomyTransactionID: &txID})
	})
	if err != nil {
		return GameResultOutput{}, err
	}
	return out, nil
}

func lockSession(ctx context.Context, q *db.Queries, sessionID uuid.UUID) (db.GameSession, error) {
	session, err := q.LockGameSession(ctx, sessionID)
	if isNoRows(err) {
		return db.GameSession{}, fmt.Errorf("%w: game session", ErrNotFound)
	}
	if err != nil {
		return db.GameSession{}, fmt.Errorf("lock game session: %w", err)
	}
	return session, nil
}

func lockOwnedSession(ctx context.Context, q *db.Queries, sessionID, hostID uuid.UUID) (db.GameSession, error) {
	session, err := lockSession(ctx, q, sessionID)
	if err != nil {
		return db.GameSession{}, err
	}
	if session.HostID != hostID {
		return db.GameSession{}, fmt.Errorf("%w: only the host can manage this session", ErrForbidden)
	}
	return session, nil
}

// firstFreeSeat trả về số ghế trống nhỏ nhất trong [1, max], hoặc 0 nếu hết ghế. taken đã sắp xếp tăng dần.
func firstFreeSeat(taken []int32, max int32) int32 {
	seat := int32(1)
	for _, t := range taken {
		if t == seat {
			seat++
		} else if t > seat {
			break
		}
	}
	if seat > max {
		return 0
	}
	return seat
}
