-- ============================================================
-- 07. INTEGRITY HARDENING (DOWN)
-- ============================================================

-- 6. MINI GAME
ALTER TABLE game_results
    DROP CONSTRAINT IF EXISTS check_game_result_rewarded,
    DROP CONSTRAINT IF EXISTS unique_game_result_player,
    DROP CONSTRAINT IF EXISTS fk_game_result_player_identity,
    DROP COLUMN IF EXISTS reward_coin;
ALTER TABLE game_players DROP CONSTRAINT IF EXISTS unique_game_player_identity;
DROP INDEX IF EXISTS idx_game_sessions_one_open_per_room;
ALTER TABLE game_sessions
    DROP CONSTRAINT IF EXISTS check_game_session_ended_at,
    DROP CONSTRAINT IF EXISTS check_game_session_started_at,
    DROP CONSTRAINT IF EXISTS fk_game_session_room_host;

-- 5. PK BATTLE
DROP INDEX IF EXISTS idx_pk_side_rooms_one_active;
ALTER TABLE pk_side_rooms DROP CONSTRAINT IF EXISTS fk_pk_side_room_host;
ALTER TABLE pk_battles
    DROP CONSTRAINT IF EXISTS check_pk_winner_only_finished,
    DROP CONSTRAINT IF EXISTS check_pk_ended_at,
    DROP CONSTRAINT IF EXISTS check_pk_started_at,
    DROP CONSTRAINT IF EXISTS fk_pk_winner_side;
ALTER TABLE pk_score_events DROP CONSTRAINT IF EXISTS fk_pk_score_event_side_battle;
ALTER TABLE pk_sides
    DROP CONSTRAINT IF EXISTS unique_pk_side_battle,
    DROP CONSTRAINT IF EXISTS check_pk_side_number_positive;
ALTER TABLE pk_battles
    ADD CONSTRAINT fk_pk_winner_side
        FOREIGN KEY (winner_side_id) REFERENCES pk_sides(id) ON DELETE SET NULL;

-- 4. GIFT
ALTER TABLE gift_logs DROP CONSTRAINT IF EXISTS check_gift_not_self;

-- 3. LIVESTREAM
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS check_chat_message_length;
ALTER TABLE rooms
    DROP CONSTRAINT IF EXISTS unique_room_host,
    DROP CONSTRAINT IF EXISTS check_room_time_order,
    DROP CONSTRAINT IF EXISTS check_room_ended_at,
    DROP CONSTRAINT IF EXISTS check_room_started_at;
DROP INDEX IF EXISTS idx_rooms_one_live_per_host;

-- 2. ECONOMY
ALTER TABLE idempotency_keys ALTER COLUMN user_id DROP NOT NULL;
DROP TRIGGER IF EXISTS trg_ledger_balanced ON economy_ledger_entries;
DROP FUNCTION IF EXISTS check_ledger_transaction_balanced();
DROP TRIGGER IF EXISTS trg_ledger_immutable ON economy_ledger_entries;
DROP FUNCTION IF EXISTS forbid_ledger_mutation();
ALTER TABLE economy_ledger_entries
    DROP CONSTRAINT economy_ledger_entries_transaction_id_fkey,
    ADD CONSTRAINT economy_ledger_entries_transaction_id_fkey
        FOREIGN KEY (transaction_id) REFERENCES economy_transactions(id) ON DELETE CASCADE,
    DROP CONSTRAINT IF EXISTS check_ledger_balance_math,
    ALTER COLUMN balance_after DROP NOT NULL,
    ALTER COLUMN balance_before DROP NOT NULL;
ALTER TABLE economy_transactions DROP CONSTRAINT IF EXISTS check_eco_tx_completed_at;
-- Lưu ý: sẽ thất bại nếu ví hệ thống đang có số dư âm
ALTER TABLE wallets
    DROP CONSTRAINT IF EXISTS check_user_wallet_non_negative,
    ADD CONSTRAINT wallets_coin_check CHECK (coin >= 0),
    ADD CONSTRAINT wallets_gem_check CHECK (gem >= 0);

-- 1. ACCOUNT
DROP INDEX IF EXISTS idx_users_username_lower;
DROP INDEX IF EXISTS idx_users_email_lower;

-- 0. updated_at triggers
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
        EXECUTE format('DROP TRIGGER IF EXISTS trg_%1$s_updated_at ON %1$I', t);
    END LOOP;
END $$;
DROP FUNCTION IF EXISTS set_updated_at();
