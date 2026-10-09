package service

import (
	"testing"

	"github.com/google/uuid"
)

// giftScene: một host đang live, một viewer có sẵn coin, một món quà giá 10.
type giftScene struct {
	f      *fixture
	host   uuid.UUID
	viewer uuid.UUID
	roomID uuid.UUID
	giftID uuid.UUID
}

func newGiftScene(t *testing.T, viewerCoin int64) giftScene {
	f := newFixture(t)
	host := f.createUser("host")
	viewer := f.createUser("viewer")
	if viewerCoin > 0 {
		f.topup(viewer, viewerCoin)
	}
	return giftScene{f: f, host: host, viewer: viewer, roomID: f.liveRoom(host).ID, giftID: f.createGift("Rose", 10)}
}

func TestSendGift_MovesCoinToRevenueAndIssuesGemToHost(t *testing.T) {
	s := newGiftScene(t, 1000)

	res, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1")
	if err != nil {
		t.Fatalf("send gift: %v", err)
	}

	// 3 x 10 = 30 coin; rate 50% → 15 gem
	if res.TotalCoin != 30 || res.TotalGem != 15 || res.SenderCoinBalance != 970 || res.Replayed {
		t.Fatalf("result = %+v", res)
	}
	if got := s.f.userCoin(s.viewer); got != 970 {
		t.Fatalf("viewer coin = %d, want 970", got)
	}
	if got := s.f.userGem(s.host); got != 15 {
		t.Fatalf("host gem = %d, want 15", got)
	}
	if coin, _ := s.f.systemBalance(walletPlatformRevenue); coin != 30 {
		t.Fatalf("PLATFORM_REVENUE coin = %d, want 30", coin)
	}
	if _, gem := s.f.systemBalance(walletGemLiability); gem != -15 {
		t.Fatalf("GEM_LIABILITY gem = %d, want -15", gem)
	}

	var txType string
	if err := s.f.pool.QueryRow(s.f.ctx, `SELECT type FROM economy_transactions WHERE id = $1`, res.TransactionID).Scan(&txType); err != nil {
		t.Fatalf("read transaction: %v", err)
	}
	if txType != "GIFT" {
		t.Fatalf("transaction type = %s, want GIFT", txType)
	}
	if n := s.f.count("gift_logs"); n != 1 {
		t.Fatalf("gift_logs = %d, want 1", n)
	}
}

func TestSendGift_SameKeyReplaysWithoutChargingAgain(t *testing.T) {
	s := newGiftScene(t, 1000)

	first, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}

	if !second.Replayed || second.GiftLogID != first.GiftLogID || second.TransactionID != first.TransactionID {
		t.Fatalf("replay = %+v, want same result as %+v with Replayed=true", second, first)
	}
	if got := s.f.userCoin(s.viewer); got != 970 {
		t.Fatalf("viewer coin = %d, want 970 (charged once)", got)
	}
	if n := s.f.count("gift_logs"); n != 1 {
		t.Fatalf("gift_logs = %d, want 1", n)
	}
}

func TestSendGift_SameKeyDifferentRequestIsConflict(t *testing.T) {
	s := newGiftScene(t, 1000)

	if _, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 4, "k1")
	wantErrIs(t, err, ErrConflict)
}

func TestSendGift_InsufficientBalanceLeavesNoTrace(t *testing.T) {
	s := newGiftScene(t, 20)
	entriesBefore := s.f.count("economy_ledger_entries")
	txBefore := s.f.count("economy_transactions")

	_, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1") // cần 30, có 20
	wantErrIs(t, err, ErrInsufficientBalance)

	// Cả transaction rollback: không tiền nào đổi, không biên lai, không idempotency key
	if got := s.f.userCoin(s.viewer); got != 20 {
		t.Fatalf("viewer coin = %d, want 20", got)
	}
	if n := s.f.count("gift_logs"); n != 0 {
		t.Fatalf("gift_logs = %d, want 0", n)
	}
	if n := s.f.count("idempotency_keys"); n != 0 {
		t.Fatalf("idempotency_keys = %d, want 0 (key must roll back so the user can retry)", n)
	}
	if n := s.f.count("economy_transactions"); n != txBefore {
		t.Fatalf("economy_transactions = %d, want %d", n, txBefore)
	}
	if n := s.f.count("economy_ledger_entries"); n != entriesBefore {
		t.Fatalf("ledger entries = %d, want %d", n, entriesBefore)
	}

	// Nạp thêm rồi gửi lại cùng key → thành công
	s.f.topup(s.viewer, 100)
	if _, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1"); err != nil {
		t.Fatalf("retry after topup: %v", err)
	}
}

func TestSendGift_CannotGiftYourself(t *testing.T) {
	s := newGiftScene(t, 0)
	s.f.topup(s.host, 100)

	_, err := s.f.sendGift(s.host, s.roomID, s.giftID, 1, "k1")
	wantErrIs(t, err, ErrInvalidInput)
}

func TestSendGift_RoomMustBeLive(t *testing.T) {
	s := newGiftScene(t, 1000)
	if _, err := s.f.rooms.EndRoom(s.f.ctx, s.roomID, s.host); err != nil {
		t.Fatalf("end room: %v", err)
	}

	_, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 1, "k1")
	wantErrIs(t, err, ErrConflict)
}

func TestSendGift_InactiveGiftIsRejected(t *testing.T) {
	s := newGiftScene(t, 1000)
	if _, err := s.f.pool.Exec(s.f.ctx, `UPDATE gifts SET is_active = false WHERE id = $1`, s.giftID); err != nil {
		t.Fatalf("deactivate gift: %v", err)
	}

	_, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 1, "k1")
	wantErrIs(t, err, ErrNotFound)
}

func TestSendGift_AddsScoreToSideInActivePK(t *testing.T) {
	s := newGiftScene(t, 1000)
	otherHost := s.f.createUser("host2")
	otherRoom := s.f.liveRoom(otherHost)

	battleID, err := s.f.pk.StartBattle(s.f.ctx, [][]uuid.UUID{{s.roomID}, {otherRoom.ID}})
	if err != nil {
		t.Fatalf("start battle: %v", err)
	}

	res, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 3, "k1")
	if err != nil {
		t.Fatalf("send gift: %v", err)
	}
	if res.PKBattleID == nil || *res.PKBattleID != battleID {
		t.Fatalf("PKBattleID = %v, want %s", res.PKBattleID, battleID)
	}

	var side1, side2 int64
	err = s.f.pool.QueryRow(s.f.ctx, `
		SELECT MAX(score) FILTER (WHERE side_number = 1), MAX(score) FILTER (WHERE side_number = 2)
		FROM pk_sides WHERE battle_id = $1`, battleID).Scan(&side1, &side2)
	if err != nil {
		t.Fatalf("read scores: %v", err)
	}
	if side1 != 30 || side2 != 0 {
		t.Fatalf("scores = %d / %d, want 30 / 0", side1, side2)
	}
	if n := s.f.count("pk_score_events"); n != 1 {
		t.Fatalf("pk_score_events = %d, want 1", n)
	}
}
