-- ============================================================
-- 07. INTEGRITY HARDENING (UP)
-- Siết ràng buộc dữ liệu cho các domain đã tạo ở 000001 - 000006.
-- ============================================================

-- ------------------------------------------------------------
-- 0. Tự động cập nhật updated_at cho mọi bảng có cột này
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'users', 'user_sessions', 'wallets', 'economy_transactions', 'cashout_requests',
        'rooms', 'room_reaction_stats', 'gifts', 'inventories', 'pk_battles',
        'games', 'game_sessions', 'game_players'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER trg_%1$s_updated_at BEFORE UPDATE ON %1$I FOR EACH ROW EXECUTE FUNCTION set_updated_at()',
            t
        );
    END LOOP;
END $$;

-- ------------------------------------------------------------
-- 1. ACCOUNT: email/username không phân biệt hoa thường
-- ------------------------------------------------------------
CREATE UNIQUE INDEX idx_users_email_lower ON users (LOWER(email));
CREATE UNIQUE INDEX idx_users_username_lower ON users (LOWER(username));

-- ------------------------------------------------------------
-- 2. ECONOMY
-- ------------------------------------------------------------

-- 2.1 Ví USER không được âm. Ví SYSTEM là tài khoản đối ứng nên được phép âm
--     (VD: FIAT_RESERVE âm = tổng COIN đã phát hành qua nạp tiền).
--     Bất biến: với mỗi currency, tổng số dư của mọi ví luôn = 0.
ALTER TABLE wallets
    DROP CONSTRAINT wallets_coin_check,
    DROP CONSTRAINT wallets_gem_check,
    ADD CONSTRAINT check_user_wallet_non_negative
        CHECK (account_type <> 'USER' OR (coin >= 0 AND gem >= 0));

-- 2.2 Giao dịch COMPLETED phải có completed_at
ALTER TABLE economy_transactions
    ADD CONSTRAINT check_eco_tx_completed_at
        CHECK (status <> 'COMPLETED' OR completed_at IS NOT NULL);

-- 2.3 Sổ cái: số dư trước/sau bắt buộc và phải khớp phép tính
ALTER TABLE economy_ledger_entries
    ALTER COLUMN balance_before SET NOT NULL,
    ALTER COLUMN balance_after SET NOT NULL,
    ADD CONSTRAINT check_ledger_balance_math CHECK (
        (entry_type = 'CREDIT' AND balance_after = balance_before + amount) OR
        (entry_type = 'DEBIT'  AND balance_after = balance_before - amount)
    );

-- 2.4 Không cho xóa transaction đã có bút toán (trước đây là ON DELETE CASCADE)
ALTER TABLE economy_ledger_entries
    DROP CONSTRAINT economy_ledger_entries_transaction_id_fkey,
    ADD CONSTRAINT economy_ledger_entries_transaction_id_fkey
        FOREIGN KEY (transaction_id) REFERENCES economy_transactions(id) ON DELETE RESTRICT;

-- 2.5 Sổ cái bất biến: cấm UPDATE/DELETE bút toán
CREATE OR REPLACE FUNCTION forbid_ledger_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'economy_ledger_entries is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_immutable
    BEFORE UPDATE OR DELETE ON economy_ledger_entries
    FOR EACH ROW EXECUTE FUNCTION forbid_ledger_mutation();

-- 2.6 Kế toán kép: tại thời điểm COMMIT, mỗi transaction phải cân (tổng DEBIT = tổng CREDIT theo từng currency)
CREATE OR REPLACE FUNCTION check_ledger_transaction_balanced() RETURNS trigger AS $$
DECLARE
    bad_currency currency_type;
BEGIN
    SELECT currency INTO bad_currency
    FROM economy_ledger_entries
    WHERE transaction_id = NEW.transaction_id
    GROUP BY currency
    HAVING SUM(CASE WHEN entry_type = 'CREDIT' THEN amount ELSE -amount END) <> 0
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'ledger transaction % is unbalanced for currency %', NEW.transaction_id, bad_currency;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_ledger_balanced
    AFTER INSERT ON economy_ledger_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_ledger_transaction_balanced();

-- 2.7 Idempotency key luôn gắn với một user
ALTER TABLE idempotency_keys ALTER COLUMN user_id SET NOT NULL;

-- ------------------------------------------------------------
-- 3. LIVESTREAM
-- ------------------------------------------------------------

-- 3.1 Mỗi host chỉ có tối đa 1 phòng đang LIVE
CREATE UNIQUE INDEX idx_rooms_one_live_per_host ON rooms (host_id) WHERE status = 'LIVE';

-- 3.2 Mốc thời gian phải khớp trạng thái
ALTER TABLE rooms
    ADD CONSTRAINT check_room_started_at CHECK (status NOT IN ('LIVE', 'ENDED') OR started_at IS NOT NULL),
    ADD CONSTRAINT check_room_ended_at CHECK (status <> 'ENDED' OR ended_at IS NOT NULL),
    ADD CONSTRAINT check_room_time_order CHECK (ended_at IS NULL OR started_at IS NULL OR ended_at >= started_at),
    -- Khóa phụ để các bảng khác ràng buộc (room_id, host_id) đúng cặp
    ADD CONSTRAINT unique_room_host UNIQUE (id, host_id);

-- 3.3 Nội dung chat không rỗng, giới hạn độ dài
ALTER TABLE chat_messages
    ADD CONSTRAINT check_chat_message_length CHECK (char_length(message) BETWEEN 1 AND 1000);

-- ------------------------------------------------------------
-- 4. GIFT
-- ------------------------------------------------------------
ALTER TABLE gift_logs
    ADD CONSTRAINT check_gift_not_self CHECK (sender_id <> receiver_id);

-- ------------------------------------------------------------
-- 5. PK BATTLE
-- ------------------------------------------------------------

-- 5.1 Side phải thuộc đúng battle được tham chiếu
ALTER TABLE pk_sides
    ADD CONSTRAINT check_pk_side_number_positive CHECK (side_number > 0),
    ADD CONSTRAINT unique_pk_side_battle UNIQUE (id, battle_id);

ALTER TABLE pk_score_events
    ADD CONSTRAINT fk_pk_score_event_side_battle
        FOREIGN KEY (side_id, battle_id) REFERENCES pk_sides(id, battle_id) ON DELETE CASCADE;

-- Winner side phải là side của chính battle đó
ALTER TABLE pk_battles
    DROP CONSTRAINT fk_pk_winner_side,
    ADD CONSTRAINT fk_pk_winner_side
        FOREIGN KEY (winner_side_id, id) REFERENCES pk_sides(id, battle_id);

-- 5.2 Trạng thái và mốc thời gian
ALTER TABLE pk_battles
    ADD CONSTRAINT check_pk_started_at CHECK (status NOT IN ('ACTIVE', 'FINISHED') OR started_at IS NOT NULL),
    ADD CONSTRAINT check_pk_ended_at CHECK (status <> 'FINISHED' OR ended_at IS NOT NULL),
    ADD CONSTRAINT check_pk_winner_only_finished CHECK (winner_side_id IS NULL OR status = 'FINISHED');

-- 5.3 host_id phải đúng là host của room; một room chỉ ở trong 1 trận PK chưa kết thúc
ALTER TABLE pk_side_rooms
    ADD CONSTRAINT fk_pk_side_room_host
        FOREIGN KEY (room_id, host_id) REFERENCES rooms(id, host_id) ON DELETE CASCADE;

CREATE UNIQUE INDEX idx_pk_side_rooms_one_active ON pk_side_rooms (room_id) WHERE left_at IS NULL;

-- ------------------------------------------------------------
-- 6. MINI GAME
-- ------------------------------------------------------------

-- 6.1 host_id của session phải là host của room; mỗi room chỉ 1 session đang mở
ALTER TABLE game_sessions
    ADD CONSTRAINT fk_game_session_room_host
        FOREIGN KEY (room_id, host_id) REFERENCES rooms(id, host_id) ON DELETE CASCADE,
    ADD CONSTRAINT check_game_session_started_at CHECK (status NOT IN ('RUNNING', 'FINISHED') OR started_at IS NOT NULL),
    ADD CONSTRAINT check_game_session_ended_at CHECK (status NOT IN ('FINISHED', 'CANCELLED') OR ended_at IS NOT NULL);

CREATE UNIQUE INDEX idx_game_sessions_one_open_per_room
    ON game_sessions (room_id) WHERE status IN ('WAITING', 'RUNNING');

-- 6.2 Kết quả phải khớp (player, session, user); mỗi player chỉ có 1 kết quả (chống trả thưởng 2 lần)
ALTER TABLE game_players
    ADD CONSTRAINT unique_game_player_identity UNIQUE (id, session_id, user_id);

ALTER TABLE game_results
    ADD COLUMN reward_coin BIGINT NOT NULL DEFAULT 0 CHECK (reward_coin >= 0),
    ADD CONSTRAINT fk_game_result_player_identity
        FOREIGN KEY (player_id, session_id, user_id) REFERENCES game_players(id, session_id, user_id) ON DELETE CASCADE,
    ADD CONSTRAINT unique_game_result_player UNIQUE (player_id),
    ADD CONSTRAINT check_game_result_rewarded
        CHECK (status <> 'REWARDED' OR (economy_transaction_id IS NOT NULL AND rewarded_at IS NOT NULL));
