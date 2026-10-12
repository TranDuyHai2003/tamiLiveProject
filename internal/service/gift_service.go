package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxGiftQuantity    = 9999
	idempotencyKeyTTL  = 24 * time.Hour
	maxIdempotencyKey  = 255
	giftRefTypeGiftLog = "GIFT_LOG"
)

type GiftService struct {
	pool           *pgxpool.Pool
	gemRatePercent int64
}

func NewGiftService(pool *pgxpool.Pool, gemRatePercent int64) *GiftService {
	return &GiftService{pool: pool, gemRatePercent: gemRatePercent}
}

type SendGiftInput struct {
	SenderID uuid.UUID
	RoomID   uuid.UUID
	GiftID   uuid.UUID
	// Bỏ trống = tặng cho host của phòng. Nếu chỉ định thì phải là host hoặc CO_HOST đang JOINED.
	ReceiverID     *uuid.UUID
	Quantity       int64
	IdempotencyKey string
}

type SendGiftResult struct {
	GiftLogID         uuid.UUID  `json:"gift_log_id"`
	TransactionID     uuid.UUID  `json:"transaction_id"`
	ReceiverID        uuid.UUID  `json:"receiver_id"`
	TotalCoin         int64      `json:"total_coin"`
	TotalGem          int64      `json:"total_gem"`
	SenderCoinBalance int64      `json:"sender_coin_balance"`
	PKBattleID        *uuid.UUID `json:"pk_battle_id,omitempty"`
	// true nếu đây là kết quả của một request trước đó với cùng idempotency key
	Replayed bool `json:"replayed"`
}

// SendGift thực hiện tặng quà trong một transaction duy nhất:
//
//  1. Chống request trùng bằng idempotency key (theo user).
//  2. Kiểm tra phòng LIVE (khóa FOR SHARE để phòng không bị kết thúc giữa chừng), người nhận hợp lệ, quà còn bán.
//  3. Hạch toán kế toán kép:
//     COIN: người gửi  -total_coin  -> PLATFORM_REVENUE +total_coin
//     GEM : GEM_LIABILITY -total_gem -> người nhận +total_gem   (total_gem = total_coin * rate%)
//  4. Ghi gift_logs, và cộng điểm PK nếu phòng đang trong trận PK ACTIVE.
func (s *GiftService) SendGift(ctx context.Context, in SendGiftInput) (SendGiftResult, error) {
	if in.Quantity < 1 || in.Quantity > maxGiftQuantity {
		return SendGiftResult{}, invalidField("quantity", "range", fmt.Sprintf("quantity must be in [1, %d]", maxGiftQuantity))
	}
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > maxIdempotencyKey {
		return SendGiftResult{}, invalidField("Idempotency-Key", "required", fmt.Sprintf("Idempotency-Key header must be 1-%d chars", maxIdempotencyKey))
	}
	requestHash := hashGiftRequest(in)

	var result SendGiftResult
	err := withTx(ctx, s.pool, func(q *db.Queries) error {
		// 1. Idempotency: request trùng key sẽ bị chặn ở unique (user_id, key) cho tới khi request đầu commit/rollback
		keyID, err := q.InsertIdempotencyKey(ctx, db.InsertIdempotencyKeyParams{
			UserID:      in.SenderID,
			Key:         in.IdempotencyKey,
			RequestHash: requestHash,
			ExpiresAt:   time.Now().Add(idempotencyKeyTTL),
		})
		if isNoRows(err) {
			result, err = replayGift(ctx, q, in.SenderID, in.IdempotencyKey, requestHash)
			return err
		}
		if err != nil {
			return fmt.Errorf("insert idempotency key: %w", err)
		}

		// 2. Kiểm tra phòng, người nhận, quà
		room, err := q.GetRoomForShare(ctx, in.RoomID)
		if isNoRows(err) {
			return fmt.Errorf("%w: room", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("get room: %w", err)
		}
		if room.Status != db.RoomStatusLIVE {
			return errRoomNotLive()
		}

		receiverID := room.HostID
		if in.ReceiverID != nil && *in.ReceiverID != room.HostID {
			member, err := q.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: room.ID, UserID: *in.ReceiverID})
			if err != nil && !isNoRows(err) {
				return fmt.Errorf("get receiver membership: %w", err)
			}
			if isNoRows(err) || member.Role != db.RoomMemberRoleCOHOST || member.Status != db.RoomMemberStatusJOINED {
				return fmt.Errorf("%w: receiver must be the host or a co-host of the room", ErrInvalidInput)
			}
			receiverID = *in.ReceiverID
		}
		if receiverID == in.SenderID {
			return fmt.Errorf("%w: cannot send a gift to yourself", ErrInvalidInput)
		}

		gift, err := q.GetActiveGift(ctx, in.GiftID)
		if isNoRows(err) {
			return fmt.Errorf("%w: gift not found or inactive", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("get gift: %w", err)
		}
		if gift.CoinPrice > math.MaxInt64/in.Quantity {
			return fmt.Errorf("%w: gift total overflows", ErrInvalidInput)
		}
		totalCoin := gift.CoinPrice * in.Quantity
		totalGem := percentOf(totalCoin, s.gemRatePercent)

		// 3. Hạch toán
		senderWalletID, err := userWalletID(ctx, q, in.SenderID)
		if err != nil {
			return err
		}
		receiverWalletID, err := userWalletID(ctx, q, receiverID)
		if err != nil {
			return err
		}
		revenueWalletID, err := systemWalletID(ctx, q, walletPlatformRevenue)
		if err != nil {
			return err
		}
		liabilityWalletID, err := systemWalletID(ctx, q, walletGemLiability)
		if err != nil {
			return err
		}

		metadata, _ := json.Marshal(map[string]any{
			"room_id":     room.ID,
			"gift_id":     gift.ID,
			"sender_id":   in.SenderID,
			"receiver_id": receiverID,
			"quantity":    in.Quantity,
		})
		txID, err := q.CreateEconomyTransaction(ctx, db.CreateEconomyTransactionParams{
			Type:           db.EconomyTransactionTypeGIFT,
			ReferenceType:  ptr("GIFT"),
			ReferenceID:    &gift.ID,
			IdempotencyKey: &in.IdempotencyKey,
			Metadata:       metadata,
		})
		if err != nil {
			return fmt.Errorf("create economy transaction: %w", err)
		}

		legs := []ledgerLeg{
			{WalletID: senderWalletID, Currency: db.CurrencyTypeCOIN, Delta: -totalCoin, Description: "gift sent: " + gift.Name},
			{WalletID: revenueWalletID, Currency: db.CurrencyTypeCOIN, Delta: totalCoin, Description: "gift revenue"},
		}
		if totalGem > 0 {
			legs = append(legs,
				ledgerLeg{WalletID: liabilityWalletID, Currency: db.CurrencyTypeGEM, Delta: -totalGem, Description: "gem issued for gift"},
				ledgerLeg{WalletID: receiverWalletID, Currency: db.CurrencyTypeGEM, Delta: totalGem, Description: "gift received: " + gift.Name},
			)
		}
		balances, err := postLedger(ctx, q, txID, legs)
		if err != nil {
			return err
		}

		// 4. Biên lai + điểm PK
		giftLog, err := q.CreateGiftLog(ctx, db.CreateGiftLogParams{
			SenderID:             in.SenderID,
			ReceiverID:           receiverID,
			RoomID:               room.ID,
			GiftID:               gift.ID,
			EconomyTransactionID: txID,
			Quantity:             in.Quantity,
			TotalCoin:            totalCoin,
			TotalGem:             totalGem,
		})
		if err != nil {
			return fmt.Errorf("create gift log: %w", err)
		}

		battleID, err := addPKScoreForGift(ctx, q, room.ID, in.SenderID, giftLog.ID, totalCoin)
		if err != nil {
			return err
		}

		result = SendGiftResult{
			GiftLogID:         giftLog.ID,
			TransactionID:     txID,
			ReceiverID:        receiverID,
			TotalCoin:         totalCoin,
			TotalGem:          totalGem,
			SenderCoinBalance: balances[0],
			PKBattleID:        battleID,
		}

		body, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("marshal gift result: %w", err)
		}
		return q.CompleteIdempotencyKey(ctx, db.CompleteIdempotencyKeyParams{
			ID:             keyID,
			ResponseStatus: ptr(int32(http.StatusOK)),
			ResponseBody:   body,
			TransactionID:  &txID,
		})
	})
	if err != nil {
		return SendGiftResult{}, err
	}
	return result, nil
}

// replayGift trả lại kết quả của request trước đó có cùng idempotency key.
func replayGift(ctx context.Context, q *db.Queries, userID uuid.UUID, key, requestHash string) (SendGiftResult, error) {
	existing, err := q.GetIdempotencyKey(ctx, db.GetIdempotencyKeyParams{UserID: userID, Key: key})
	if err != nil {
		return SendGiftResult{}, fmt.Errorf("get idempotency key: %w", err)
	}
	if existing.RequestHash != requestHash {
		return SendGiftResult{}, newError(ErrConflict, CodeIdempotencyKeyReused, "Idempotency-Key was already used for a different request")
	}
	if existing.Status != db.IdempotencyStatusCOMPLETED {
		return SendGiftResult{}, fmt.Errorf("%w: request with this idempotency_key is still processing", ErrConflict)
	}

	var result SendGiftResult
	if err := json.Unmarshal(existing.ResponseBody, &result); err != nil {
		return SendGiftResult{}, fmt.Errorf("decode stored gift result: %w", err)
	}
	result.Replayed = true
	return result, nil
}

func hashGiftRequest(in SendGiftInput) string {
	receiver := ""
	if in.ReceiverID != nil {
		receiver = in.ReceiverID.String()
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "gift|%s|%s|%s|%d", in.RoomID, in.GiftID, receiver, in.Quantity))
	return hex.EncodeToString(sum[:])
}

// percentOf tính floor(amount * percent / 100) mà không bị tràn số.
func percentOf(amount, percent int64) int64 {
	return amount/100*percent + amount%100*percent/100
}
