-- ============================================================
-- 02. ECONOMY DOMAIN SCHEMA (UP)
-- ============================================================

-- 1. Khởi tạo các Enum cho Economy
CREATE TYPE currency_type AS ENUM ('COIN', 'GEM');
CREATE TYPE account_type AS ENUM ('USER', 'SYSTEM_REVENUE', 'SYSTEM_LIABILITY', 'SYSTEM_REWARD', 'SYSTEM_ESCROW', 'SYSTEM_RESERVE');
CREATE TYPE economy_transaction_type AS ENUM ('TOPUP', 'GIFT_SEND', 'GIFT_RECEIVE', 'CASHOUT', 'GAME_REWARD', 'REFUND', 'ADJUSTMENT');
CREATE TYPE economy_transaction_status AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED', 'CANCELLED', 'REFUNDED');
CREATE TYPE ledger_entry_type AS ENUM ('DEBIT', 'CREDIT');
CREATE TYPE idempotency_status AS ENUM ('PROCESSING', 'COMPLETED', 'FAILED');
CREATE TYPE cashout_status AS ENUM ('REQUESTED', 'PROCESSING', 'COMPLETED', 'FAILED', 'REJECTED', 'CANCELLED');

-- 2. Bảng wallets (Tài khoản ví cá nhân và ví hệ thống)
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    account_type account_type NOT NULL,
    account_code VARCHAR(100) UNIQUE,
    coin BIGINT NOT NULL DEFAULT 0 CHECK (coin >= 0),
    gem BIGINT NOT NULL DEFAULT 0 CHECK (gem >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ràng buộc: Ví USER bắt buộc có user_id, Ví SYSTEM bắt buộc user_id bằng NULL
    CONSTRAINT check_wallet_owner CHECK (
        (account_type = 'USER' AND user_id IS NOT NULL AND account_code IS NULL) OR
        (account_type != 'USER' AND user_id IS NULL AND account_code IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_wallets_user_id ON wallets(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_wallets_account_type ON wallets(account_type);

-- 3. Seed các System Accounts mặc định ngay khi khởi tạo Database
-- Giúp hệ thống luôn có địa chỉ hạch toán cố định cho các bút toán kế toán kép
INSERT INTO wallets (account_type, account_code, coin, gem) VALUES
('SYSTEM_REVENUE', 'PLATFORM_REVENUE', 0, 0),
('SYSTEM_LIABILITY', 'GEM_LIABILITY', 0, 0),
('SYSTEM_REWARD', 'SYSTEM_REWARD', 0, 0),
('SYSTEM_ESCROW', 'CASHOUT_ESCROW', 0, 0),
('SYSTEM_RESERVE', 'FIAT_RESERVE', 0, 0);

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
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_eco_tx_type ON economy_transactions(type);
CREATE INDEX idx_eco_tx_status ON economy_transactions(status);
CREATE INDEX idx_eco_tx_ref ON economy_transactions(reference_type, reference_id);

-- 5. Bảng economy_ledger_entries (Sổ cái bất biến - Double-Entry Ledger)
CREATE TABLE economy_ledger_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id UUID NOT NULL REFERENCES economy_transactions(id) ON DELETE CASCADE,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    currency currency_type NOT NULL,
    entry_type ledger_entry_type NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    balance_before BIGINT,
    balance_after BIGINT,
    description VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ledger_tx ON economy_ledger_entries(transaction_id);
CREATE INDEX idx_ledger_wallet ON economy_ledger_entries(wallet_id);
CREATE INDEX idx_ledger_wallet_currency ON economy_ledger_entries(wallet_id, currency, created_at);

-- 6. Bảng idempotency_keys (Chống đúp request tài chính)
CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
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