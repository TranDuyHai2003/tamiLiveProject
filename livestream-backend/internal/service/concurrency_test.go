package service

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// runConcurrently chạy n hàm cùng lúc (cùng xuất phát) và trả về lỗi của từng hàm.
func runConcurrently(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			errs[i] = fn(i)
		}(i)
	}
	start.Done()
	done.Wait()
	return errs
}

// Ví có 100 coin, 20 request tặng quà 10 coin chạy cùng lúc:
// đúng 10 cái thành công, 10 cái thiếu tiền, ví về 0, không bao giờ âm (chống double-spend).
func TestConcurrent_SpendingNeverOverdraws(t *testing.T) {
	s := newGiftScene(t, 100)

	errs := runConcurrently(20, func(i int) error {
		_, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 1, fmt.Sprintf("k%d", i))
		return err
	})

	var ok, insufficient int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrInsufficientBalance):
			insufficient++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 10 || insufficient != 10 {
		t.Fatalf("ok = %d, insufficient = %d, want 10 and 10", ok, insufficient)
	}
	if got := s.f.userCoin(s.viewer); got != 0 {
		t.Fatalf("viewer coin = %d, want 0", got)
	}
}

// 10 request cùng một idempotency key chạy cùng lúc: chỉ trừ tiền đúng một lần.
func TestConcurrent_SameKeyChargesOnce(t *testing.T) {
	s := newGiftScene(t, 1000)

	results := make([]SendGiftResult, 10)
	errs := runConcurrently(10, func(i int) error {
		var err error
		results[i], err = s.f.sendGift(s.viewer, s.roomID, s.giftID, 1, "same-key")
		return err
	})

	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	for _, r := range results {
		if r.GiftLogID != results[0].GiftLogID {
			t.Fatalf("requests returned different gift logs: %s vs %s", r.GiftLogID, results[0].GiftLogID)
		}
	}
	if got := s.f.userCoin(s.viewer); got != 990 {
		t.Fatalf("viewer coin = %d, want 990 (charged once)", got)
	}
	if n := s.f.count("gift_logs"); n != 1 {
		t.Fatalf("gift_logs = %d, want 1", n)
	}
}

// A tặng quà trong phòng của B và B tặng quà trong phòng của A cùng lúc, nhiều lần.
// Nếu postLedger không khóa ví theo thứ tự cố định, hai transaction sẽ chờ nhau (deadlock)
// và Postgres hủy một bên với lỗi 40P01.
func TestConcurrent_CrossGiftingDoesNotDeadlock(t *testing.T) {
	f := newFixture(t)
	a, b := f.createUser("alice"), f.createUser("bob")
	f.topup(a, 1000)
	f.topup(b, 1000)
	roomA, roomB := f.liveRoom(a).ID, f.liveRoom(b).ID
	gift := f.createGift("Rose", 10)

	const pairs = 20
	errs := runConcurrently(pairs*2, func(i int) error {
		sender, room := a, roomB
		if i%2 == 1 {
			sender, room = b, roomA
		}
		_, err := f.sendGift(sender, room, gift, 1, fmt.Sprintf("k%d", i))
		return err
	})

	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d failed (pg code %q): %v", i, pgCode(err), err)
		}
	}
	// Mỗi người tặng 20 lần x 10 coin
	for _, u := range []uuid.UUID{a, b} {
		if got := f.userCoin(u); got != 800 {
			t.Fatalf("coin = %d, want 800", got)
		}
	}
}
