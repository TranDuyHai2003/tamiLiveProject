-- ============================================================
-- 03. LIVESTREAM DOMAIN SCHEMA (DOWN)
-- ============================================================

DROP TABLE IF EXISTS room_reaction_stats CASCADE;
DROP TABLE IF EXISTS chat_messages CASCADE;
DROP TABLE IF EXISTS room_members CASCADE;
DROP TABLE IF EXISTS rooms CASCADE;

DROP TYPE IF EXISTS room_member_status;
DROP TYPE IF EXISTS room_member_role;
DROP TYPE IF EXISTS room_status;