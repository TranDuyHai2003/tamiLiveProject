-- name: CreateLiveRoom :one
INSERT INTO rooms (host_id, title, description, category, status, livekit_room_name, started_at)
VALUES ($1, $2, $3, $4, 'LIVE', $5, NOW())
RETURNING *;

-- Phân trang bằng cursor (keyset): lấy các phòng "sau" phòng cuối của trang trước theo thứ tự
-- (started_at, id) giảm dần. Không dùng OFFSET: trang sau không bị lệch khi có phòng mới lên sóng,
-- và tốc độ không chậm dần khi lật sâu.
-- name: ListLiveRooms :many
SELECT r.id, r.title, r.category, r.host_id, u.username AS host_username, r.started_at
FROM rooms r
JOIN users u ON u.id = r.host_id
WHERE r.status = 'LIVE'
  AND (sqlc.narg(cursor_started_at)::timestamptz IS NULL
       OR (r.started_at, r.id) < (sqlc.narg(cursor_started_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY r.started_at DESC, r.id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetRoom :one
SELECT * FROM rooms WHERE id = $1;

-- FOR SHARE: giữ trạng thái phòng ổn định trong suốt transaction (chặn EndRoom chạy song song).
-- name: GetRoomForShare :one
SELECT * FROM rooms WHERE id = $1 FOR SHARE;

-- name: EndRoom :one
UPDATE rooms
SET status = 'ENDED', ended_at = NOW()
WHERE id = $1 AND host_id = $2 AND status = 'LIVE'
RETURNING *;

-- name: JoinRoomMember :one
-- Thành viên bị KICKED/BANNED không thể join lại (ON CONFLICT ... WHERE không khớp => không trả về dòng nào).
INSERT INTO room_members (room_id, user_id, role, status)
VALUES ($1, $2, $3, 'JOINED')
ON CONFLICT (room_id, user_id) DO UPDATE
SET status = 'JOINED', joined_at = NOW(), left_at = NULL
WHERE room_members.status IN ('JOINED', 'LEFT')
RETURNING role, status;

-- name: GetRoomMember :one
SELECT role, status FROM room_members WHERE room_id = $1 AND user_id = $2;

-- name: LeaveRoom :exec
UPDATE room_members
SET status = 'LEFT', left_at = NOW()
WHERE room_id = $1 AND user_id = $2 AND status = 'JOINED';

-- name: LeaveAllRoomMembers :exec
UPDATE room_members
SET status = 'LEFT', left_at = NOW()
WHERE room_id = $1 AND status = 'JOINED';

-- Chỉ ghi được khi phòng đang LIVE và người gửi đang JOINED (kiểm tra nguyên tử trong 1 câu lệnh).
-- name: CreateChatMessage :one
INSERT INTO chat_messages (room_id, user_id, message)
SELECT r.id, m.user_id, sqlc.arg(message)::text
FROM rooms r
JOIN room_members m ON m.room_id = r.id
WHERE r.id = sqlc.arg(room_id) AND r.status = 'LIVE'
  AND m.user_id = sqlc.arg(user_id) AND m.status = 'JOINED'
RETURNING id, created_at;
