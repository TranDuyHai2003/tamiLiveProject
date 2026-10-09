# Database — TamiLive

> Tài liệu xương sống cho tầng dữ liệu. Nguồn sự thật là các file trong `db/migration/`;
> tài liệu này giải thích **vì sao** schema như vậy, các **bất biến** phải giữ, và **việc còn thiếu**.

## 1. Nguyên tắc

| Thành phần | Vai trò | Mất dữ liệu thì sao? |
|---|---|---|
| **PostgreSQL** | Source of truth: tiền, quà, kết quả PK/game, lịch sử | Không được phép mất |
| **Redis** | Cache, presence, counter realtime, Pub/Sub, rate limit | Dựng lại được từ Postgres hoặc chấp nhận mất |
| **LiveKit** | Media WebRTC | Không lưu gì nghiệp vụ |

Quy tắc bắt buộc:

1. **Mọi thay đổi số dư đi qua `postLedger`** (`internal/service/ledger.go`) trong cùng một DB transaction. Không bao giờ `UPDATE wallets` trực tiếp ở chỗ khác.
2. **Ràng buộc nằm trong DB**, không chỉ trong code Go: CHECK, UNIQUE, FK kép, partial unique index, trigger. Code có bug thì DB vẫn chặn.
3. **Không hard-delete user.** Dùng `users.deleted_at` + `status = 'DELETED'`. Dữ liệu tài chính phải tồn tại vĩnh viễn.
4. **Redis chỉ là bản sao.** Dữ liệu nào ảnh hưởng tiền/thắng thua phải ghi Postgres trong cùng transaction với hành động gây ra nó.
5. **Schema chỉ đổi qua migration** (`golang-migrate`). Truy vấn chỉ viết trong `db/query/*.sql` rồi `sqlc generate`.

## 2. Bản đồ migration

| File | Domain | Bảng |
|---|---|---|
| `000001_account_schema` | Account | `users`, `user_sessions`, `follows` + hàm `set_updated_at()` |
| `000002_economy_schema` | Economy | `wallets` (+ seed ví hệ thống), `economy_transactions`, `economy_ledger_entries`, `idempotency_keys`, `cashout_requests` |
| `000003_livestream_schema` | Livestream | `rooms`, `room_members`, `chat_messages`, `room_reaction_stats` |
| `000004_gift_inventory_schema` | Gift | `gifts`, `inventories`, `gift_logs` |
| `000005_pk_battle_schema` | PK | `pk_battles`, `pk_sides`, `pk_side_rooms`, `pk_score_events` |
| `000006_minigame_schema` | Mini game | `games`, `game_sessions`, `game_players`, `game_results` |

Thứ tự phụ thuộc: Account → Economy → Livestream → Gift → PK → Mini game. Migration sau chỉ được tham chiếu bảng của migration trước.

```text
users ─┬─ user_sessions, follows
       ├─ wallets ── economy_ledger_entries ── economy_transactions
       │                                         ├─ idempotency_keys
       │                                         ├─ cashout_requests
       │                                         ├─ gift_logs
       │                                         └─ game_results
       └─ rooms ─┬─ room_members, chat_messages, room_reaction_stats
                 ├─ gift_logs ── gifts ── inventories
                 ├─ pk_side_rooms ── pk_sides ── pk_battles ── pk_score_events
                 └─ game_sessions ── game_players ── game_results
                          └─ games
```

## 3. Economy — phần quan trọng nhất

### 3.1 Mô hình tài khoản

Có 2 loại tiền: **COIN** (viewer nạp bằng tiền thật, dùng để mua quà) và **GEM** (host nhận được, dùng để rút tiền).

| `account_code` | `account_type` | Ý nghĩa | Được âm? |
|---|---|---|---|
| *(ví user)* | `USER` | Ví của từng người dùng | **Không** |
| `FIAT_RESERVE` | `SYSTEM_RESERVE` | Đối ứng khi phát hành COIN qua nạp tiền | Có |
| `PLATFORM_REVENUE` | `SYSTEM_REVENUE` | COIN thu về khi user tiêu | Có |
| `GEM_LIABILITY` | `SYSTEM_LIABILITY` | Đối ứng khi phát hành GEM cho host (nợ phải trả) | Có |
| `SYSTEM_REWARD` | `SYSTEM_REWARD` | Nguồn thưởng (game, sự kiện) | Có |
| `CASHOUT_ESCROW` | `SYSTEM_ESCROW` | GEM đang tạm giữ chờ rút | Có |
| `CHARGEBACK_LOSS` *(S5, chưa làm)* | `SYSTEM_LOSS` | COIN đã bị tiêu nhưng tiền nạp bị hoàn (chargeback). Số dư âm = tổng nợ của user | Có |

**Bất biến:** với mỗi currency, `SUM(số dư mọi ví) = 0` tại mọi thời điểm. Đây là phép kiểm tra sức khỏe hằng ngày nên chạy.

### 3.2 Quy ước DEBIT / CREDIT

Dự án dùng quy ước đơn giản, **không phải** quy ước kế toán chuẩn:

- `CREDIT` = số dư ví **tăng** → `balance_after = balance_before + amount`
- `DEBIT` = số dư ví **giảm** → `balance_after = balance_before - amount`
- `amount` luôn > 0.

CHECK `check_ledger_balance_math` ép đúng công thức này.

### 3.3 Các bút toán chuẩn

| Nghiệp vụ | COIN | GEM |
|---|---|---|
| **Nạp tiền** (TOPUP) | `FIAT_RESERVE` DEBIT → user CREDIT | — |
| **Nạp tiền khi đang nợ** (TOPUP) | `FIAT_RESERVE` DEBIT → user CREDIT (toàn bộ), rồi user DEBIT R → `CHARGEBACK_LOSS` CREDIT R (R = phần trả nợ). Giảm `users.coin_debt` đi R | — |
| **Tặng quà** (GIFT) | viewer DEBIT → `PLATFORM_REVENUE` CREDIT | `GEM_LIABILITY` DEBIT → **host** CREDIT (`total_gem = total_coin × gem_rate% của host`) |
| **Thưởng game** | `SYSTEM_REWARD` DEBIT → user CREDIT | — |
| **Hoàn tiền thủ công** (REFUND) | `PLATFORM_REVENUE` DEBIT → viewer CREDIT | — (không đụng GEM của host) |
| **Chargeback** (bị hoàn X, user còn B) | user DEBIT min(B,X) → `FIAT_RESERVE` CREDIT min(B,X); nếu X > B: `CHARGEBACK_LOSS` DEBIT (X−B) → `FIAT_RESERVE` CREDIT (X−B), và `users.coin_debt += X−B` | — (host giữ GEM) |
| **Yêu cầu rút** | — | host DEBIT → `CASHOUT_ESCROW` CREDIT |
| **Rút thành công** | — | `CASHOUT_ESCROW` DEBIT → `GEM_LIABILITY` CREDIT (xóa nợ) |
| **Rút bị từ chối** | — | `CASHOUT_ESCROW` DEBIT → host CREDIT |

Mỗi nghiệp vụ = **1** dòng `economy_transactions` + **N** dòng `economy_ledger_entries`. Tặng quà là **một** transaction có 4 leg, không phải 2 transaction "gửi" và "nhận".

**Vì sao chargeback không làm ví âm:** `check_user_wallet_non_negative` là thứ đảm bảo không ai tiêu quá số dư, kể cả khi nhiều request chạy song song. Phần nợ nằm ở ví hệ thống `CHARGEBACK_LOSS` (trong sổ cái) và được ghi thêm vào `users.coin_debt` (để kiểm tra nhanh), thay vì để ví user âm. Nhờ đó bảo đảm trên vẫn giữ nguyên cho mọi luồng khác, `FIAT_RESERVE` luôn khớp với số tiền thật đã thu, và khoản nợ có dấu vết đầy đủ trong sổ cái.

Bất biến đi kèm: `−CHARGEBACK_LOSS.coin = SUM(users.coin_debt)` (xem phép đối soát ở §6).

### 3.4 Bảo vệ ở tầng DB (đã có)

- `check_user_wallet_non_negative` — ví USER không âm.
- `check_wallet_owner` — ví USER có `user_id`, ví SYSTEM có `account_code`.
- `trg_ledger_immutable` — sổ cái append-only, cấm UPDATE/DELETE.
- `trg_ledger_balanced` (constraint trigger, `DEFERRABLE INITIALLY DEFERRED`) — lúc COMMIT, mỗi transaction phải cân theo từng currency.
- `idempotency_keys UNIQUE (user_id, key)` — request trùng key bị chặn.
- `economy_transactions.provider_txn_id UNIQUE` — webhook cổng thanh toán gọi lại nhiều lần cũng chỉ cộng tiền một lần.

### 3.5 Chống double-spend và deadlock

- Trừ tiền bằng `UPDATE wallets SET coin = coin + delta WHERE id = $1 AND coin + delta >= 0 RETURNING coin` (`ApplyWalletCoinDelta`). Câu UPDATE tự lấy row lock; không cần `SELECT ... FOR UPDATE` riêng, không đủ tiền thì trả `no rows`.
- `postLedger` khóa ví theo thứ tự `(wallet_id, currency)` tăng dần → mọi giao dịch lấy lock cùng thứ tự → không deadlock.

**Thứ tự khóa chung cho mọi transaction** *(bắt buộc; transaction nào bỏ qua một bước thì vẫn giữ thứ tự của các bước còn lại)*:

| Bước | Khóa | Ví dụ |
|---|---|---|
| 1 | `users` của người thực hiện (`FOR NO KEY UPDATE`) | Tặng quà, nạp tiền, chargeback, cộng exp |
| 2 | `rooms` (`FOR SHARE` khi đọc, UPDATE khi kết thúc phòng) | Tặng quà, mở game, kết thúc live |
| 3 | `game_sessions` (`FOR UPDATE`) | Join game, nhận kết quả game |
| 4 | `wallets`, qua `postLedger` (theo wallet_id tăng dần) | Mọi giao dịch tiền |
| 5 | `pk_battles` (`FOR SHARE` khi cộng điểm, `FOR UPDATE` khi chốt trận) → `pk_sides` | Tặng quà trong PK, chốt trận |

Ví dụ deadlock nếu vi phạm: `SendGift` khóa ví rồi mới UPDATE `users` để cộng exp, trong khi nạp tiền khi đang nợ UPDATE `users.coin_debt` rồi mới khóa ví. Cùng một user vừa tặng quà vừa nạp tiền thì hai transaction chờ nhau mãi.

`FOR NO KEY UPDATE` (chứ không phải `FOR UPDATE`) để không chặn các câu INSERT có khóa ngoại trỏ tới `users` (ví dụ `idempotency_keys`, `gift_logs`). Kiểm tra khóa ngoại chỉ lấy `FOR KEY SHARE`, không xung đột với `FOR NO KEY UPDATE`.

## 4. Các domain khác — bất biến chính

**Account**
- Email/username unique không phân biệt hoa thường (`LOWER()` index).
- `follows`: không follow chính mình, không follow trùng.
- `user_sessions` chỉ lưu **hash** của refresh token.

**Livestream**
- Mỗi host tối đa 1 phòng `LIVE` (`idx_rooms_one_live_per_host`).
- `started_at`/`ended_at` phải khớp `status`.
- `rooms UNIQUE (id, host_id)` để bảng khác FK kép, đảm bảo `host_id` đúng là chủ phòng.
- `room_members` là lịch sử; ai đang online thì hỏi Redis.
- `room_reaction_stats` là snapshot được flush định kỳ từ Redis, không ghi từng lượt thả tim.

**Gift**
- `gift_logs` là biên lai nghiệp vụ, **không** phải sổ cái. Muốn biết tiền thì đọc ledger.
- `gift_logs` lưu snapshot `total_coin`, `total_gem` — giá quà đổi sau này không ảnh hưởng lịch sử.

**PK**
- `pk_battles → pk_sides → pk_side_rooms`: hỗ trợ 1v1, 2v2, team PK mà không đổi schema.
- FK kép đảm bảo `winner_side_id` và `pk_score_events.side_id` thuộc đúng trận.
- Một room chỉ ở trong 1 trận chưa kết thúc (`idx_pk_side_rooms_one_active`).
- Điểm PK được cộng **trong cùng transaction tặng quà** → quà và điểm cùng thành công hoặc cùng thất bại.

**Mini game**
- Mỗi room tối đa 1 session `WAITING`/`RUNNING`.
- Mỗi ghế 1 người, mỗi user 1 lần / session.
- `game_results UNIQUE (player_id)` + `check_game_result_rewarded` → không nhận thưởng 2 lần.

## 5. Đánh giá: schema đã chuẩn chưa?

**Kết luận:** Phần lõi (ledger kế toán kép, idempotency, ràng buộc toàn vẹn) đã **tốt và vượt mức một bản MVP**. Các migration hiện tại đã sửa phần lớn lỗ hổng của bản DBML ban đầu (thiếu CHECK, thiếu FK kép, `balance_before/after` nullable, `idempotency_keys.user_id` nullable...). Còn các điểm dưới đây, xếp theo mức độ ưu tiên.

### 🔴 Nên sửa trước khi có dữ liệu thật

| # | Vấn đề | Đề xuất |
|---|---|---|
| D1 | **Enum `economy_transaction_type` sai mô hình**: có `GIFT_SEND` và `GIFT_RECEIVE` trong khi một lần tặng là một transaction. Thiếu loại cho các bước rút tiền. | Đổi thành: `TOPUP`, `GIFT`, `GAME_REWARD`, `CASHOUT_HOLD`, `CASHOUT_PAYOUT`, `CASHOUT_RELEASE`, `REFUND`, `ADJUSTMENT`. |
| D2 | **`ON DELETE CASCADE` trên dữ liệu tài chính**: `wallets.user_id`, `gift_logs.*` (kể cả `economy_transaction_id`), `game_results.*`, `game_sessions.game_id`... Xóa một user, một quà hay một game sẽ kéo theo xóa biên lai. | Đổi sang `ON DELETE RESTRICT` cho mọi FK trỏ vào `users`, `wallets`, `economy_transactions` từ bảng tài chính/biên lai. Chỉ giữ CASCADE cho dữ liệu phụ (`user_sessions`, `follows`, `room_members`, `chat_messages`). |
| D3 | **Ví hệ thống là "hot row"**: mọi lượt tặng quà đều `UPDATE` cùng 1 dòng `PLATFORM_REVENUE` và `GEM_LIABILITY` → mọi giao dịch quà toàn hệ thống xếp hàng tuần tự. Ổn ở vài chục quà/giây, nghẽn khi có sự kiện lớn. | Giai đoạn đầu: chấp nhận, đo bằng benchmark. Khi cần scale: chia mỗi ví hệ thống thành N "bucket" (`PLATFORM_REVENUE_00..15`), chọn bucket ngẫu nhiên khi ghi; số dư thật = tổng các bucket. |
| D4 | **Thiếu cấu hình tỷ lệ quy đổi COIN→GEM**: hiện đang là tham số `gemRatePercent` trong code. | Đủ dùng cho MVP. Nếu cần tỷ lệ theo quà / theo agency / theo thời gian: thêm `gifts.gem_value` hoặc bảng `economy_configs`. Luôn snapshot vào `gift_logs.total_gem` (đã làm). |
| D5 | **`pk_battles` không có thời lượng**: không biết trận kết thúc lúc nào nếu server restart. | Thêm `duration_seconds INT NOT NULL` và `ends_at TIMESTAMPTZ`. Job settlement dựa vào `ends_at`, không dựa vào TTL của Redis. |

### 🟡 Thiếu so với nghiệp vụ đã mô tả

| # | Thiếu | Đề xuất |
|---|---|---|
| D6 | **Level theo tiền nạp/tiêu** nhưng `users` chỉ có `level`. | Thêm `users.exp BIGINT NOT NULL DEFAULT 0` (hoặc bảng `user_stats` với `total_topup_coin`, `total_spent_coin`, `total_received_gem`). Cập nhật trong cùng transaction tặng quà/nạp tiền. Level tính từ `exp` bằng bảng ngưỡng. |
| D7 | **Nạp tiền** chưa có bảng đơn hàng. | Thêm `coin_packages` (giá VND → số COIN) và `topup_orders` (user, package, số tiền fiat, provider, `status`, `provider_txn_id`). Chỉ khi webhook xác nhận mới tạo transaction `TOPUP`. |
| D8 | **Agency** có trong `user_role` nhưng không có bảng. | Giai đoạn 2: `agencies`, `agency_hosts (agency_id, host_id, revenue_share_percent, joined_at, left_at)`. |
| D9 | **Buff X2 điểm PK** không lưu được hệ số. | Thêm `pk_score_events.multiplier INT NOT NULL DEFAULT 1` và `base_points`. `points = base_points × multiplier`. |
| D10 | **Kiểm duyệt**: chưa có báo cáo vi phạm, cấm chat toàn hệ thống. | Giai đoạn 2: `user_reports`, `user_bans`. Ban trong phòng dùng `room_members.status = 'BANNED'` (đã có). |
| D11 | **`inventories`**: nghiệp vụ chưa nói user mua quà vào kho hay tặng thẳng. | MVP: **tặng thẳng**, chưa dùng `inventories`. Nếu sau này có kho: mua quà = transaction `GIFT_PURCHASE` (ghi doanh thu lúc mua), tặng từ kho chỉ phát GEM. |

### 🟢 Cải thiện kỹ thuật

| # | Đề xuất |
|---|---|
| D12 | Sinh UUID **v7** trong Go (`uuid.NewV7()`) cho bảng ghi nhiều (`economy_ledger_entries`, `chat_messages`, `pk_score_events`, `gift_logs`) — ID tăng theo thời gian nên B-tree index không bị phân mảnh. Có thể bỏ extension `uuid-ossp` và dùng `gen_random_uuid()` có sẵn từ PG13. |
| D13 | Job dọn dẹp: xóa `idempotency_keys` hết hạn, `user_sessions` hết hạn/đã revoke. |
| D14 | Khi `chat_messages` lớn (hàng chục triệu dòng): partition theo tháng trên `created_at`. Chưa cần làm bây giờ. |
| D15 | Nếu một room được tổ chức nhiều ván game, cần `game_sessions` = 1 ván (hiện tại đúng) — ghi rõ trong code để tránh tái sử dụng session. |

> Vì dự án chưa có dữ liệu production, các sửa đổi D1–D5 có thể **sửa thẳng vào migration gốc** rồi `migrate down` / `up` lại. Sau khi đã deploy thì mọi thay đổi phải là migration mới.

> **Trạng thái sau khi chốt nghiệp vụ (BUSINESS.md §9):** D1 → S7 · D4 → S1 · D5 → S8 · D6 → S3 · D7 → S6. Các mục còn lại giữ nguyên.

## 5b. Thay đổi schema theo quyết định nghiệp vụ *(chưa làm)*

Danh sách này suy ra từ [BUSINESS.md §9](BUSINESS.md). Đánh số S1–S13 để đối chiếu khi làm migration.

| # | Thay đổi | Migration | Phục vụ luật |
|---|---|---|---|
| S1 | `users.gem_rate_percent SMALLINT NULL CHECK (gem_rate_percent BETWEEN 0 AND 60)`. `NULL` = dùng mặc định 50% từ config | 000001 | Gem rate theo host, agency tối đa 60% |
| S2 | `gift_logs.gem_rate_percent SMALLINT NOT NULL` (snapshot) và `gift_logs.target_user_id UUID NULL REFERENCES users(id)` | 000004 | Audit rate đã áp dụng; quà buff cho người chơi nhưng GEM về host |
| S3 | `users.exp BIGINT NOT NULL DEFAULT 0 CHECK (exp >= 0)`; bảng `level_thresholds (level INT PRIMARY KEY, min_exp BIGINT NOT NULL UNIQUE)` + seed; `users.level` cập nhật cùng transaction | 000001 | 1 COIN = 1 EXP, ngưỡng cố định |
| S4 | `gifts.min_level INT NOT NULL DEFAULT 0 CHECK (min_level >= 0)` | 000004 | Quà độc quyền theo level |
| S5 | Enum `user_status` thêm `RESTRICTED`; `users.coin_debt BIGINT NOT NULL DEFAULT 0 CHECK (coin_debt >= 0)`; `users.chargeback_count INT NOT NULL DEFAULT 0`, `users.last_chargeback_at TIMESTAMPTZ`; CHECK: `coin_debt > 0` ⇒ `status IN ('RESTRICTED','BANNED','SUSPENDED','DELETED')`. Enum `account_type` thêm `SYSTEM_LOSS` và seed ví `('SYSTEM_LOSS', 'CHARGEBACK_LOSS')` | 000001, 000002 | Chargeback mà không làm ví âm; nợ có dấu vết trong sổ cái |
| S6 | `coin_packages (id, channel ENUM('WEB','IOS','ANDROID'), store_product_id, price_amount, price_currency, coin_amount, is_active)`; `topup_orders (id, user_id, package_id, channel, amount_fiat, provider, provider_txn_id UNIQUE, status, economy_transaction_id, created_at, completed_at)` | 000002 | Bảng giá theo kênh, nạp qua webhook |
| S7 | Enum `economy_transaction_type`: `TOPUP`, `GIFT`, `GAME_REWARD`, `REFUND`, `CHARGEBACK`, `CASHOUT_HOLD`, `CASHOUT_PAYOUT`, `CASHOUT_RELEASE`, `ADJUSTMENT` (bỏ `GIFT_SEND`/`GIFT_RECEIVE`/`CASHOUT`). Partial unique index `(reference_id) WHERE type = 'REFUND'` để mỗi giao dịch gốc chỉ hoàn một lần | 000002 | D1; hoàn tiền tối đa 1 lần |
| S8 | Enum `pk_status` thêm `PUNISHMENT` (`CANCELLED` đã có). `pk_battles` thêm `duration_seconds INT NOT NULL`, `ends_at TIMESTAMPTZ`, `punishment_ends_at TIMESTAMPTZ`, `end_reason` (`SCORE`, `DRAW`, `TKO`, `CANCELLED`). **Sửa** `check_pk_winner_only_finished` → `winner_side_id IS NULL OR status IN ('PUNISHMENT','FINISHED')`; `check_pk_ended_at` cũng áp cho `PUNISHMENT`. **Thêm** CHECK khớp kết quả: `end_reason IS NULL` khi chưa chốt; `SCORE`/`TKO` ⇒ `winner_side_id IS NOT NULL`; `DRAW`/`CANCELLED` ⇒ `winner_side_id IS NULL`; `status = 'CANCELLED'` ⇔ `end_reason = 'CANCELLED'`. Bảng phụ hoặc cột `pk_side_rooms.disconnected_at` để job biết phòng nào đang trong 30 giây chờ | 000005 | Phase phạt, xử thua TKO, job chốt trận theo `ends_at` |
| S9 | Bảng `host_pk_stats (user_id PK REFERENCES users, win_streak, max_win_streak, wins, losses, draws, updated_at)`, cập nhật trong cùng transaction chốt trận | 000005 | Chuỗi thắng. Tách bảng riêng thay vì thêm cột vào `users`, vì chỉ host dùng và cập nhật thường xuyên |
| S10 | `cashout_requests` thêm `gross_amount_vnd`, `tax_amount_vnd`, `net_amount_vnd` (BIGINT), `vnd_per_gem` (snapshot tỷ giá); CHECK `net = gross - tax`. KYC host (bảng `host_kyc`) để giai đoạn 2 | 000002 | Rút tiền và thuế (chờ kế toán) |
| S11 | D2: đổi `ON DELETE CASCADE` → `RESTRICT` cho FK từ bảng biên lai (`gift_logs`, `game_results`, `topup_orders`, `cashout_requests`) trỏ tới `users`, `rooms`, `gifts`, `games`, `economy_transactions` | 000002–000006 | Không mất biên lai khi xóa quà/game/phòng |
| S12 | Mở rộng ràng buộc không âm cho ví hệ thống có ngân sách: `CHECK (account_type NOT IN ('USER','SYSTEM_REWARD','SYSTEM_ESCROW') OR (coin >= 0 AND gem >= 0))` (thay cho `check_user_wallet_non_negative`) | 000002 | Ngân sách thưởng game có giới hạn; escrow không bao giờ âm |
| S13 | `games.webhook_secret_hash` (hoặc tham chiếu tới secret manager) + `games.allowed_ips`; bảng `game_reward_rules (game_id, rank_from, rank_to, reward_coin)`; `games.max_reward_per_result`, `max_reward_per_user_per_day`, `max_reward_per_day`. `game_results` dùng đủ luồng `PENDING → VALIDATED/REJECTED → REWARDED` | 000006 | Backend tự tính thưởng, có trần, secret riêng từng provider |

**Code phải sửa theo:**
- `SendGift`: khóa `users` của người gửi trước; chỉ host nhận GEM (bỏ nhánh `CO_HOST`); đọc rate của host; kiểm tra `min_level` và trạng thái `RESTRICTED`; cộng `exp`/`level`; ghi `target_user_id` và `gem_rate_percent`.
- `EndRoom` / chốt trận: phòng rời đi thì xử thua (TKO) thay vì chốt theo điểm. `GetOpenPKBattleIDByRoom` phải trả thêm `side_id` để biết phe nào bị TKO; `finishBattleTx` nhận thêm tham số "phe bị xử thua"; thêm phase `PUNISHMENT`; cập nhật `host_pk_stats`.
- Thêm job `pk_settlement` đọc `ends_at` và job kiểm tra rời trận quá 30 giây.
- `SubmitResult`: không tin `reward_coin` từ provider, tra `game_reward_rules`; áp trần, vượt trần thì để `PENDING`; xác thực bằng secret của đúng game.
- `JoinSession`: bắt thêm lỗi `unique_session_seat` → 409 (lưới phòng thủ, hiện không xảy ra vì đã có `FOR UPDATE`).
- API mới: kick thành viên (xem BUSINESS §3), `GET /rooms/:id/messages?after=` để lấy lại tin bị lỡ.
- Vận hành: khi tắt server, gửi close frame cho mọi WebSocket (`http.Server.Shutdown` không quản lý kết nối đã hijack); log có cấu trúc cả các lỗi 4xx của luồng tiền.

**Thứ tự triển khai** (mỗi nhóm: sửa migration gốc → `migrate down -all` → `up` → `sqlc generate` → `go build`):

| Nhóm | Gồm | Ghi chú |
|---|---|---|
| 1 | S7, S11, S12 | Nền tảng: enum loại giao dịch, CASCADE → RESTRICT, ví hệ thống có ngân sách không được âm |
| 2 | S1, S3, S5, S4, S2 | Cột cho user và quà, ví `CHARGEBACK_LOSS` |
| 3 | S8, S9 | PK |
| 4 | S13 | Thưởng game an toàn (trước khi tích hợp provider thật) |
| 5 | S6 | Nạp tiền, cần cho MVP-2 |
| 6 | S10 | Rút tiền, giai đoạn 2 |

Sau đó sửa code: `SendGift` → `EndRoom` và chốt trận PK → các job.

## 6. Quy trình làm việc với DB

```bash
# Tạo migration mới
migrate create -ext sql -dir db/migration -seq <ten_migration>

# Chạy migration
migrate -path db/migration -database "$DATABASE_URL" up
migrate -path db/migration -database "$DATABASE_URL" down 1

# Sinh code Go từ db/query/*.sql
sqlc generate
```

Checklist khi thêm bảng mới:

- [ ] Có `created_at`; có `updated_at` + trigger `set_updated_at()` nếu bảng được UPDATE.
- [ ] Mọi FK có `ON DELETE` được chọn có chủ đích (tài chính = RESTRICT).
- [ ] CHECK cho số lượng (`> 0` hoặc `>= 0`), độ dài text, quan hệ status ↔ timestamp.
- [ ] Index cho mọi FK và mọi cột dùng trong `WHERE`/`ORDER BY` thường xuyên.
- [ ] File `.down.sql` xóa đúng theo thứ tự ngược.
- [ ] Nếu đụng tiền: đi qua `postLedger`, có idempotency.

Câu kiểm tra sức khỏe sổ cái (nên chạy định kỳ):

```sql
-- 1. Tổng số dư mỗi currency phải = 0
SELECT SUM(coin) AS coin_total, SUM(gem) AS gem_total FROM wallets;

-- 2. Số dư ví phải khớp ledger
SELECT w.id, w.coin,
       COALESCE(SUM(CASE WHEN l.entry_type = 'CREDIT' THEN l.amount ELSE -l.amount END), 0) AS ledger_coin
FROM wallets w
LEFT JOIN economy_ledger_entries l ON l.wallet_id = w.id AND l.currency = 'COIN'
GROUP BY w.id, w.coin
HAVING w.coin <> COALESCE(SUM(CASE WHEN l.entry_type = 'CREDIT' THEN l.amount ELSE -l.amount END), 0);
-- (chạy lại câu trên với gem / currency = 'GEM')

-- 3. Nợ chargeback trong sổ cái phải khớp nợ ghi trên user (sau khi làm S5)
SELECT (SELECT -coin FROM wallets WHERE account_code = 'CHARGEBACK_LOSS') AS ledger_debt,
       (SELECT COALESCE(SUM(coin_debt), 0) FROM users)                   AS user_debt;
-- Hai cột phải bằng nhau.
```
