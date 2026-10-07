-- ============================================================
-- 01. ACCOUNT DOMAIN SCHEMA (DOWN)
-- ============================================================

DROP TABLE IF EXISTS follows CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;
DROP TABLE IF EXISTS users CASCADE;

DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;
