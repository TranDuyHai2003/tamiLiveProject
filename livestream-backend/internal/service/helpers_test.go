package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"livestream-backend/internal/auth"
	"livestream-backend/internal/db"
	"livestream-backend/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testGemRatePercent = 50
	testGameSecret     = "test-minigame-secret"
)

// fixture gom các service dùng chung trên DB test.
// newFixture xóa sạch dữ liệu trước test và kiểm tra bất biến sổ cái sau test.
type fixture struct {
	t     *testing.T
	ctx   context.Context
	pool  *pgxpool.Pool
	q     *db.Queries
	users *UserService
	gifts *GiftService
	rooms *RoomService
	pk    *PKService
	games *MiniGameService
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.Reset(t, pool)
	t.Cleanup(func() { testutil.AssertLedgerInvariants(t, pool) })

	return &fixture{
		t:     t,
		ctx:   context.Background(),
		pool:  pool,
		q:     db.New(pool),
		users: NewUserService(pool, auth.NewTokenIssuer("test-jwt-secret-0123456789abcdef0123", time.Hour)),
		gifts: NewGiftService(pool, testGemRatePercent),
		rooms: NewRoomService(pool, "devkey", "test-livekit-secret-0123456789abcdef"),
		pk:    NewPKService(pool),
		games: NewMiniGameService(pool, testGameSecret),
	}
}

func (f *fixture) createUser(name string) uuid.UUID {
	f.t.Helper()
	u, err := f.users.Register(f.ctx, name, name+"@test.local", "password123")
	if err != nil {
		f.t.Fatalf("register %s: %v", name, err)
	}
	return u.ID
}

// topup nạp coin cho user bằng một giao dịch TOPUP thật qua sổ cái (FIAT_RESERVE → user).
func (f *fixture) topup(userID uuid.UUID, coin int64) {
	f.t.Helper()
	err := withTx(f.ctx, f.pool, func(q *db.Queries) error {
		reserve, err := systemWalletID(f.ctx, q, "FIAT_RESERVE")
		if err != nil {
			return err
		}
		wallet, err := userWalletID(f.ctx, q, userID)
		if err != nil {
			return err
		}
		txID, err := q.CreateEconomyTransaction(f.ctx, db.CreateEconomyTransactionParams{Type: db.EconomyTransactionTypeTOPUP})
		if err != nil {
			return err
		}
		_, err = postLedger(f.ctx, q, txID, []ledgerLeg{
			{WalletID: reserve, Currency: db.CurrencyTypeCOIN, Delta: -coin, Description: "test topup"},
			{WalletID: wallet, Currency: db.CurrencyTypeCOIN, Delta: coin, Description: "test topup"},
		})
		return err
	})
	if err != nil {
		f.t.Fatalf("topup: %v", err)
	}
}

// fundRewardBudget nạp quỹ thưởng game: PROMO_BUDGET → SYSTEM_REWARD (giao dịch ADJUSTMENT).
func (f *fixture) fundRewardBudget(coin int64) {
	f.t.Helper()
	err := withTx(f.ctx, f.pool, func(q *db.Queries) error {
		promo, err := systemWalletID(f.ctx, q, "PROMO_BUDGET")
		if err != nil {
			return err
		}
		reward, err := systemWalletID(f.ctx, q, walletSystemReward)
		if err != nil {
			return err
		}
		txID, err := q.CreateEconomyTransaction(f.ctx, db.CreateEconomyTransactionParams{Type: db.EconomyTransactionTypeADJUSTMENT})
		if err != nil {
			return err
		}
		_, err = postLedger(f.ctx, q, txID, []ledgerLeg{
			{WalletID: promo, Currency: db.CurrencyTypeCOIN, Delta: -coin, Description: "test budget"},
			{WalletID: reward, Currency: db.CurrencyTypeCOIN, Delta: coin, Description: "test budget"},
		})
		return err
	})
	if err != nil {
		f.t.Fatalf("fund reward budget: %v", err)
	}
}

func (f *fixture) createGift(name string, price int64) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO gifts (name, coin_price) VALUES ($1, $2) RETURNING id`, name, price).Scan(&id); err != nil {
		f.t.Fatalf("create gift: %v", err)
	}
	return id
}

func (f *fixture) liveRoom(hostID uuid.UUID) db.Room {
	f.t.Helper()
	room, err := f.rooms.CreateRoom(f.ctx, hostID, "test room", "", "")
	if err != nil {
		f.t.Fatalf("create room: %v", err)
	}
	return room
}

func (f *fixture) sendGift(senderID, roomID, giftID uuid.UUID, qty int64, key string) (SendGiftResult, error) {
	return f.gifts.SendGift(f.ctx, SendGiftInput{
		SenderID: senderID, RoomID: roomID, GiftID: giftID, Quantity: qty, IdempotencyKey: key,
	})
}

func (f *fixture) userCoin(userID uuid.UUID) int64 {
	f.t.Helper()
	w, err := f.q.GetUserWallet(f.ctx, &userID)
	if err != nil {
		f.t.Fatalf("get wallet: %v", err)
	}
	return w.Coin
}

func (f *fixture) userGem(userID uuid.UUID) int64 {
	f.t.Helper()
	w, err := f.q.GetUserWallet(f.ctx, &userID)
	if err != nil {
		f.t.Fatalf("get wallet: %v", err)
	}
	return w.Gem
}

func (f *fixture) systemBalance(code string) (coin, gem int64) {
	f.t.Helper()
	if err := f.pool.QueryRow(f.ctx, `SELECT coin, gem FROM wallets WHERE account_code = $1`, code).Scan(&coin, &gem); err != nil {
		f.t.Fatalf("system wallet %s: %v", code, err)
	}
	return coin, gem
}

// count đếm số dòng của một bảng (tên bảng do test viết cứng, không phải input người dùng).
func (f *fixture) count(table string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&n); err != nil {
		f.t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// pgCode trả về mã lỗi Postgres (VD "23503" vi phạm khóa ngoại), rỗng nếu không phải lỗi Postgres.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func wantErrIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("got error %v, want %v", err, target)
	}
}
