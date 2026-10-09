-- ============================================================
-- 04. GIFT / INVENTORY DOMAIN SCHEMA (UP)
-- ============================================================

-- Khởi tạo Enum cho kho đồ
CREATE TYPE inventory_item_type AS ENUM ('GIFT');

-- 1. Bảng gifts (Danh mục quà tặng của nền tảng)
CREATE TABLE gifts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    coin_price BIGINT NOT NULL CHECK (coin_price > 0),
    icon_url VARCHAR(500),
    animation_url VARCHAR(500),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_gifts_is_active ON gifts(is_active);

CREATE TRIGGER trg_gifts_updated_at BEFORE UPDATE ON gifts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2. Bảng inventories (Kho đồ vật phẩm của người dùng)
CREATE TABLE inventories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_type inventory_item_type NOT NULL DEFAULT 'GIFT',
    item_id UUID NOT NULL REFERENCES gifts(id) ON DELETE CASCADE,
    quantity BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_user_inventory_item UNIQUE (user_id, item_type, item_id)
);

CREATE INDEX idx_inventories_user ON inventories(user_id);
CREATE INDEX idx_inventories_item ON inventories(item_type, item_id);

CREATE TRIGGER trg_inventories_updated_at BEFORE UPDATE ON inventories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3. Bảng gift_logs (Biên lai / Nhật ký sự kiện tặng quà)
-- Gắn kết giữa Người gửi, Người nhận, Phòng live, Quà và Giao dịch tài chính
CREATE TABLE gift_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    gift_id UUID NOT NULL REFERENCES gifts(id) ON DELETE CASCADE,
    economy_transaction_id UUID NOT NULL REFERENCES economy_transactions(id) ON DELETE CASCADE,
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    total_coin BIGINT NOT NULL CHECK (total_coin >= 0),
    total_gem BIGINT NOT NULL DEFAULT 0 CHECK (total_gem >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Không tự tặng quà cho chính mình
    CONSTRAINT check_gift_not_self CHECK (sender_id <> receiver_id)
);

CREATE INDEX idx_gift_logs_sender ON gift_logs(sender_id);
CREATE INDEX idx_gift_logs_receiver ON gift_logs(receiver_id);
CREATE INDEX idx_gift_logs_room ON gift_logs(room_id, created_at);
CREATE INDEX idx_gift_logs_tx ON gift_logs(economy_transaction_id);
