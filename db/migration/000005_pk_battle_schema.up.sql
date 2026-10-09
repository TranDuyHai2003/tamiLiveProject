-- ============================================================
-- 05. PK BATTLE DOMAIN SCHEMA (UP)
-- ============================================================

-- Khởi tạo các Enum cho PK Battle
CREATE TYPE pk_status AS ENUM ('PENDING', 'COUNTDOWN', 'ACTIVE', 'FINISHED', 'CANCELLED');
CREATE TYPE pk_score_source AS ENUM ('GIFT', 'BONUS', 'GAME', 'MANUAL_ADJUSTMENT');

-- 1. Bảng pk_battles (Quản lý tổng quan một trận đối kháng)
CREATE TABLE pk_battles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    status pk_status NOT NULL DEFAULT 'PENDING',
    winner_side_id UUID, -- Sẽ gắn khóa ngoại (ALTER TABLE) sau để tránh vòng lặp tham chiếu
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Trạng thái và mốc thời gian
    CONSTRAINT check_pk_started_at CHECK (status NOT IN ('ACTIVE', 'FINISHED') OR started_at IS NOT NULL),
    CONSTRAINT check_pk_ended_at CHECK (status <> 'FINISHED' OR ended_at IS NOT NULL),
    CONSTRAINT check_pk_winner_only_finished CHECK (winner_side_id IS NULL OR status = 'FINISHED')
);

CREATE INDEX idx_pk_battles_status ON pk_battles(status);
CREATE INDEX idx_pk_battles_created ON pk_battles(created_at);

CREATE TRIGGER trg_pk_battles_updated_at BEFORE UPDATE ON pk_battles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2. Bảng pk_sides (Các phe tham gia chiến đấu)
CREATE TABLE pk_sides (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    battle_id UUID NOT NULL REFERENCES pk_battles(id) ON DELETE CASCADE,
    side_number INT NOT NULL,
    score BIGINT NOT NULL DEFAULT 0 CHECK (score >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_battle_side UNIQUE (battle_id, side_number),
    CONSTRAINT check_pk_side_number_positive CHECK (side_number > 0),
    -- Khóa phụ để bảng khác ràng buộc "side này thuộc đúng battle kia"
    CONSTRAINT unique_pk_side_battle UNIQUE (id, battle_id)
);

CREATE INDEX idx_pk_sides_battle ON pk_sides(battle_id);

-- Bổ sung Khóa ngoại cho winner_side_id sau khi bảng pk_sides đã tồn tại.
-- Khóa kép (winner_side_id, id): winner phải là side của CHÍNH trận này, không phải side của trận khác.
ALTER TABLE pk_battles
ADD CONSTRAINT fk_pk_winner_side
FOREIGN KEY (winner_side_id, id) REFERENCES pk_sides(id, battle_id);

-- 3. Bảng pk_side_rooms (Ghép nối nhiều phòng live vào một phe - Hỗ trợ Team PK 2v2/3v3)
CREATE TABLE pk_side_rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    side_id UUID NOT NULL REFERENCES pk_sides(id) ON DELETE CASCADE,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,

    CONSTRAINT unique_side_room UNIQUE (side_id, room_id),
    -- host_id phải đúng là host của room
    CONSTRAINT fk_pk_side_room_host FOREIGN KEY (room_id, host_id) REFERENCES rooms(id, host_id) ON DELETE CASCADE
);

CREATE INDEX idx_pk_side_rooms_side ON pk_side_rooms(side_id);
CREATE INDEX idx_pk_side_rooms_room ON pk_side_rooms(room_id);
-- Một room chỉ ở trong 1 trận PK chưa kết thúc (left_at được set khi trận kết thúc)
CREATE UNIQUE INDEX idx_pk_side_rooms_one_active ON pk_side_rooms (room_id) WHERE left_at IS NULL;

-- 4. Bảng pk_score_events (Lịch sử audit trail chi tiết các sự kiện cộng điểm PK)
CREATE TABLE pk_score_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    battle_id UUID NOT NULL REFERENCES pk_battles(id) ON DELETE CASCADE,
    side_id UUID NOT NULL REFERENCES pk_sides(id) ON DELETE CASCADE,
    room_id UUID REFERENCES rooms(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    source pk_score_source NOT NULL,
    points BIGINT NOT NULL CHECK (points > 0),
    reference_type VARCHAR(50),
    reference_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- side_id phải thuộc đúng battle_id
    CONSTRAINT fk_pk_score_event_side_battle FOREIGN KEY (side_id, battle_id) REFERENCES pk_sides(id, battle_id) ON DELETE CASCADE
);

CREATE INDEX idx_pk_score_events_battle ON pk_score_events(battle_id, created_at);
CREATE INDEX idx_pk_score_events_side ON pk_score_events(side_id);
