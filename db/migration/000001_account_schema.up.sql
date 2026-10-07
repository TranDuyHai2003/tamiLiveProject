-- ============================================================
-- 01. ACCOUNT DOMAIN SCHEMA (UP)
-- ============================================================

-- Bật extension hỗ trợ sinh UUID cho PostgreSQL
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Khởi tạo các Enum định danh
CREATE TYPE user_role AS ENUM ('VIEWER', 'HOST', 'AGENCY', 'ADMIN');
CREATE TYPE user_status AS ENUM ('ACTIVE', 'SUSPENDED', 'BANNED', 'DELETED');

-- 1. Bảng users (Trung tâm định danh)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role user_role NOT NULL DEFAULT 'VIEWER',
    status user_status NOT NULL DEFAULT 'ACTIVE',
    avatar_url VARCHAR(500),
    level INT NOT NULL DEFAULT 1 CHECK (level > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_created_at ON users(created_at);

-- 2. Bảng user_sessions (Quản lý vòng đời thiết bị/refresh token)
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash VARCHAR(255) UNIQUE NOT NULL,
    user_agent VARCHAR(1000),
    ip_address INET,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX idx_user_sessions_revoked_at ON user_sessions(revoked_at);

-- 3. Bảng follows (Mạng xã hội - Quan hệ follow)
CREATE TABLE follows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    follower_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    following_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Ràng buộc: Chặn việc 1 người bấm follow 2 lần
    CONSTRAINT unique_follow UNIQUE (follower_id, following_id),
    -- Ràng buộc: Chặn việc tự follow chính mình
    CONSTRAINT check_not_self_follow CHECK (follower_id != following_id)
);

CREATE INDEX idx_follows_follower_id ON follows(follower_id);
CREATE INDEX idx_follows_following_id ON follows(following_id);