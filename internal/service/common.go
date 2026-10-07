package service

import (
	"context"
	"errors"
	"fmt"

	"livestream-backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lỗi nghiệp vụ chuẩn. Service bọc chúng bằng fmt.Errorf("%w: ...") để handler map sang HTTP status.
var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrInsufficientBalance = errors.New("insufficient balance")
)

// withTx chạy fn trong một transaction; rollback nếu fn lỗi hoặc commit lỗi
// (ràng buộc DEFERRABLE như kiểm tra cân sổ cái sẽ báo lỗi tại thời điểm commit).
func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *db.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(db.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// isUniqueViolation trả về true nếu err là vi phạm UNIQUE của constraint/index có tên name.
func isUniqueViolation(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == name
}

func ptr[T any](v T) *T {
	return &v
}
