-- name: GetActiveGame :one
SELECT id FROM games WHERE id = $1 AND status = 'ACTIVE';

-- name: CreateGameSession :one
INSERT INTO game_sessions (room_id, game_id, host_id, status, max_players)
VALUES ($1, $2, $3, 'WAITING', $4)
RETURNING *;

-- name: LockGameSession :one
SELECT * FROM game_sessions WHERE id = $1 FOR UPDATE;

-- name: ListTakenSeats :many
SELECT seat_number::int AS seat_number
FROM game_players
WHERE session_id = $1 AND seat_number IS NOT NULL
ORDER BY seat_number;

-- name: CreateGamePlayer :one
INSERT INTO game_players (session_id, user_id, status, seat_number)
VALUES ($1, $2, 'REQUESTED', $3)
RETURNING *;

-- name: StartGameSession :exec
UPDATE game_sessions SET status = 'RUNNING', started_at = NOW() WHERE id = $1;

-- name: MarkGamePlayersPlaying :exec
UPDATE game_players
SET status = 'PLAYING', joined_at = NOW()
WHERE session_id = $1 AND status IN ('REQUESTED', 'APPROVED');

-- name: CloseGameSession :exec
UPDATE game_sessions SET status = $2, ended_at = NOW() WHERE id = $1;

-- name: MarkGamePlayersFinished :exec
UPDATE game_players
SET status = 'FINISHED', left_at = NOW()
WHERE session_id = $1 AND status IN ('REQUESTED', 'APPROVED', 'PLAYING');

-- name: CancelOpenGameSessionsByRoom :many
UPDATE game_sessions
SET status = 'CANCELLED', ended_at = NOW()
WHERE room_id = $1 AND status IN ('WAITING', 'RUNNING')
RETURNING id;

-- name: GetGamePlayer :one
SELECT id, status FROM game_players WHERE session_id = $1 AND user_id = $2;

-- name: CreateGameResult :one
INSERT INTO game_results (session_id, player_id, user_id, score, reward_coin, result_payload, signature, status, validated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'VALIDATED', NOW())
RETURNING id;

-- name: MarkGameResultRewarded :exec
UPDATE game_results
SET status = 'REWARDED', economy_transaction_id = $2, rewarded_at = NOW()
WHERE id = $1;
