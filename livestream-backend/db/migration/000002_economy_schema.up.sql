-- ============================================================
-- 02. ECONOMY DOMAIN SCHEMA (UP)
-- ============================================================

-- 1. Khởi tạo các Enum cho Economy
CREATE TYPE currency_type AS ENUM ('COIN', 'GEM');
CREATE TYPE account_type AS ENUM ('USER', 'SYSTEM_REVENUE', 'SYSTEM_LIABILITY', 'SYSTEM_REWARD', 'SYSTEM_ESCROW', 'SYSTEM_RESERVE', 'SYSTEM_PROMO');
-- Mỗi giá trị = một sự kiện nghiệp vụ = một economy_transaction (có thể nhiều bút toán).
-- Rút tiền gồm 3 sự kiện ở 3 thời điểm khác nhau nên có 3 loại riêng.
CREATE TYPE economy_transaction_type AS ENUM (
    'TOPUP',           -- nạp tiền
    'GIFT',            -- tặng quà (1 giao dịch, 4 bút toán)
    'GAME_REWARD',     -- thưởng mini game
    'REFUND',          -- admin hoàn coin do lỗi hệ thống
    'CHARGEBACK',      -- store/ngân hàng ép hoàn tiền nạp
    'CASHOUT_HOLD',    -- host yêu cầu rút: giữ GEM vào escrow
    'CASHOUT_PAYOUT',  -- đã chuyển khoản: xóa nợ GEM
    'CASHOUT_RELEASE', -- từ chối/thất bại: trả GEM cho host
    'ADJUSTMENT'       -- điều chỉnh thủ công, VD nạp ngân sách thưởng
);
CREATE TYPE economy_transaction_status AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED', 'CANCELLED', 'REFUNDED');
CREATE TYPE ledger_entry_type AS ENUM ('DEBIT', 'CREDIT');
CREATE TYPE idempotency_status AS ENUM ('PROCESSING', 'COMPLETED', 'FAILED');
CREATE TYPE cashout_status AS ENUM ('REQUESTED', 'PROCESSING', 'COMPLETED', 'FAILED', 'REJECTED', 'CANCELLED');

-- 2. Bảng wallets (Tài khoản ví cá nhân và ví hệ thống)
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- RESTRICT: ví là dữ liệu tài chính, không bao giờ bị xóa theo user (user chỉ xóa mềm)
    user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    account_type account_type NOT NULL,
    account_code VARCHAR(100) UNIQUE,
    coin BIGINT NOT NULL DEFAULT 0,
    gem BIGINT NOT NULL DEFAULT 0,
    -- true  = ví PHÁT HÀNH / đối ứng: âm là trạng thái tự nhiên (VD FIAT_RESERVE âm = tổng COIN đã bán ra)
    -- false = ví QUỸ hoặc ví user: số dư là tiền thật đang có, không được âm
    -- Câu SQL trừ tiền (ApplyWallet*Delta) đọc chính cột này, nên luật chỉ nằm ở một chỗ.
    allow_negative BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ràng buộc: Ví USER bắt buộc có user_id, Ví SYSTEM bắt buộc user_id bằng NULL
    CONSTRAINT check_wallet_owner CHECK (
        (account_type = 'USER' AND user_id IS NOT NULL AND account_code IS NULL) OR
        (account_type != 'USER' AND user_id IS NULL AND account_code IS NOT NULL)
    ),
    -- Ví không được âm thì số dư luôn >= 0.
    -- Bất biến toàn hệ thống: với mỗi currency, tổng số dư của mọi ví luôn = 0.
    CONSTRAINT check_wallet_non_negative CHECK (
        allow_negative OR (coin >= 0 AND gem >= 0)
    ),
    -- Khóa cứng: ví USER KHÔNG BAO GIỜ được âm, kể cả khi ai đó lỡ UPDATE cột allow_negative.
    -- Đây là chốt chặn chống tiêu quá số dư (double-spend).
    CONSTRAINT check_user_wallet_never_negative CHECK (
        account_type <> 'USER' OR NOT allow_negative
    )
);

CREATE UNIQUE INDEX idx_wallets_user_id ON wallets(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_wallets_account_type ON wallets(account_type);

CREATE TRIGGER trg_wallets_updated_at BEFORE UPDATE ON wallets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3. Seed các System Accounts mặc định ngay khi khởi tạo Database
-- Giúp hệ thống luôn có địa chỉ hạch toán cố định cho các bút toán kế toán kép
--   Ví phát hành / đối ứng (được âm):
--     FIAT_RESERVE     âm = tổng COIN đã bán ra
--     GEM_LIABILITY    âm = tổng GEM đang nợ host
--     PLATFORM_REVENUE âm = đã chi (hoàn tiền) nhiều hơn đã thu; cho âm để luôn hoàn được tiền khi nền tảng gây lỗi
--     PROMO_BUDGET     âm = tổng đã chi cho khuyến mãi/thưởng
--   Ví quỹ (không được âm):
--     SYSTEM_REWARD    quỹ thưởng còn lại; chỉ tăng khi admin nạp từ PROMO_BUDGET (ADJUSTMENT)
--     CASHOUT_ESCROW   GEM đang tạm giữ chờ rút
INSERT INTO wallets (account_type, account_code, coin, gem, allow_negative) VALUES
('SYSTEM_REVENUE',   'PLATFORM_REVENUE', 0, 0, TRUE),
('SYSTEM_LIABILITY', 'GEM_LIABILITY',    0, 0, TRUE),
('SYSTEM_REWARD',    'SYSTEM_REWARD',    0, 0, FALSE),
('SYSTEM_ESCROW',    'CASHOUT_ESCROW',   0, 0, FALSE),
('SYSTEM_RESERVE',   'FIAT_RESERVE',     0, 0, TRUE),
('SYSTEM_PROMO',     'PROMO_BUDGET',     0, 0, TRUE);

-- 4. Bảng economy_transactions (Ghi nhận nghiệp vụ tài chính tổng thể)
CREATE TABLE economy_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type economy_transaction_type NOT NULL,
    status economy_transaction_status NOT NULL DEFAULT 'PENDING',
    reference_type VARCHAR(50),
    reference_id UUID,
    provider VARCHAR(100),
    provider_txn_id VARCHAR(255) UNIQUE,
    idempotency_key VARCHAR(255),
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    -- Giao dịch COMPLETED phải có completed_at
    CONSTRAINT check_eco_tx_completed_at CHECK (status <> 'COMPLETED' OR completed_at IS NOT NULL),
    -- Hoàn tiền phải chỉ rõ giao dịch gốc được hoàn
    CONSTRAINT check_refund_has_reference CHECK (
        type <> 'REFUND' OR (reference_type IS NOT NULL AND reference_id IS NOT NULL)
    )
);

CREATE INDEX idx_eco_tx_type ON economy_transactions(type);
CREATE INDEX idx_eco_tx_status ON economy_transactions(status);
CREATE INDEX idx_eco_tx_ref ON economy_transactions(reference_type, reference_id);
-- Mỗi giao dịch gốc chỉ được hoàn tiền MỘT lần (admin lỡ bấm "Hoàn" 2 lần thì lần sau bị chặn)
CREATE UNIQUE INDEX idx_eco_tx_refund_once ON economy_transactions(reference_id) WHERE type = 'REFUND';

CREATE TRIGGER trg_economy_transactions_updated_at BEFORE UPDATE ON economy_transactions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 5. Bảng economy_ledger_entries (Sổ cái bất biến - Double-Entry Ledger)
CREATE TABLE economy_ledger_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- RESTRICT: không cho xóa transaction đã có bút toán
    transaction_id UUID NOT NULL REFERENCES economy_transactions(id) ON DELETE RESTRICT,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    currency currency_type NOT NULL,
    entry_type ledger_entry_type NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    description VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Số dư trước/sau phải khớp phép tính
    CONSTRAINT check_ledger_balance_math CHECK (
        (entry_type = 'CREDIT' AND balance_after = balance_before + amount) OR
        (entry_type = 'DEBIT'  AND balance_after = balance_before - amount)
    )
);

CREATE INDEX idx_ledger_tx ON economy_ledger_entries(transaction_id);
CREATE INDEX idx_ledger_wallet ON economy_ledger_entries(wallet_id);
CREATE INDEX idx_ledger_wallet_currency ON economy_ledger_entries(wallet_id, currency, created_at);

-- Sổ cái bất biến: cấm UPDATE/DELETE bút toán
CREATE OR REPLACE FUNCTION forbid_ledger_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'economy_ledger_entries is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_immutable
    BEFORE UPDATE OR DELETE ON economy_ledger_entries
    FOR EACH ROW EXECUTE FUNCTION forbid_ledger_mutation();

-- Kế toán kép: tại thời điểm COMMIT, mỗi transaction phải cân (tổng DEBIT = tổng CREDIT theo từng currency).
-- DEFERRED vì các bút toán được INSERT lần lượt; giữa chừng transaction chắc chắn đang lệch.
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

-- 6. Bảng idempotency_keys (Chống đúp request tài chính)
CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(128) NOT NULL,
    status idempotency_status NOT NULL DEFAULT 'PROCESSING',
    response_status INT,
    response_body JSONB,
    transaction_id UUID REFERENCES economy_transactions(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT unique_user_idempotency UNIQUE (user_id, key)
);

CREATE INDEX idx_idempotency_key ON idempotency_keys(key);

-- 7. Bảng cashout_requests (Quản lý luồng rút tiền & ví tạm giữ Escrow)
CREATE TABLE cashout_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    economy_transaction_id UUID REFERENCES economy_transactions(id),
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency currency_type NOT NULL DEFAULT 'GEM',
    status cashout_status NOT NULL DEFAULT 'REQUESTED',
    payout_provider VARCHAR(100),
    payout_reference VARCHAR(255),
    payout_account JSONB,
    rejection_reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX idx_cashout_user ON cashout_requests(user_id);
CREATE INDEX idx_cashout_status ON cashout_requests(status);

CREATE TRIGGER trg_cashout_requests_updated_at BEFORE UPDATE ON cashout_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
