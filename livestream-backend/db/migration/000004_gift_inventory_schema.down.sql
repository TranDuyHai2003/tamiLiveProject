-- ============================================================
-- 04. GIFT / INVENTORY DOMAIN SCHEMA (DOWN)
-- ============================================================

DROP TABLE IF EXISTS gift_logs CASCADE;
DROP TABLE IF EXISTS inventories CASCADE;
DROP TABLE IF EXISTS gifts CASCADE;

DROP TYPE IF EXISTS inventory_item_type;