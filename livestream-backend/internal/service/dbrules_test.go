package service

import (
	"testing"

	"github.com/google/uuid"
)

// Các luật của bước 0 (S7, S11, S12) do DB ép, không phụ thuộc code Go.

func TestDBRule_ReceiptsCannotBeDeleted(t *testing.T) {
	s := newGiftScene(t, 1000)
	if _, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 1, "k1"); err != nil {
		t.Fatalf("send gift: %v", err)
	}

	for name, sql := range map[string]string{
		"gift that was sent":          `DELETE FROM gifts WHERE id = $1`,
		"room that received a gift":   `DELETE FROM rooms WHERE id = $1`,
		"user who made a transaction": `DELETE FROM users WHERE id = $1`,
	} {
		arg := map[string]uuid.UUID{
			"gift that was sent":          s.giftID,
			"room that received a gift":   s.roomID,
			"user who made a transaction": s.viewer,
		}[name]
		if _, err := s.f.pool.Exec(s.f.ctx, sql, arg); pgCode(err) != "23503" {
			t.Errorf("deleting %s: got %v, want foreign key violation (23503)", name, err)
		}
	}

	// Quà chưa ai tặng thì vẫn xóa được: chốt chặn chỉ nằm ở biên lai
	unused := s.f.createGift("Unused", 5)
	if _, err := s.f.pool.Exec(s.f.ctx, `DELETE FROM gifts WHERE id = $1`, unused); err != nil {
		t.Fatalf("deleting unused gift: %v", err)
	}
}

func TestDBRule_RefundOnlyOnceAndMustReferenceOriginal(t *testing.T) {
	s := newGiftScene(t, 1000)
	res, err := s.f.sendGift(s.viewer, s.roomID, s.giftID, 1, "k1")
	if err != nil {
		t.Fatalf("send gift: %v", err)
	}

	insertRefund := `INSERT INTO economy_transactions (type, status, reference_type, reference_id, completed_at)
	                 VALUES ('REFUND', 'COMPLETED', 'ECONOMY_TRANSACTION', $1, NOW())`

	if _, err := s.f.pool.Exec(s.f.ctx, `INSERT INTO economy_transactions (type, status, completed_at) VALUES ('REFUND', 'COMPLETED', NOW())`); pgCode(err) != "23514" {
		t.Errorf("refund without original: got %v, want check violation (23514)", err)
	}
	if _, err := s.f.pool.Exec(s.f.ctx, insertRefund, res.TransactionID); err != nil {
		t.Fatalf("first refund: %v", err)
	}
	if _, err := s.f.pool.Exec(s.f.ctx, insertRefund, res.TransactionID); pgCode(err) != "23505" {
		t.Errorf("second refund: got %v, want unique violation (23505)", err)
	}
}

func TestDBRule_UserWalletCanNeverAllowNegative(t *testing.T) {
	f := newFixture(t)
	user := f.createUser("alice")

	_, err := f.pool.Exec(f.ctx, `UPDATE wallets SET allow_negative = true WHERE user_id = $1`, user)
	if pgCode(err) != "23514" {
		t.Fatalf("got %v, want check violation (23514)", err)
	}
}

// Thưởng game khi quỹ thưởng hết: trả lỗi thiếu số dư (402), kết quả bị rollback để gửi lại được.
// Nạp quỹ rồi gửi lại thì được trả thưởng.
func TestGameReward_LimitedByRewardBudget(t *testing.T) {
	f := newFixture(t)
	host, player := f.createUser("host"), f.createUser("player")
	room := f.liveRoom(host)

	var gameID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO games (name, endpoint_url) VALUES ('Race', 'https://game.test') RETURNING id`).Scan(&gameID); err != nil {
		t.Fatalf("create game: %v", err)
	}
	if _, err := f.rooms.JoinRoom(f.ctx, room.ID, player); err != nil {
		t.Fatalf("join room: %v", err)
	}
	session, err := f.games.CreateSession(f.ctx, host, room.ID, gameID, 0)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := f.games.JoinSession(f.ctx, session.ID, player); err != nil {
		t.Fatalf("join session: %v", err)
	}
	if err := f.games.StartSession(f.ctx, session.ID, host); err != nil {
		t.Fatalf("start session: %v", err)
	}

	result := GameResultInput{
		SessionID:  session.ID,
		UserID:     player,
		Score:      42,
		RewardCoin: 500,
		Signature:  SignGameResult([]byte(testGameSecret), session.ID, player, 42, 500),
	}

	_, err = f.games.SubmitResult(f.ctx, result)
	wantErrIs(t, err, ErrInsufficientBalance)
	if n := f.count("game_results"); n != 0 {
		t.Fatalf("game_results = %d after failed reward, want 0", n)
	}

	f.fundRewardBudget(1000)
	if _, err := f.games.SubmitResult(f.ctx, result); err != nil {
		t.Fatalf("submit after funding: %v", err)
	}
	if got := f.userCoin(player); got != 500 {
		t.Fatalf("player coin = %d, want 500", got)
	}
	if coin, _ := f.systemBalance(walletSystemReward); coin != 500 {
		t.Fatalf("SYSTEM_REWARD coin = %d, want 500", coin)
	}
}

func TestGameReward_RejectsBadSignature(t *testing.T) {
	f := newFixture(t)
	_, err := f.games.SubmitResult(f.ctx, GameResultInput{
		SessionID: uuid.New(), UserID: uuid.New(), Score: 1, RewardCoin: 1_000_000, Signature: "forged",
	})
	wantErrIs(t, err, ErrUnauthorized)
}
