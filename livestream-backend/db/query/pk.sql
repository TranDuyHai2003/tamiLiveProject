-- name: CreatePKBattle :one
INSERT INTO pk_battles (status, started_at)
VALUES ('ACTIVE', NOW())
RETURNING id;

-- name: CreatePKSide :one
INSERT INTO pk_sides (battle_id, side_number)
VALUES ($1, $2)
RETURNING id;

-- name: AddPKSideRoom :exec
INSERT INTO pk_side_rooms (side_id, room_id, host_id)
VALUES ($1, $2, $3);

-- Tìm side mà room đang thi đấu trong trận ACTIVE; FOR SHARE trên battle để FinishPKBattle
-- (FOR UPDATE) phải chờ mọi lượt cộng điểm đang chạy xong.
-- name: GetActivePKSideByRoomForShare :one
SELECT s.id AS side_id, s.battle_id
FROM pk_side_rooms sr
JOIN pk_sides s ON s.id = sr.side_id
JOIN pk_battles b ON b.id = s.battle_id
WHERE sr.room_id = $1 AND sr.left_at IS NULL AND b.status = 'ACTIVE'
FOR SHARE OF b;

-- name: GetOpenPKBattleIDByRoom :one
SELECT s.battle_id
FROM pk_side_rooms sr
JOIN pk_sides s ON s.id = sr.side_id
WHERE sr.room_id = $1 AND sr.left_at IS NULL;

-- name: IncrementPKSideScore :exec
UPDATE pk_sides SET score = score + sqlc.arg(points)::bigint WHERE id = sqlc.arg(id);

-- name: CreatePKScoreEvent :exec
INSERT INTO pk_score_events (battle_id, side_id, room_id, user_id, source, points, reference_type, reference_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: LockPKBattle :one
SELECT id, status FROM pk_battles WHERE id = $1 FOR UPDATE;

-- name: ListPKSidesByScore :many
SELECT id, side_number, score
FROM pk_sides
WHERE battle_id = $1
ORDER BY score DESC, side_number;

-- name: FinishPKBattle :exec
UPDATE pk_battles
SET status = 'FINISHED', winner_side_id = $2, ended_at = NOW()
WHERE id = $1;

-- name: ReleasePKSideRooms :exec
UPDATE pk_side_rooms sr
SET left_at = NOW()
FROM pk_sides s
WHERE sr.side_id = s.id AND s.battle_id = $1 AND sr.left_at IS NULL;
