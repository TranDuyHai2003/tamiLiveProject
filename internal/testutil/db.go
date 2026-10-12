// Package testutil chứa công cụ dùng chung cho test tích hợp chạy trên Postgres thật.
//
// Vì sao không dùng kiểu "mỗi test một transaction rồi rollback":
//   - Trigger cân sổ cái (trg_ledger_balanced) là DEFERRED, chỉ chạy lúc COMMIT.
//     Rollback thì trigger không bao giờ chạy, sổ lệch cũng không bị phát hiện.
//   - Test đồng thời cần nhiều transaction thật, khóa dòng thật.
//
// Vì vậy test COMMIT thật, và Reset xóa dữ liệu nghiệp vụ trước mỗi test.
// Các package test dùng chung một DB nên phải chạy tuần tự: go test -p 1.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"livestream-backend/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	poolOnce sync.Once
	pool     *pgxpool.Pool
	poolErr  error
)

// Pool trả về connection pool tới TEST_DATABASE_URL, dùng chung cho cả package test.
// Từ chối chạy nếu tên database không kết thúc bằng "_test": Reset xóa sạch dữ liệu,
// trỏ nhầm vào DB dev là mất hết.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	poolOnce.Do(func() {
		dsn := os.Getenv("TEST_DATABASE_URL")
		if dsn == "" {
			poolErr = fmt.Errorf("TEST_DATABASE_URL is not set; run tests with `make test`")
			return
		}
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			poolErr = fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
			return
		}
		if name := cfg.ConnConfig.Database; !strings.HasSuffix(name, "_test") {
			poolErr = fmt.Errorf("refusing to run tests against database %q: name must end with _test", name)
			return
		}
		db.ConfigurePool(cfg) // cùng thiết lập với server (VD thời gian UTC)
		cfg.MaxConns = 30     // đủ cho test đồng thời
		pool, poolErr = pgxpool.NewWithConfig(context.Background(), cfg)
		if poolErr == nil {
			poolErr = pool.Ping(context.Background())
		}
	})
	if poolErr != nil {
		t.Fatalf("test database: %v", poolErr)
	}
	return pool
}

// Các bảng KHÔNG bị TRUNCATE vì chứa dữ liệu seed từ migration.
// Thêm bảng seed mới (VD level_thresholds, coin_packages) thì phải thêm vào đây.
var keepTables = map[string]bool{
	"schema_migrations": true,
	"wallets":           true, // giữ ví hệ thống, chỉ xóa ví USER
	"users":             true, // xóa bằng DELETE sau khi đã xóa ví USER
}

// Reset xóa mọi dữ liệu nghiệp vụ, giữ lại ví hệ thống (đưa số dư về 0).
// TRUNCATE không kích hoạt trigger cấm sửa/xóa sổ cái (trigger đó là FOR EACH ROW ON DELETE).
func Reset(t testing.TB, p *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	rows, err := p.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var truncate []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		if !keepTables[name] {
			truncate = append(truncate, name)
		}
	}
	rows.Close()

	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatalf("begin reset: %v", err)
	}
	defer tx.Rollback(ctx)

	stmts := []string{
		"TRUNCATE " + strings.Join(truncate, ", "),
		"DELETE FROM wallets WHERE account_type = 'USER'",
		"DELETE FROM users",
		"UPDATE wallets SET coin = 0, gem = 0",
	}
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s); err != nil {
			t.Fatalf("reset (%s): %v", s, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit reset: %v", err)
	}
}

// AssertLedgerInvariants kiểm tra các bất biến của sổ cái. Gọi sau mỗi test:
// kể cả test không nhắm vào tiền, nếu code làm lệch sổ thì vẫn bị bắt.
func AssertLedgerInvariants(t testing.TB, p *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// 1. Tổng số dư mọi ví theo từng loại tiền = 0 (tiền chỉ chuyển chỗ, không tự sinh ra/mất đi)
	var coinSum, gemSum int64
	if err := p.QueryRow(ctx, `SELECT COALESCE(SUM(coin), 0), COALESCE(SUM(gem), 0) FROM wallets`).Scan(&coinSum, &gemSum); err != nil {
		t.Fatalf("sum wallets: %v", err)
	}
	if coinSum != 0 || gemSum != 0 {
		t.Errorf("ledger invariant broken: SUM(coin)=%d, SUM(gem)=%d, want 0 and 0", coinSum, gemSum)
	}

	// 2. Số dư từng ví khớp tổng bút toán của ví đó
	var mismatched int
	err := p.QueryRow(ctx, `
		SELECT COUNT(*) FROM wallets w
		WHERE w.coin <> COALESCE((SELECT SUM(CASE WHEN l.entry_type = 'CREDIT' THEN l.amount ELSE -l.amount END)
		                          FROM economy_ledger_entries l WHERE l.wallet_id = w.id AND l.currency = 'COIN'), 0)
		   OR w.gem  <> COALESCE((SELECT SUM(CASE WHEN l.entry_type = 'CREDIT' THEN l.amount ELSE -l.amount END)
		                          FROM economy_ledger_entries l WHERE l.wallet_id = w.id AND l.currency = 'GEM'), 0)`,
	).Scan(&mismatched)
	if err != nil {
		t.Fatalf("compare wallets with ledger: %v", err)
	}
	if mismatched != 0 {
		t.Errorf("ledger invariant broken: %d wallet(s) whose balance differs from their ledger entries", mismatched)
	}
}
