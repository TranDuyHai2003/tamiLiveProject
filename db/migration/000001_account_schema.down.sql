-- ============================================================
-- 01. ACCOUNT DOMAIN SCHEMA (DOWN)
-- ============================================================

DROP TABLE IF EXISTS follows CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;
DROP TABLE IF EXISTS users CASCADE;

DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;

-- Dùng chung cho mọi migration sau nên drop cuối cùng (down chạy ngược 06 -> 01)
DROP FUNCTION IF EXISTS set_updated_at();
