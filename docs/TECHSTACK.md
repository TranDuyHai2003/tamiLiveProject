# Tech stack & kiến trúc — TamiLive

> Công nghệ được chọn, lý do chọn, cách các phần nối với nhau, và quy ước code.
> Nghiệp vụ xem [BUSINESS.md](BUSINESS.md), schema xem [DATABASE.md](DATABASE.md).

## 1. Tổng quan

```text
                 ┌──────────────────────────────┐
                 │  Next.js + Zustand + Tailwind │
                 │  LiveKit React SDK · iframe   │
                 └───┬───────────┬──────────┬────┘
              REST   │ WebSocket │   WebRTC │ (UDP)
                     ▼           ▼          ▼
            ┌─────────────────────────┐  ┌──────────────┐
            │  Go API (Gin)           │  │ LiveKit SFU  │
            │  + WS gateway (gorilla) │◄─┤ (+ Ingress)  │
            │  + background jobs      │  └──────────────┘
            └───┬────────────┬────────┘   webhook ▲   ▲ Server API
                │ pgx/sqlc   │ go-redis           │   │ (token, permission)
                ▼            ▼                    └───┘
         ┌────────────┐ ┌──────────────────┐
         │ PostgreSQL │ │ Redis            │
         │ (truth)    │ │ cache · pub/sub  │
         └────────────┘ └──────────────────┘
                ▲
                │ webhook (HMAC)
         Game provider / Payment gateway
```

Go backend **không bao giờ** đi qua luồng video — chỉ cấp quyền. Video đi thẳng client ↔ LiveKit.

## 2. Lựa chọn công nghệ

| Tầng | Chọn | Lý do | Đánh giá |
|---|---|---|---|
| Frontend | **Next.js** (App Router) | SSR cho trang chủ/SEO, React ecosystem | ✅ Hợp lý |
| State FE | **Zustand** | Nhẹ, update tần suất cao không re-render toàn cây | ✅ |
| Style | **Tailwind CSS** | Nhanh, nhất quán | ✅ |
| Danh sách chat | `@tanstack/react-virtual` hoặc `react-window` | Virtual list cho chat cuộn nhanh | ✅ |
| Hiệu ứng quà | Lottie / SVGA / video alpha | Phổ biến trong app live | ✅ |
| Media | **LiveKit** (SFU, WebRTC) | Độ trễ < 500ms, SDK tốt, tự host được | ✅ |
| OBS | **LiveKit Ingress** (WHIP ưu tiên, RTMP nếu cần) | OBS không nói WebRTC trực tiếp; cần thêm service Ingress | ⚠️ Bản gốc chưa nhắc |
| API | **Go + Gin** | Nhanh, một binary, ít RAM | ✅ |
| WebSocket | **gorilla/websocket** | Ổn định, phổ biến | ✅ |
| DB | **PostgreSQL 16+** | ACID cho tiền | ✅ |
| Truy cập DB | **pgx/v5 + sqlc** | SQL viết tay, type-safe, không ma thuật | ✅ — **thay GORM** trong bản gốc |
| Migration | **golang-migrate** | Đơn giản, file SQL thuần | ✅ |
| Cache/realtime | **Redis 7** + `go-redis/v9` | Pub/Sub, counter, presence, rate limit | ✅ |
| Auth | `golang-jwt/jwt/v5` + `bcrypt` | Chuẩn | ✅ |
| LiveKit từ Go | `livekit/protocol/auth` (token) + **`livekit/server-sdk-go`** (RoomService, webhook) | `protocol` chỉ đủ sinh token; cần server SDK để `UpdateParticipant`, `RemoveParticipant`, verify webhook | ⚠️ Cần thêm vào `go.mod` khi làm game/kick |

### Đánh giá tổng thể

Stack **hợp lý** cho một nền tảng livestream quy mô nhỏ–vừa. Điểm cần sửa so với bản mô tả gốc:

1. **GORM → sqlc + pgx.** Code hiện tại đã dùng sqlc/pgx; tài liệu cũ nhắc GORM gây nhầm. Không trộn hai cái.
2. **Thiếu LiveKit Ingress** nếu muốn hỗ trợ OBS.
3. **Thiếu phần vận hành**: logging, config, health check, metrics, docker-compose dev, background jobs. Xem mục 6.
4. **"Một binary ôm hàng vạn WebSocket"** đúng ở giai đoạn đầu. Khi scale: tách WS gateway ra process riêng, vẫn chung codebase (xem mục 7).

## 3. Cấu trúc thư mục backend

```text
livestream-backend/
├── main.go                    # Khởi tạo config, pool, redis, service, router; graceful shutdown
├── db/
│   ├── migration/             # golang-migrate: 0000NN_*.up.sql / .down.sql
│   └── query/                 # SQL cho sqlc, mỗi domain 1 file
├── internal/
│   ├── config/                # Đọc env → struct Config
│   ├── db/                    # ⚙️ code sinh bởi sqlc (KHÔNG sửa tay) + conn.go
│   ├── auth/                  # JWT, hash password
│   ├── pkg/                   # Tiện ích dùng chung: redis client, ...
│   ├── service/               # Nghiệp vụ. Mở transaction, gọi db.Queries
│   └── delivery/
│       └── http/              # Gin handler, middleware, WebSocket handler
├── docs/                      # Tài liệu này
└── sqlc.yaml
```

**Luật phụ thuộc** (chỉ được gọi theo chiều mũi tên):

```text
delivery/http  →  service  →  db (sqlc)
                     └──────→  pkg (redis, livekit, ...)
```

- **Handler**: parse request, validate cơ bản, lấy user từ context, gọi service, map lỗi → HTTP status. Không chứa SQL, không mở transaction.
- **Service**: toàn bộ luật nghiệp vụ. Mở transaction bằng `withTx`. Trả lỗi dạng `ErrNotFound`, `ErrConflict`, `ErrInvalidInput`, `ErrInsufficientBalance`...
- **db**: chỉ code sinh từ sqlc.

## 4. Các luồng kỹ thuật chính

### 4.1 Xác thực
- `Authorization: Bearer <access_token>` cho REST. Middleware verify JWT → đặt `user_id`, `role` vào `gin.Context`.
- Refresh token: cookie `HttpOnly; Secure; SameSite=Lax`, path `/api/auth`.
- WebSocket: client gọi `POST /api/ws/ticket` → nhận ticket ngẫu nhiên (Redis `SET ws_ticket:<t> <user_id> EX 30`) → mở `wss://.../ws?ticket=<t>` → server `GETDEL` ticket. Token thật không bao giờ nằm trên URL.

### 4.2 WebSocket + Redis Pub/Sub
- Mỗi kết nối: 1 goroutine **readPump**, 1 goroutine **writePump**, một channel `send` có buffer.
- **Chỉ writePump được ghi** vào connection (gorilla không cho ghi đồng thời).
- Channel `send` đầy (client chậm) → **đóng kết nối**, không block cả phòng.
- Ping/pong: server ping mỗi ~30s, `SetReadDeadline` ~60s.
- Mỗi node Go giữ `map[roomID]set[*Client]` cục bộ. Node subscribe Redis `room:<id>:events` khi có client đầu tiên vào phòng, unsubscribe khi phòng trên node đó rỗng.
- Gửi sự kiện: service `PUBLISH room:<id>:events <json>` → mọi node nhận → fan-out xuống client của mình.
- Redis Pub/Sub là **fire-and-forget**: node mất kết nối là mất tin. Chấp nhận cho chat/hiệu ứng; dữ liệu quan trọng (số dư, điểm PK cuối) client phải lấy lại qua REST khi reconnect. Chat lấy lại qua `GET /rooms/:id/messages?after=`.
- **Hiện trạng code:** mỗi kết nối tự subscribe Redis riêng (chưa có hub), chưa có channel `send` có buffer. Đủ cho MVP; chuyển sang mô hình hub ở trên khi số người xem mỗi phòng lớn.
- **Tắt server:** `http.Server.Shutdown` không đóng và không chờ các kết nối WebSocket (Go coi là kết nối đã hijack). Hub phải tự giữ danh sách kết nối và gửi close frame (`1001 Going Away`) cho từng cái khi shutdown, để client phân biệt "server bảo trì" với "mất mạng".
- **Sự kiện điều khiển** đi chung kênh phòng: `member.kicked {user_id}` → node đang giữ kết nối của user đó đóng kết nối ngay.

### 4.3 Redis key convention

| Key | Kiểu | Dùng cho | TTL |
|---|---|---|---|
| `room:<id>:events` | channel | Sự kiện phòng | — |
| `global_events` | channel | Broadcast toàn server | — |
| `room:<id>:viewers` | ZSET (member=user_id, score=last_seen) | Presence; xóa member quá 60s | theo phòng |
| `room:<id>:reactions` | HASH (type → count) | Counter thả tim, flush định kỳ | 1 ngày |
| `pk:<battle_id>` | HASH (side_id → score, ends_at, multiplier) | Hiển thị PK | duration + 10 phút |
| `ws_ticket:<t>` | STRING | Ticket mở WebSocket | 30s |
| `ratelimit:<scope>:<id>` | STRING/INCR | Rate limit | cửa sổ |
| `lock:job:<name>` | STRING (`SET NX EX`) | Chỉ 1 node chạy job | vài giây |

### 4.4 Giao dịch tiền
- Luôn: `withTx` → idempotency → kiểm tra nghiệp vụ → `postLedger` → biên lai → COMMIT → **rồi mới** publish Redis.
- Không gọi HTTP ra ngoài (payment, game provider, LiveKit) **bên trong** transaction — giữ transaction ngắn để không giữ lock lâu.
- Khi cần đảm bảo "đã commit thì chắc chắn gửi event" (VD webhook ra bên thứ ba): dùng **outbox table** — ghi event vào bảng trong cùng transaction, worker đọc và gửi. Chưa cần cho MVP.

### 4.5 LiveKit
- Token: `auth.NewAccessToken(apiKey, secret)` với `VideoGrant{RoomJoin, Room, CanPublish, CanSubscribe, CanPublishData}`. Host: publish + publish data; viewer: chỉ subscribe, **không** publish data (data message không qua backend nên né được kiểm duyệt chat); người chơi game: `CanPublishSources: ["microphone"]`.
- Hạn token nên ngắn (~10 phút; code hiện để 6 giờ). LiveKit tự gia hạn cho người đang kết nối, còn người đã bị `RemoveParticipant` thì phải xin token mới từ backend, và backend sẽ từ chối.
- Đổi quyền khi đang trong phòng (lên/xuống ghế): `RoomServiceClient.UpdateParticipant`.
- Nhận webhook (`room_finished`, `participant_joined/left`, `ingress_*`) tại `POST /api/livekit/webhook`, verify chữ ký bằng API secret.

### 4.6 Background jobs
Chạy trong cùng binary (goroutine + `time.Ticker`), mỗi lần chạy lấy Redis lock `lock:job:<name>` để khi chạy nhiều node chỉ 1 node làm:

| Job | Chu kỳ | Việc |
|---|---|---|
| `pk_settlement` | 1s | Kết thúc trận có `ends_at <= now()` |
| `pk_invite_timeout` | 5s | Hủy lời mời PK quá hạn |
| `reaction_flush` | 30s | Redis reactions → `room_reaction_stats` |
| ~~`chat_flush`~~ | — | *Không dùng ở MVP:* chat ghi DB đồng bộ (xem BUSINESS §4). Chỉ thêm khi đo được DB nghẽn vì chat |
| `presence_cleanup` | 15s | Xóa viewer không heartbeat, cập nhật viewer count |
| `cleanup` | 1h | Xóa idempotency key/session hết hạn |
| `ledger_check` | 1 ngày | Chạy câu kiểm tra sức khỏe sổ cái, cảnh báo nếu lệch |

## 5. Quy ước code

- **Lỗi**: bọc bằng `fmt.Errorf("làm gì: %w", err)`. Service trả sentinel error; handler map sang HTTP status ở **một chỗ duy nhất**.
- **Context**: mọi hàm I/O nhận `ctx context.Context` làm tham số đầu.
- **Tiền**: luôn `int64`, đơn vị nhỏ nhất. Không dùng `float`. Kiểm tra tràn số khi nhân.
- **Thời gian**: lưu `TIMESTAMPTZ`, xử lý UTC, client tự đổi múi giờ.
- **ID**: UUID. Bảng ghi nhiều dùng UUIDv7 (`uuid.NewV7()`).
- **Log**: `log/slog` dạng JSON, mỗi request có `request_id`. Không log password, token, `payout_account`.
- **Config**: chỉ qua biến môi trường. `.env` không commit; có `.env.example` liệt kê đủ key.
- **Format response lỗi** thống nhất:
  ```json
  { "error": { "code": "INSUFFICIENT_BALANCE", "message": "not enough COIN" } }
  ```

## 6. Môi trường & vận hành

**Dev local** — nên có `docker-compose.yml` gồm: `postgres:16`, `redis:7`, `livekit/livekit-server --dev`, (tùy chọn) `livekit/ingress`.

**Biến môi trường tối thiểu**

```text
APP_ENV, HTTP_ADDR
DATABASE_URL
REDIS_URL
JWT_SECRET, JWT_ACCESS_TTL, JWT_REFRESH_TTL
LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET
GEM_RATE_PERCENT
CORS_ALLOWED_ORIGINS
```

**Cần có trước khi lên production**
- `GET /healthz` (sống) và `GET /readyz` (ping được Postgres + Redis).
- Graceful shutdown: dừng nhận request, đóng WebSocket kèm close frame, chờ request đang chạy.
- Metrics Prometheus: số kết nối WS, latency API, số giao dịch/giây, lỗi.
- Rate limit theo user/IP cho login, chat, tặng quà.
- CORS và kiểm tra `Origin` khi upgrade WebSocket (`CheckOrigin` không được `return true`).
- TLS (WebRTC trên trình duyệt yêu cầu HTTPS ngoài localhost); TURN server cho người dùng sau NAT chặt (LiveKit có TURN tích hợp).

**Test**
- Unit test cho service với Postgres thật (testcontainers hoặc DB test riêng) — đừng mock DB cho phần tiền.
- Test đồng thời: 100 goroutine cùng tặng quà từ 1 ví → số dư cuối đúng, không âm, sổ cái cân.

## 7. Hướng scale (khi cần, chưa làm bây giờ)

| Khi nào | Làm gì |
|---|---|
| > ~10k kết nối WS / node | Chạy nhiều node sau load balancer (sticky không bắt buộc vì đã có Pub/Sub) |
| API và WS tranh tài nguyên | Tách `cmd/api` và `cmd/gateway` — cùng repo, 2 binary |
| Ví hệ thống nghẽn khi nhiều quà | Bucket ví hệ thống (DATABASE.md D3) |
| Postgres đọc nặng | Read replica cho danh sách phòng, lịch sử, bảng xếp hạng |
| Pub/Sub mất tin không chấp nhận được | Redis Streams hoặc NATS JetStream |
| Phòng rất đông người xem | LiveKit cluster nhiều node; cân nhắc HLS/CDN cho người xem không tương tác |
