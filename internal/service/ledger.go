package service

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"livestream-backend/internal/db"

	"github.com/google/uuid"
)

// Mã các ví hệ thống được seed ở migration 000002.
const (
	walletPlatformRevenue = "PLATFORM_REVENUE"
	walletGemLiability    = "GEM_LIABILITY"
	walletSystemReward    = "SYSTEM_REWARD"
)

// ledgerLeg là một bút toán: Delta > 0 là CREDIT (tăng số dư ví), Delta < 0 là DEBIT.
type ledgerLeg struct {
	WalletID    uuid.UUID
	Currency    db.CurrencyType
	Delta       int64
	Description string
}

// postLedger cập nhật số dư và ghi bút toán kế toán kép cho một economy transaction.
// Trả về số dư sau giao dịch của từng leg, theo đúng thứ tự đầu vào.
//
// Các ví được khóa (UPDATE) theo thứ tự (wallet_id, currency) tăng dần, nên mọi
// giao dịch tài chính đều lấy lock theo cùng một thứ tự và không thể deadlock lẫn nhau.
func postLedger(ctx context.Context, q *db.Queries, txID uuid.UUID, legs []ledgerLeg) ([]int64, error) {
	net := map[db.CurrencyType]int64{}
	for _, leg := range legs {
		if leg.Delta == 0 {
			return nil, fmt.Errorf("ledger leg with zero amount")
		}
		net[leg.Currency] += leg.Delta
	}
	for currency, sum := range net {
		if sum != 0 {
			return nil, fmt.Errorf("unbalanced ledger for %s: net %d", currency, sum)
		}
	}

	order := make([]int, len(legs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		la, lb := legs[order[a]], legs[order[b]]
		if c := bytes.Compare(la.WalletID[:], lb.WalletID[:]); c != 0 {
			return c < 0
		}
		return la.Currency < lb.Currency
	})

	balances := make([]int64, len(legs))
	for _, i := range order {
		leg := legs[i]

		var after int64
		var err error
		switch leg.Currency {
		case db.CurrencyTypeCOIN:
			after, err = q.ApplyWalletCoinDelta(ctx, db.ApplyWalletCoinDeltaParams{ID: leg.WalletID, Delta: leg.Delta})
		case db.CurrencyTypeGEM:
			after, err = q.ApplyWalletGemDelta(ctx, db.ApplyWalletGemDeltaParams{ID: leg.WalletID, Delta: leg.Delta})
		default:
			return nil, fmt.Errorf("unsupported currency %q", leg.Currency)
		}
		if isNoRows(err) {
			// Câu UPDATE chỉ không khớp dòng nào khi ví không được âm (allow_negative = false:
			// ví user hoặc ví quỹ như SYSTEM_REWARD) không đủ số dư
			return nil, fmt.Errorf("%w: not enough %s", ErrInsufficientBalance, leg.Currency)
		}
		if err != nil {
			return nil, fmt.Errorf("apply wallet delta: %w", err)
		}

		entryType, amount := db.LedgerEntryTypeCREDIT, leg.Delta
		if leg.Delta < 0 {
			entryType, amount = db.LedgerEntryTypeDEBIT, -leg.Delta
		}
		err = q.CreateLedgerEntry(ctx, db.CreateLedgerEntryParams{
			TransactionID: txID,
			WalletID:      leg.WalletID,
			Currency:      leg.Currency,
			EntryType:     entryType,
			Amount:        amount,
			BalanceBefore: after - leg.Delta,
			BalanceAfter:  after,
			Description:   ptr(leg.Description),
		})
		if err != nil {
			return nil, fmt.Errorf("create ledger entry: %w", err)
		}
		balances[i] = after
	}
	return balances, nil
}

func systemWalletID(ctx context.Context, q *db.Queries, code string) (uuid.UUID, error) {
	id, err := q.GetSystemWalletID(ctx, ptr(code))
	if err != nil {
		return uuid.Nil, fmt.Errorf("system wallet %s: %w", code, err)
	}
	return id, nil
}

func userWalletID(ctx context.Context, q *db.Queries, userID uuid.UUID) (uuid.UUID, error) {
	w, err := q.GetUserWallet(ctx, ptr(userID))
	if isNoRows(err) {
		return uuid.Nil, fmt.Errorf("%w: wallet of user %s", ErrNotFound, userID)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("get user wallet: %w", err)
	}
	return w.ID, nil
}
