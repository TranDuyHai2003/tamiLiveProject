-- name: GetActiveGift :one
SELECT id, name, coin_price
FROM gifts
WHERE id = $1 AND is_active;

-- name: CreateGiftLog :one
INSERT INTO gift_logs (sender_id, receiver_id, room_id, gift_id, economy_transaction_id, quantity, total_coin, total_gem)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, created_at;
