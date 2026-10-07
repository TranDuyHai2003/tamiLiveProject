-- ============================================================
-- 05. PK BATTLE DOMAIN SCHEMA (DOWN)
-- ============================================================

-- Gỡ bỏ ràng buộc khóa ngoại vòng lặp trước khi drop bảng
ALTER TABLE pk_battles DROP CONSTRAINT IF EXISTS fk_pk_winner_side;

DROP TABLE IF EXISTS pk_score_events CASCADE;
DROP TABLE IF EXISTS pk_side_rooms CASCADE;
DROP TABLE IF EXISTS pk_sides CASCADE;
DROP TABLE IF EXISTS pk_battles CASCADE;

DROP TYPE IF EXISTS pk_score_source;
DROP TYPE IF EXISTS pk_status;