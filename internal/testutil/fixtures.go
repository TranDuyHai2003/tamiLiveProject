package testutil

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Topup nạp coin cho user bằng một giao dịch TOPUP cân sổ (FIAT_RESERVE −coin → user +coin),
// viết thẳng bằng SQL để package nào cũng dùng được (không phụ thuộc package service).
func Topup(t testing.TB, p *pgxpool.Pool, userID uuid.UUID, coin int64) {
	t.Helper()
	_, err := p.Exec(context.Background(), `
		WITH tx AS (
			INSERT INTO economy_transactions (type, status, completed_at)
			VALUES ('TOPUP', 'COMPLETED', NOW()) RETURNING id
		),
		u AS (UPDATE wallets SET coin = coin + $2 WHERE user_id = $1 RETURNING id, coin),
		r AS (UPDATE wallets SET coin = coin - $2 WHERE account_code = 'FIAT_RESERVE' RETURNING id, coin)
		INSERT INTO economy_ledger_entries (transaction_id, wallet_id, currency, entry_type, amount, balance_before, balance_after)
		SELECT tx.id, u.id, 'COIN'::currency_type, 'CREDIT'::ledger_entry_type, $2, u.coin - $2, u.coin FROM tx, u
		UNION ALL
		SELECT tx.id, r.id, 'COIN'::currency_type, 'DEBIT'::ledger_entry_type, $2, r.coin + $2, r.coin FROM tx, r`, userID, coin)
	if err != nil {
		t.Fatalf("topup: %v", err)
	}
}

// CreateGift thêm một món quà đang bán.
func CreateGift(t testing.TB, p *pgxpool.Pool, name string, price int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := p.QueryRow(context.Background(),
		`INSERT INTO gifts (name, coin_price) VALUES ($1, $2) RETURNING id`, name, price).Scan(&id); err != nil {
		t.Fatalf("create gift: %v", err)
	}
	return id
}
