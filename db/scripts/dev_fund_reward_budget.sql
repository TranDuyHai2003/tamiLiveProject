-- ============================================================
-- DEV ONLY: nạp ngân sách thưởng mini game (chưa có API admin).
--
--   psql "$DATABASE_URL" -v amount=100000 -f db/scripts/dev_fund_reward_budget.sql
--
-- Bút toán (giao dịch ADJUSTMENT, cân theo COIN):
--   PROMO_BUDGET  −amount  (được âm: tổng đã chi cho khuyến mãi/thưởng)
--   SYSTEM_REWARD +amount  (không được âm: quỹ thưởng còn lại)
-- ============================================================

\set ON_ERROR_STOP on
\if :{?amount}
\else
  \set amount 100000
\endif

BEGIN;

WITH tx AS (
    INSERT INTO economy_transactions (type, status, reference_type, metadata, completed_at)
    VALUES ('ADJUSTMENT', 'COMPLETED', 'REWARD_BUDGET',
            jsonb_build_object('note', 'dev reward budget top-up', 'amount', :amount::bigint), NOW())
    RETURNING id
),
src AS (
    UPDATE wallets SET coin = coin - :amount::bigint
    WHERE account_code = 'PROMO_BUDGET'
    RETURNING id, coin
),
dst AS (
    UPDATE wallets SET coin = coin + :amount::bigint
    WHERE account_code = 'SYSTEM_REWARD'
    RETURNING id, coin
)
INSERT INTO economy_ledger_entries
    (transaction_id, wallet_id, currency, entry_type, amount, balance_before, balance_after, description)
SELECT tx.id, src.id, 'COIN'::currency_type, 'DEBIT'::ledger_entry_type, :amount::bigint,
       src.coin + :amount::bigint, src.coin, 'reward budget funded'
FROM tx, src
UNION ALL
SELECT tx.id, dst.id, 'COIN'::currency_type, 'CREDIT'::ledger_entry_type, :amount::bigint,
       dst.coin - :amount::bigint, dst.coin, 'reward budget received'
FROM tx, dst;

COMMIT;

SELECT account_code, coin FROM wallets WHERE account_code IN ('PROMO_BUDGET', 'SYSTEM_REWARD') ORDER BY account_code;
