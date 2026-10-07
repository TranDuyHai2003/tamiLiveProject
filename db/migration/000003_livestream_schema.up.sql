-- ============================================================
-- 03. LIVESTREAM DOMAIN SCHEMA (UP)
-- ============================================================

-- Khởi tạo các Enum cho Livestream
CREATE TYPE room_status AS ENUM ('SCHEDULED', 'LIVE', 'ENDED', 'CANCELLED');
CREATE TYPE room_member_role AS ENUM ('VIEWER', 'HOST', 'CO_HOST', 'MODERATOR');
CREATE TYPE room_member_status AS ENUM ('JOINED', 'LEFT', 'KICKED', 'BANNED');

-- 1. Bảng rooms (Phiên phát sóng livestream)
CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(100),
    status room_status NOT NULL DEFAULT 'SCHEDULED',
    livekit_room_name VARCHAR(255) UNIQUE,
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rooms_host_id ON rooms(host_id);
CREATE INDEX idx_rooms_status ON rooms(status);
CREATE INDEX idx_rooms_category ON rooms(category);
CREATE INDEX idx_rooms_created_at ON rooms(created_at);

-- 2. Bảng room_members (Lịch sử thành viên tham gia phòng live)
CREATE TABLE room_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role room_member_role NOT NULL DEFAULT 'VIEWER',
    status room_member_status NOT NULL DEFAULT 'JOINED',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,

    CONSTRAINT unique_room_member UNIQUE (room_id, user_id)
);

CREATE INDEX idx_room_members_room ON room_members(room_id);
CREATE INDEX idx_room_members_user ON room_members(user_id);

-- 3. Bảng chat_messages (Lịch sử tin nhắn chat trong phòng)
CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message TEXT NOT NULL,
    reply_to_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_chat_messages_room_time ON chat_messages(room_id, created_at);

-- 4. Bảng room_reaction_stats (Thống kê tổng hợp lượt tương tác/thả tim từ Redis)
CREATE TABLE room_reaction_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    reaction_type VARCHAR(50) NOT NULL,
    total_count BIGINT NOT NULL DEFAULT 0 CHECK (total_count >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_room_reaction UNIQUE (room_id, reaction_type)
);

CREATE INDEX idx_reaction_stats_room ON room_reaction_stats(room_id);