-- ============================================================
-- 06. MINI GAME DOMAIN SCHEMA (DOWN)
-- ============================================================

DROP TABLE IF EXISTS game_results CASCADE;
DROP TABLE IF EXISTS game_players CASCADE;
DROP TABLE IF EXISTS game_sessions CASCADE;
DROP TABLE IF EXISTS games CASCADE;

DROP TYPE IF EXISTS game_result_status;
DROP TYPE IF EXISTS game_player_status;
DROP TYPE IF EXISTS game_session_status;
DROP TYPE IF EXISTS game_status;
