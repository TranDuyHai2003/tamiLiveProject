-- ============================================================
-- 02. ECONOMY DOMAIN SCHEMA (DOWN)
-- ============================================================

DROP TABLE IF EXISTS cashout_requests CASCADE;
DROP TABLE IF EXISTS idempotency_keys CASCADE;
DROP TABLE IF EXISTS economy_ledger_entries CASCADE;
DROP TABLE IF EXISTS economy_transactions CASCADE;
DROP TABLE IF EXISTS wallets CASCADE;

DROP TYPE IF EXISTS cashout_status;
DROP TYPE IF EXISTS idempotency_status;
DROP TYPE IF EXISTS ledger_entry_type;
DROP TYPE IF EXISTS economy_transaction_status;
DROP TYPE IF EXISTS economy_transaction_type;
DROP TYPE IF EXISTS account_type;
DROP TYPE IF EXISTS currency_type;