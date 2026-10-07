-- name: CreateUserWallet :one
INSERT INTO wallets (user_id, account_type)
VALUES ($1, 'USER')
RETURNING id;

-- name: GetUserWallet :one
SELECT id, coin, gem
FROM wallets
WHERE user_id = $1 AND account_type = 'USER';

-- name: GetSystemWalletID :one
SELECT id
FROM wallets
WHERE account_code = $1 AND account_type <> 'USER';

-- Cộng/trừ số dư nguyên tử, khóa dòng ví. Ví USER không được xuống âm:
-- trả về pgx.ErrNoRows nếu không đủ số dư.
-- name: ApplyWalletCoinDelta :one
UPDATE wallets
SET coin = coin + sqlc.arg(delta)::bigint
WHERE id = sqlc.arg(id) AND (account_type <> 'USER' OR coin + sqlc.arg(delta)::bigint >= 0)
RETURNING coin;

-- name: ApplyWalletGemDelta :one
UPDATE wallets
SET gem = gem + sqlc.arg(delta)::bigint
WHERE id = sqlc.arg(id) AND (account_type <> 'USER' OR gem + sqlc.arg(delta)::bigint >= 0)
RETURNING gem;

-- name: CreateEconomyTransaction :one
INSERT INTO economy_transactions (type, status, reference_type, reference_id, idempotency_key, metadata, completed_at)
VALUES ($1, 'COMPLETED', $2, $3, $4, $5, NOW())
RETURNING id;

-- name: CreateLedgerEntry :exec
INSERT INTO economy_ledger_entries (transaction_id, wallet_id, currency, entry_type, amount, balance_before, balance_after, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertIdempotencyKey :one
INSERT INTO idempotency_keys (user_id, key, request_hash, status, expires_at)
VALUES ($1, $2, $3, 'PROCESSING', $4)
ON CONFLICT (user_id, key) DO NOTHING
RETURNING id;

-- name: GetIdempotencyKey :one
SELECT id, request_hash, status, response_status, response_body, transaction_id
FROM idempotency_keys
WHERE user_id = $1 AND key = $2;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET status = 'COMPLETED', response_status = $2, response_body = $3, transaction_id = $4, completed_at = NOW()
WHERE id = $1;
