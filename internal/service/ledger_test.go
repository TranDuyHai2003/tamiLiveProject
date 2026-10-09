package service

import (
	"testing"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
)

// postInTx chạy postLedger trong một transaction thật (commit nếu thành công).
func (f *fixture) postInTx(legs func(q *db.Queries) []ledgerLeg) ([]int64, error) {
	var balances []int64
	err := withTx(f.ctx, f.pool, func(q *db.Queries) error {
		txID, err := q.CreateEconomyTransaction(f.ctx, db.CreateEconomyTransactionParams{Type: db.EconomyTransactionTypeADJUSTMENT})
		if err != nil {
			return err
		}
		balances, err = postLedger(f.ctx, q, txID, legs(q))
		return err
	})
	return balances, err
}

func (f *fixture) walletOf(q *db.Queries, userID uuid.UUID) uuid.UUID {
	f.t.Helper()
	id, err := userWalletID(f.ctx, q, userID)
	if err != nil {
		f.t.Fatalf("wallet: %v", err)
	}
	return id
}

func (f *fixture) system(q *db.Queries, code string) uuid.UUID {
	f.t.Helper()
	id, err := systemWalletID(f.ctx, q, code)
	if err != nil {
		f.t.Fatalf("system wallet: %v", err)
	}
	return id
}

func TestPostLedger_RejectsUnbalancedLegs(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice")

	_, err := f.postInTx(func(q *db.Queries) []ledgerLeg {
		return []ledgerLeg{
			{WalletID: f.system(q, "FIAT_RESERVE"), Currency: db.CurrencyTypeCOIN, Delta: -10},
			{WalletID: f.walletOf(q, user), Currency: db.CurrencyTypeCOIN, Delta: 5},
		}
	})
	if err == nil {
		t.Fatal("unbalanced legs were accepted")
	}
	if n := f.count("economy_ledger_entries"); n != 0 {
		t.Fatalf("ledger entries written for rejected transaction: %d", n)
	}
}

func TestPostLedger_RejectsZeroLeg(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice")

	_, err := f.postInTx(func(q *db.Queries) []ledgerLeg {
		return []ledgerLeg{
			{WalletID: f.system(q, "FIAT_RESERVE"), Currency: db.CurrencyTypeCOIN, Delta: 0},
			{WalletID: f.walletOf(q, user), Currency: db.CurrencyTypeCOIN, Delta: 0},
		}
	})
	if err == nil {
		t.Fatal("zero-amount legs were accepted")
	}
}

func TestPostLedger_UserWalletCannotGoNegative(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice") // 0 coin

	_, err := f.postInTx(func(q *db.Queries) []ledgerLeg {
		return []ledgerLeg{
			{WalletID: f.walletOf(q, user), Currency: db.CurrencyTypeCOIN, Delta: -10},
			{WalletID: f.system(q, walletPlatformRevenue), Currency: db.CurrencyTypeCOIN, Delta: 10},
		}
	})
	wantErrIs(t, err, ErrInsufficientBalance)
	if got := f.userCoin(user); got != 0 {
		t.Fatalf("user coin = %d after failed debit, want 0", got)
	}
}

func TestPostLedger_IssuanceWalletMayGoNegative(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice")

	f.topup(user, 100) // FIAT_RESERVE (allow_negative) −100 → user +100

	if got := f.userCoin(user); got != 100 {
		t.Fatalf("user coin = %d, want 100", got)
	}
	if coin, _ := f.systemBalance("FIAT_RESERVE"); coin != -100 {
		t.Fatalf("FIAT_RESERVE coin = %d, want -100", coin)
	}
}

func TestPostLedger_FundWalletCannotGoNegative(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice")

	// SYSTEM_REWARD là ví quỹ (allow_negative = false), đang 0
	_, err := f.postInTx(func(q *db.Queries) []ledgerLeg {
		return []ledgerLeg{
			{WalletID: f.system(q, walletSystemReward), Currency: db.CurrencyTypeCOIN, Delta: -10},
			{WalletID: f.walletOf(q, user), Currency: db.CurrencyTypeCOIN, Delta: 10},
		}
	})
	wantErrIs(t, err, ErrInsufficientBalance)
}

func TestPostLedger_ReturnsBalancesInInputOrderAndRecordsEntries(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice")
	f.topup(user, 50)

	// Thứ tự đầu vào: doanh thu trước, user sau. postLedger khóa theo wallet_id
	// nhưng phải trả số dư đúng theo thứ tự đầu vào.
	balances, err := f.postInTx(func(q *db.Queries) []ledgerLeg {
		return []ledgerLeg{
			{WalletID: f.system(q, walletPlatformRevenue), Currency: db.CurrencyTypeCOIN, Delta: 30},
			{WalletID: f.walletOf(q, user), Currency: db.CurrencyTypeCOIN, Delta: -30},
		}
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if len(balances) != 2 || balances[0] != 30 || balances[1] != 20 {
		t.Fatalf("balances = %v, want [30 20]", balances)
	}

	var entryType string
	var amount, before, after int64
	err = f.pool.QueryRow(f.ctx, `
		SELECT l.entry_type, l.amount, l.balance_before, l.balance_after
		FROM economy_ledger_entries l JOIN wallets w ON w.id = l.wallet_id
		WHERE w.user_id = $1 AND l.entry_type = 'DEBIT'`, user).Scan(&entryType, &amount, &before, &after)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if amount != 30 || before != 50 || after != 20 {
		t.Fatalf("entry = %s amount %d %d→%d, want DEBIT 30 50→20", entryType, amount, before, after)
	}
}
