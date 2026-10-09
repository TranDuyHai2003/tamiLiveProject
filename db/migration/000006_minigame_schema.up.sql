-- ============================================================
-- 06. MINI GAME DOMAIN SCHEMA (UP)
-- ============================================================

-- Khởi tạo các Enum cho Mini Game
CREATE TYPE game_status AS ENUM ('ACTIVE', 'INACTIVE');
CREATE TYPE game_session_status AS ENUM ('WAITING', 'RUNNING', 'FINISHED', 'CANCELLED');
CREATE TYPE game_player_status AS ENUM ('REQUESTED', 'APPROVED', 'PLAYING', 'LEFT', 'KICKED', 'FINISHED');
CREATE TYPE game_result_status AS ENUM ('PENDING', 'VALIDATED', 'REJECTED', 'REWARDED');

-- 1. Bảng games (Danh mục trò chơi bên thứ ba cung cấp)
CREATE TABLE games (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    provider VARCHAR(100),
    endpoint_url VARCHAR(1000) NOT NULL,
    status game_status NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_games_provider ON games(provider);
CREATE INDEX idx_games_status ON games(status);

CREATE TRIGGER trg_games_updated_at BEFORE UPDATE ON games
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2. Bảng game_sessions (Phiên chơi game đang diễn ra bên trong một phòng livestream)
CREATE TABLE game_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    game_id UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status game_session_status NOT NULL DEFAULT 'WAITING',
    max_players INT NOT NULL DEFAULT 4 CHECK (max_players > 0),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- host_id của session phải là host của room
    CONSTRAINT fk_game_session_room_host FOREIGN KEY (room_id, host_id) REFERENCES rooms(id, host_id) ON DELETE CASCADE,
    CONSTRAINT check_game_session_started_at CHECK (status NOT IN ('RUNNING', 'FINISHED') OR started_at IS NOT NULL),
    CONSTRAINT check_game_session_ended_at CHECK (status NOT IN ('FINISHED', 'CANCELLED') OR ended_at IS NOT NULL)
);

CREATE INDEX idx_game_sessions_room ON game_sessions(room_id);
CREATE INDEX idx_game_sessions_game ON game_sessions(game_id);
CREATE INDEX idx_game_sessions_status ON game_sessions(status);
-- Mỗi room chỉ có 1 session đang mở
CREATE UNIQUE INDEX idx_game_sessions_one_open_per_room
    ON game_sessions (room_id) WHERE status IN ('WAITING', 'RUNNING');

CREATE TRIGGER trg_game_sessions_updated_at BEFORE UPDATE ON game_sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3. Bảng game_players (Danh sách người tham gia và vị trí ghế ngồi)
CREATE TABLE game_players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status game_player_status NOT NULL DEFAULT 'REQUESTED',
    seat_number INT CHECK (seat_number > 0),
    joined_at TIMESTAMPTZ,
    left_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ràng buộc: Mỗi user chỉ join một lần trong một session
    CONSTRAINT unique_session_user UNIQUE (session_id, user_id),
    -- Ràng buộc: Mỗi ghế ngồi chỉ có duy nhất một người trong một session
    CONSTRAINT unique_session_seat UNIQUE (session_id, seat_number),
    -- Khóa phụ để game_results ràng buộc (player, session, user) khớp nhau
    CONSTRAINT unique_game_player_identity UNIQUE (id, session_id, user_id)
);

CREATE INDEX idx_game_players_session ON game_players(session_id);
CREATE INDEX idx_game_players_user ON game_players(user_id);

CREATE TRIGGER trg_game_players_updated_at BEFORE UPDATE ON game_players
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 4. Bảng game_results (Kết quả từ bên thứ ba gửi về - Chờ xác thực chữ ký trước khi trả thưởng)
CREATE TABLE game_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
    player_id UUID NOT NULL REFERENCES game_players(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    score BIGINT NOT NULL DEFAULT 0 CHECK (score >= 0),
    reward_coin BIGINT NOT NULL DEFAULT 0 CHECK (reward_coin >= 0),
    result_payload JSONB,
    signature VARCHAR(500) NOT NULL,
    status game_result_status NOT NULL DEFAULT 'PENDING',
    economy_transaction_id UUID REFERENCES economy_transactions(id) ON DELETE SET NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    validated_at TIMESTAMPTZ,
    rewarded_at TIMESTAMPTZ,

    -- (player, session, user) phải khớp với game_players
    CONSTRAINT fk_game_result_player_identity
        FOREIGN KEY (player_id, session_id, user_id) REFERENCES game_players(id, session_id, user_id) ON DELETE CASCADE,
    -- Mỗi player chỉ có 1 kết quả => không thể nhận thưởng 2 lần
    CONSTRAINT unique_game_result_player UNIQUE (player_id),
    CONSTRAINT check_game_result_rewarded
        CHECK (status <> 'REWARDED' OR (economy_transaction_id IS NOT NULL AND rewarded_at IS NOT NULL))
);

CREATE INDEX idx_game_results_session ON game_results(session_id);
CREATE INDEX idx_game_results_user ON game_results(user_id);
CREATE INDEX idx_game_results_status ON game_results(status);
