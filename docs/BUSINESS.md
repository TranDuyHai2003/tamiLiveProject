# Nghiệp vụ — TamiLive

> Mô tả **cái hệ thống phải làm** và **luật nghiệp vụ** của từng phân hệ.
> Chi tiết bảng xem [DATABASE.md](DATABASE.md), công nghệ xem [TECHSTACK.md](TECHSTACK.md).

## 0. Thuật ngữ

| Thuật ngữ | Nghĩa |
|---|---|
| **Viewer** | Người xem. Nạp COIN, tặng quà, chat, xin lên ghế game. |
| **Host** | Người phát sóng. Nhận GEM, mở PK, mở game, rút tiền. |
| **Agency** | Công ty quản lý nhiều host, nhận chia sẻ doanh thu. *(giai đoạn 2)* |
| **Admin** | Vận hành: quản lý quà, game, duyệt rút tiền, xử lý vi phạm. |
| **COIN** | Tiền ảo viewer mua bằng tiền thật. Chỉ dùng để tiêu. Không rút được. |
| **GEM** | Tiền ảo host nhận khi được tặng quà. Rút ra tiền thật được. |
| **Gem rate** | Tỷ lệ % COIN chuyển thành GEM cho host. Phần còn lại là phí nền tảng. Mặc định **50%**, host thuộc agency tối đa **60%**. VD 50% → quà 100 COIN cho host 50 GEM. |
| **Người nhận (receiver)** | Người nhận GEM của một món quà. **Luôn là host** của phòng. |
| **Đối tượng (target)** | Người được quà tác động tới (VD buff cho người chơi game). Có thể khác receiver; target **không** nhận GEM. |
| **Win streak** | Số trận PK thắng liên tiếp của host. Thua thì về 0; hòa hoặc trận bị hủy thì giữ nguyên. |
| **RESTRICTED** | Trạng thái user đang nợ coin do chargeback: vẫn đăng nhập và xem live được, nhưng không tặng quà, không lên mic. |

## 1. Lộ trình triển khai

Làm theo thứ tự — mỗi giai đoạn chạy được end-to-end trước khi sang giai đoạn sau. Các phase kỹ thuật để đạt từng giai đoạn xem [ROADMAP.md](ROADMAP.md).

| Giai đoạn | Phạm vi | Xong khi |
|---|---|---|
| **MVP-1** | Auth, tạo phòng, LiveKit token, xem live, chat realtime | 2 trình duyệt: 1 phát, 1 xem + chat được |
| **MVP-2** | Ví, nạp tiền (sandbox), danh mục quà, tặng quà + hiệu ứng | Tặng quà, số dư 2 bên đúng, sổ cái cân |
| **MVP-3** | PK 1v1 | 2 host đấu, điểm cập nhật live, có người thắng |
| **MVP-4** | Mini game iframe + lên ghế + thưởng | Chơi một ván, người thắng nhận COIN |
| **Giai đoạn 2** | Rút tiền, level, agency, kiểm duyệt, bảng xếp hạng, team PK | — |

## 2. Tài khoản & xác thực

**Luồng**
1. **Đăng ký** `POST /api/auth/register` — username, email, password. Tạo `users` + ví `USER` **trong cùng transaction**.
2. **Đăng nhập** `POST /api/auth/login` — trả access token (JWT, sống ~15 phút) + refresh token (chuỗi ngẫu nhiên, sống ~30 ngày, lưu **hash** vào `user_sessions`).
3. **Làm mới** `POST /api/auth/refresh` — đổi refresh token cũ lấy cặp mới (rotation), revoke session cũ.
4. **Đăng xuất** `POST /api/auth/logout` — set `revoked_at`.

**Luật**
- Mật khẩu băm bằng `bcrypt` (cost ≥ 10). Không bao giờ log mật khẩu hoặc token.
- JWT chứa `sub` (user_id), `role`, `exp`. Không chứa thông tin nhạy cảm.
- User `SUSPENDED`/`BANNED` không đăng nhập được; khi bị ban thì revoke toàn bộ session.
- User `RESTRICTED` (đang nợ coin do chargeback) vẫn đăng nhập và xem live được, nhưng **không được tặng quà, không được lên mic**. Tự động về `ACTIVE` khi trả hết nợ (xem 5.5).
- Nâng role `VIEWER → HOST`: MVP cho tự đăng ký làm host; sau này qua duyệt/agency.
- Rate limit đăng nhập theo IP + username để chống dò mật khẩu.

**Level** *(đã chốt)*
- **1 COIN tiêu (tặng quà) = 1 EXP.** Chỉ tính coin tiêu, không tính coin nạp.
- Level tra từ **bảng ngưỡng cố định** `level_thresholds` (seed data), không dùng công thức lúc chạy. User mới có level 1, exp 0.
- Ngưỡng (EXP tối thiểu để **đạt** level đó), tối đa level 50:

| Level | EXP tối thiểu | Cách sinh | ≈ VNĐ đã tiêu |
|---|---|---|---|
| 1 | 0 | — | 0 |
| 2 | 100 | mốc gốc | 10 nghìn |
| 3 → 10 | ×1,778 mỗi cấp | level 10 ≈ 10.000 | 100 nghìn |
| 11 → 50 | ×1,168 mỗi cấp | level 20 ≈ 47.000 · level 30 ≈ 224.000 · level 50 ≈ 5.000.000 | 470 nghìn · 2,2 triệu · 50 triệu |

  Một script sinh bảng này **một lần**, làm tròn, rồi lưu thành seed. Sau đó số liệu là cố định: muốn đổi thì sửa seed, không sửa công thức.
- Cập nhật `exp` và `level` **trong cùng transaction** tặng quà để không lệch.
- Level không bao giờ giảm.

**Đặc quyền theo level**

| Đặc quyền | Ví dụ | Backend làm gì |
|---|---|---|
| Màu name tag trong chat | Level 1–10 xám … level 40–50 đỏ/vàng | Gửi `level` kèm mỗi tin chat; client tự chọn màu |
| Hiệu ứng vào phòng | Level ≥ 30: thông báo nảy lên để host chào | Khi join phòng, nếu đủ level thì phát sự kiện `room.vip_enter` |
| Quà độc quyền | "Triệu hồi Rồng" chỉ level ≥ 20 | `gifts.min_level`; `SendGift` từ chối nếu người gửi chưa đủ level |

## 3. Livestream

**Vòng đời phòng:** `SCHEDULED → LIVE → ENDED` (hoặc `CANCELLED` nếu chưa từng LIVE).

**Luồng host**
1. `POST /api/rooms` — tạo phòng (title, category). Sinh `livekit_room_name`.
2. `POST /api/rooms/:id/start` — chuyển `LIVE`, set `started_at`. Mỗi host chỉ 1 phòng LIVE.
3. `POST /api/rooms/:id/token` — backend cấp LiveKit token **có quyền publish**.
4. Host phát từ trình duyệt (camera) hoặc OBS (qua LiveKit Ingress — xem TECHSTACK).
5. `POST /api/rooms/:id/end` — chuyển `ENDED`, set `ended_at`. Đóng PK/game đang mở, flush thống kê.

**Luồng viewer**
1. `GET /api/rooms?status=LIVE` — danh sách phòng đang live.
2. `POST /api/rooms/:id/join` — upsert `room_members`, trả LiveKit token **chỉ subscribe** (`canPublish=false`, `canPublishData=false`).
3. Kết nối WebSocket để nhận chat, quà, PK, game.

**Quyền trên LiveKit** do server tính từ role trong phòng, không bao giờ nhận từ client:

| Role | `canPublish` (hình/tiếng) | `canPublishData` | `canSubscribe` |
|---|---|---|---|
| HOST, CO_HOST | ✅ | ✅ | ✅ |
| VIEWER | ❌ | ❌ | ✅ |
| Người chơi game đang ngồi ghế | Chỉ microphone (qua `UpdateParticipant`) | ❌ | ✅ |

Viewer **không có** `canPublishData`: data message của LiveKit đi thẳng giữa các client, không qua backend, nên sẽ né được mọi kiểm tra chat (kick, ban, độ dài, rate limit). Chat của viewer bắt buộc đi qua WebSocket.

**Luật**
- Viewer bị `KICKED`/`BANNED` trong phòng không lấy được token mới.
- **Kick** *(chưa làm)* phải làm đủ 3 việc trong cùng một thao tác, vì mỗi kênh có cơ chế riêng:
  1. `room_members.status = 'KICKED'`: chặn lấy token mới và chặn gửi chat.
  2. Gọi LiveKit `RemoveParticipant`: cắt video ngay. Token đã cấp không thu hồi được, nên đi kèm **hạn token ngắn (~10 phút)**; LiveKit tự gia hạn cho người đang kết nối hợp lệ.
  3. Publish sự kiện `member.kicked {user_id}` lên kênh phòng: server đang giữ WebSocket của người đó gửi close frame và đóng kết nối. Không có bước này thì người bị kick vẫn **đọc** được chat cho tới khi họ thử gửi tin.
- Host mất kết nối quá N phút (LiveKit webhook `room_finished`/`participant_left`) → tự kết thúc phòng.
- Danh sách người xem và số người xem lấy từ **Redis** (presence), không query `room_members`.

## 4. Tương tác realtime: chat, reaction, broadcast

**Kênh sự kiện:** mọi sự kiện của một phòng đi qua WebSocket. Định dạng chung:

```json
{ "type": "chat.message", "room_id": "...", "ts": 1760000000000, "data": { } }
```

Các `type` chính: `chat.message`, `chat.deleted`, `reaction.burst`, `gift.sent`, `room.viewer_count`, `room.ended`, `pk.started`, `pk.score`, `pk.ended`, `game.*`, `global.vip_enter`.

**Chat**
- Gửi chat qua WebSocket. Server kiểm tra: đã join phòng, không bị mute/ban, độ dài 1–500 ký tự, rate limit (VD 1 tin / giây / user).
- *(đã chốt cho MVP)* **Ghi `chat_messages` đồng bộ trước, rồi mới publish Redis.** Câu INSERT kiểm tra luôn "phòng LIVE + người gửi JOINED" một cách nguyên tử, nên user vừa bị kick không gửi được tin tiếp theo. Server sập cũng không mất lịch sử. Đổi lại, mỗi tin tốn một lần ghi DB. Chỉ chuyển sang ghi theo lô (batch) khi đo được DB nghẽn vì chat.
- Host/moderator xóa tin → set `deleted_at` + phát `chat.deleted`.
- **Lấy lại tin bị lỡ** *(chưa làm)*: Redis Pub/Sub không lưu tin, nên client mất kết nối là mất các sự kiện trong khoảng đó. `GET /api/v1/rooms/:id/messages?after=<id hoặc sent_at của tin cuối đã thấy>` để client gọi một lần sau khi kết nối lại. Điểm PK, số dư... cũng lấy lại qua REST, không trông vào Redis.

**Reaction (thả tim)**
- Không ghi DB từng lượt. `INCR` counter trong Redis, server gom và phát `reaction.burst` theo nhịp (VD 300ms).
- Job định kỳ flush tổng sang `room_reaction_stats`.

**Global broadcast**
- Sự kiện toàn server (đại gia vào phòng, quà siêu lớn) publish lên kênh Redis `global_events`. Mọi node WebSocket đều subscribe và đẩy xuống mọi client.
- Giới hạn tần suất để không spam toàn hệ thống.

## 5. Kinh tế ảo & quà tặng (lõi)

### 5.0 Tỷ giá và bảng giá *(đã chốt)*

| Hạng mục | Giá trị |
|---|---|
| Nạp qua web / chuyển khoản (cổng VN) | 10.000 VNĐ = **100 COIN** |
| Nạp trong app iOS / Android (IAP) | 10.000 VNĐ ≈ **70 COIN** (theo tier giá của Apple/Google, VD $0.99 = 65 COIN). User chịu phí store 15–30%, nên nền tảng luôn thu đủ giá trị của số coin phát hành |
| Rút GEM | 1 GEM = **100 VNĐ** |
| Mức rút tối thiểu | **5.000 GEM** (= 500.000 VNĐ) |
| Phí chuyển khoản khi rút | Nền tảng chịu |

Ví dụ với rate 50%: nạp web 10.000đ → 100 COIN → tặng quà → host nhận 50 GEM → rút được 5.000đ.

Bảng giá lưu trong `coin_packages`, mỗi gói gắn với một kênh (`WEB`, `IOS`, `ANDROID`). Client chỉ thấy các gói thuộc kênh của mình.

> ⚠️ Chính sách của Apple/Google về việc nhắc tới giá rẻ hơn trên web **ngay trong app** thay đổi theo từng khu vực. Kiểm tra trước khi làm UI hiển thị so sánh giá.

### 5.1 Nạp tiền (TOPUP)
1. Client chọn gói → `POST /api/topup/orders` → backend tạo đơn `PENDING`, trả URL thanh toán (web) hoặc product id (IAP).
2. Cổng thanh toán (hoặc Apple/Google server notification) gọi **webhook** về backend. Backend xác minh chữ ký/receipt, rồi trong 1 transaction: đánh dấu đơn `COMPLETED` + tạo transaction `TOPUP` (FIAT_RESERVE → user).
3. `provider_txn_id` UNIQUE → webhook gọi lặp lại không cộng tiền 2 lần.
4. **Không bao giờ** cộng tiền dựa trên redirect/callback từ phía client.
5. Nếu user đang có `coin_debt` (xem 5.5): **trong cùng transaction** nạp, cộng coin rồi trừ ngay phần trả nợ về ví `CHARGEBACK_LOSS`, giảm `coin_debt` tương ứng; trả hết nợ thì `RESTRICTED → ACTIVE`.
6. Đối soát: Google Play báo hoàn tiền qua RTDN, Apple qua App Store Server Notifications. Có thêm **job hằng ngày** gọi API Voided Purchases (Google) để bắt các lần hoàn tiền bị lỡ webhook.

### 5.2 Tặng quà (GIFT)
`POST /api/rooms/:id/gifts` với header `Idempotency-Key`.

Trong **một DB transaction** (`GiftService.SendGift`):
1. Ghi idempotency key; nếu trùng key → trả lại kết quả cũ (hoặc lỗi nếu body khác).
2. Khóa dòng `users` của người gửi (`FOR NO KEY UPDATE`) **trước mọi bước đụng tới ví**. Đây là thứ tự khóa chung của mọi giao dịch tiền (xem DATABASE.md §3.5), đồng thời chặn trường hợp chargeback chuyển user sang `RESTRICTED` đúng lúc quà đang được xử lý.
3. Kiểm tra: người gửi `ACTIVE` (không `RESTRICTED`), phòng `LIVE` (khóa `FOR SHARE`), quà đang bán, người gửi đủ `gifts.min_level`, không tự tặng mình. Nếu có `target_user_id` thì target phải đang ở trong phòng.
4. Tính `total_gem = total_coin × gem_rate%`, với gem rate lấy **của host nhận quà** (`users.gem_rate_percent`; NULL = mặc định 50%).
5. Hạch toán 4 leg: viewer −COIN, PLATFORM_REVENUE +COIN, GEM_LIABILITY −GEM, host +GEM. Không đủ COIN → lỗi `402`, rollback toàn bộ.
6. Ghi `gift_logs` (snapshot `total_coin`, `total_gem`, `gem_rate_percent`, `target_user_id`), cộng điểm PK nếu trận đang `ACTIVE`, cộng `exp`/`level` cho người gửi.
7. COMMIT.

**Sau COMMIT** (không nằm trong transaction): publish `gift.sent` lên Redis → client render hiệu ứng; nếu có PK thì cập nhật điểm Redis.

**Luật**
- **Chỉ host nhận GEM.** Người xem có thể chọn một **target** khác (VD người chơi game) để buff/debuff, nhưng tiền luôn chảy về host. Lý do: tránh phải KYC toàn bộ user và giữ động lực cho host. *(Code hiện tại còn cho phép `CO_HOST` nhận GEM, cần sửa.)*
- Số lượng mỗi lần: 1–9999. Giá và rate snapshot vào `gift_logs`, nên đổi giá hay đổi rate sau này không làm sai lịch sử.
- Client sinh `Idempotency-Key` (UUID) **một lần cho mỗi lần bấm**, gửi lại y nguyên khi retry.
- Nếu publish Redis lỗi sau khi commit: tiền vẫn đúng, chỉ mất hiệu ứng — chấp nhận được. Không rollback tiền vì lỗi hiển thị.
- MVP **chỉ tặng thẳng** (trừ coin → ra hiệu ứng ngay). Kho quà (mua trước, tặng sau) để giai đoạn 2.

**Gem rate theo host** *(đã chốt)*
- Host tự do: 50% (mặc định hệ thống).
- Host thuộc agency: có thể đặt riêng, **tối đa 60%** (DB chặn bằng `CHECK (gem_rate_percent BETWEEN 0 AND 60)`). 40% còn lại bù chi phí vận hành: server, LiveKit, SMS OTP, phí chuyển khoản khi rút.
- Agency tự chia lại với host **bên ngoài hệ thống**; hệ thống chỉ trả cho host.

### 5.3 Rút tiền (CASHOUT) — giai đoạn 2
1. Host yêu cầu rút X GEM (≥ 5.000 GEM) → transaction `CASHOUT_HOLD`: host −GEM, CASHOUT_ESCROW +GEM. Trạng thái `REQUESTED`.
2. Admin duyệt → chuyển khoản thật → `CASHOUT_PAYOUT`: ESCROW −GEM, GEM_LIABILITY +GEM. Trạng thái `COMPLETED`.
3. Bị từ chối/thất bại → `CASHOUT_RELEASE`: ESCROW −GEM, host +GEM. Trạng thái `REJECTED`/`FAILED`.
4. Thông tin tài khoản ngân hàng (`payout_account`) phải được mã hóa ở tầng ứng dụng.
5. Lưu số tiền VNĐ tại thời điểm rút: `gross_amount_vnd` (= GEM × 100), `tax_amount_vnd`, `net_amount_vnd` (số thực chuyển). Thuế tính bằng VNĐ, **không** trừ bằng GEM trong sổ cái.

**Thuế TNCN**
- Mức thuế (`tax_rate`) và ngưỡng (`tax_threshold_vnd`) **lưu trong cấu hình**, được chép lại (snapshot) vào `cashout_requests` lúc tạo yêu cầu. Đề xuất ban đầu: 10% từ 2.000.000đ. *Con số cụ thể chờ kế toán xác nhận.*
- Chống chia nhỏ: tối đa **1 lần rút mỗi tuần**; ngưỡng thuế xét theo **tổng cộng dồn trong tháng**.
- **KYC host** bắt buộc trước khi rút lần đầu: CCCD (số CCCD dùng làm mã số thuế cá nhân), và tên chủ tài khoản ngân hàng phải trùng tên trên CCCD.

### 5.4 Hoàn tiền (REFUND) *(đã chốt)*
- **Không có API hoàn tiền cho client.** Hàng số tiêu hao ngay; cho hoàn sẽ bị lợi dụng (VD tặng quà, host thắng PK, rồi xin hoàn).
- Trường hợp duy nhất: **lỗi hệ thống** được admin xác minh qua ticket hỗ trợ. Với kiến trúc hiện tại thì rất hiếm, vì giao dịch lỗi giữa chừng đã bị rollback toàn bộ.
- Admin bấm **Manual Refund** trên CMS → transaction `REFUND`: PLATFORM_REVENUE −COIN → viewer +COIN. Chỉ hoàn **COIN**, không hoàn VNĐ.
- **Không đụng vào GEM của host**, kể cả khi host chưa rút: lỗi thuộc về nền tảng, host không có lỗi. Kế toán ghi nhận là chi phí hệ thống.
- Mỗi giao dịch gốc chỉ được hoàn **một lần** (DB chặn bằng unique trên `reference_id` của transaction `REFUND`).

### 5.5 Chargeback (Apple/Google/ngân hàng hoàn tiền ép buộc) *(đã chốt)*
1. Nhận webhook chargeback/refund từ Apple, Google hoặc cổng thanh toán, kèm `provider_txn_id` của đơn nạp gốc.
2. Trong 1 transaction `CHARGEBACK` (bị hoàn X coin, user còn B coin):
   - Thu hồi phần còn lại: user −min(B, X) → FIAT_RESERVE +min(B, X).
   - Phần đã tiêu (X − B, nếu > 0): CHARGEBACK_LOSS −(X − B) → FIAT_RESERVE +(X − B), và cộng `users.coin_debt += X − B`.
   - Nhờ vậy `FIAT_RESERVE` luôn khớp với số tiền thật đã thu, và khoản nợ có dấu vết đầy đủ trong sổ cái.
3. Nếu còn nợ: user chuyển sang `RESTRICTED`. Lần nạp sau trừ nợ trước (xem 5.1 bước 5).
4. Ngưỡng ban *(đã chốt, lưu trong cấu hình, không viết cứng trong code)*:
   - Lần 1: chỉ `RESTRICTED` nếu còn nợ.
   - Lần 2 trong vòng 180 ngày, **hoặc** nợ ≥ 100.000 COIN (≈ 1 triệu đồng): `BANNED`.
5. **Host không bị trừ GEM.** Khoản thất thoát ghi nhận là chi phí rủi ro (fraud loss).

> Vì sao không cho ví âm: ràng buộc `check_user_wallet_never_negative` (ví USER không bao giờ được âm) là thứ đảm bảo không ai tiêu quá số dư (kể cả khi 2 request chạy song song). Nới nó ra cho chargeback sẽ mở lỗ hổng cho mọi luồng khác. Ghi nợ ở cột riêng giữ nguyên được bảo đảm đó.

## 6. PK Battle

**Vòng đời:** `PENDING` (đã mời, chờ chấp nhận) → `COUNTDOWN` (đếm 3-2-1) → `ACTIVE` → `PUNISHMENT` (1–2 phút phạt) → `FINISHED`. Trận có thể chuyển sang `CANCELLED` từ `PENDING`, `COUNTDOWN` hoặc `ACTIVE`.

Kết quả được **chốt ngay khi hết `ACTIVE`**: người thắng (hoặc hòa) xác định lúc đó và không đổi nữa. `PUNISHMENT` chỉ là giai đoạn trình diễn.

**Luồng 1v1**
1. Host A mời host B (cả hai đang LIVE, không ở trận khác) → `PENDING`. Hết 30 giây không trả lời → `CANCELLED`.
2. B chấp nhận → tạo `pk_battles` + 2 `pk_sides` + `pk_side_rooms`, lưu `ends_at = now + duration`.
3. Mỗi quà tặng vào phòng đang PK → cộng điểm cho phe của phòng đó **trong cùng transaction tặng quà** (đã làm trong `addPKScoreForGift`). Điểm = `total_coin × multiplier`.
4. Hết giờ (`ends_at`) → job settlement khóa trận, chọn phe điểm cao nhất:
   - Có người thắng → `PUNISHMENT` (60 giây), cập nhật chuỗi thắng → hết phạt thì `FINISHED`, giải phóng phòng.
   - Hòa (`winner_side_id = NULL`) → `FINISHED` ngay, giải phóng phòng.

**Thông số MVP** *(đã chốt)*: thời lượng cố định **300 giây**, đếm ngược **3 giây**, phạt **60 giây**. Cột `duration_seconds` đã sẵn để sau này cho host chọn 180/300/600 giây mà không đổi schema. Chỉ host **thắng** được bấm tha phạt sớm. Host thua không được tự bỏ qua, vì đó là lý do người xem tặng quà cứu host.

**Hiển thị**
- Gửi cho client `ends_at` (giờ server) một lần; **client tự đếm ngược**. Server không gửi tick từng mili-giây.
- Điểm: sau mỗi quà, `HINCRBY` Redis; một goroutine gom và phát `pk.score` tối đa ~5–10 lần/giây mỗi trận.

**Buff X2**
- Server chọn ngẫu nhiên khung thời gian buff, lưu `multiplier` + thời điểm vào Redis và phát `pk.buff`. Điểm của quà trong khung đó nhân hệ số; lưu `multiplier` vào `pk_score_events` để audit.

**Luật quan trọng**
- **Postgres là trọng tài.** Kết quả thắng thua lấy từ `pk_sides.score` (hoặc `SUM(pk_score_events)`), **không** từ Redis.
- **Không dựa vào TTL Redis để kết thúc trận.** Key Redis đặt TTL = thời lượng + biên an toàn (VD +10 phút) chỉ để tự dọn rác. Kết thúc trận do job đọc `ends_at` trong Postgres — server restart vẫn kết thúc đúng.

**Kết quả, phạt và chuỗi thắng** *(đã chốt)*

| Tình huống | Kết quả | Win streak | Phạt |
|---|---|---|---|
| Hết giờ, một phe điểm cao hơn | Phe đó thắng | Thắng +1, thua về 0 | Phe thua bị phạt 1–2 phút |
| Hết giờ, điểm bằng nhau | Hòa, cả hai hiện "Draw" | Giữ nguyên | Không |
| Một phòng rời trận (tắt live, mất mạng) quá **30 giây** | **Xử thua (TKO)** dù đang dẫn điểm. Đối thủ thắng | Như thắng/thua | Có, nếu phe thua còn ở trong phòng |
| Rời trận nhưng **quay lại trong 30 giây** | Trận tiếp tục bình thường | — | — |
| **Cả hai** phòng cùng rời, hoặc LiveKit/server của nền tảng gặp sự cố | `CANCELLED`, không ai thắng | Giữ nguyên cả hai | Không |

- Lý do xử thua khi rời trận: chống việc host thấy sắp thua thì cố tình thoát để giữ chuỗi thắng.
- Lỗi phía nền tảng **không bao giờ** được làm host thua.
- **Quà tặng trong lúc `PUNISHMENT` (hoặc sau khi trận kết thúc):** không cộng điểm PK, nhưng vẫn trừ COIN, host vẫn nhận GEM, người gửi vẫn nhận EXP, hiệu ứng vẫn hiện. Code hiện tại đã đúng điểm này: `addPKScoreForGift` chỉ cộng khi trận `ACTIVE`.
- GEM đã tặng trước khi trận bị hủy **không hoàn lại**: các giao dịch đó đã hợp lệ và hoàn tất.
- Hiệu ứng phạt (filter mặt hề, méo giọng, grayscale) do **app phía người xem** áp lên video của phe thua. Backend chỉ phát sự kiện `pk.punishment` gồm phe thua và `punishment_ends_at`.

**Phát hiện rời trận:** dựa trên webhook `participant_left` / `room_finished` của LiveKit cho host, cộng một job kiểm tra hết hạn 30 giây. *(Code hiện tại: `EndRoom` đang chốt trận **theo điểm**, cần sửa thành "phòng rời đi thì thua".)*

**Các trường hợp đặc biệt khi xử thua TKO**

| Tình huống | Xử lý |
|---|---|
| Host **bấm "Kết thúc live"** giữa trận | Cố ý rời → **xử thua ngay**, không chờ 30 giây |
| Hết giờ đúng lúc một phe đang trong 30 giây chờ | Đợi hết 30 giây đó. Quay lại → chốt theo điểm; không quay lại → TKO |
| "Cả hai cùng rời" | Phe thứ hai rời **trong lúc** phe thứ nhất vẫn còn trong 30 giây chờ → `CANCELLED` |
| Team PK (giai đoạn 2) | Một phe chỉ thua TKO khi **tất cả** phòng của phe đó đã rời |

## 7. Mini game (iframe bên thứ ba)

**Luồng**
1. Host mở game → `game_sessions` `WAITING`.
2. Viewer bấm "xin lên ghế" → `game_players` `REQUESTED`.
3. Host duyệt → `APPROVED` + gán `seat_number`. Backend gọi LiveKit `UpdateParticipant` cho phép publish **chỉ microphone**.
4. Host bắt đầu → session `RUNNING`, player `PLAYING`. Backend sinh **game token** (JWT ngắn hạn, ký bằng secret riêng của từng provider) chứa `session_id`, `room_id`, danh sách `player_id`/ghế.
5. Next.js render `<iframe src="{endpoint_url}?token=...">`.
6. Ván kết thúc → **game server gọi webhook backend** (server-to-server), ký HMAC → tạo `game_results` `PENDING` → xác minh → `VALIDATED` → trả thưởng (`GAME_REWARD`) → `REWARDED`.
7. Player rời ghế/bị kick → revoke quyền publish trên LiveKit.

**Tương tác người xem với game**
- Viewer tặng quà "buff" **cho một người chơi cụ thể** → backend xử lý quà như mục 5.2 với `target_user_id` = người chơi đó. **GEM vẫn về host**, người chơi không nhận GEM.
- Sau commit: gửi lệnh tới **game server** (API server-to-server) và phát sự kiện `game.buff` để client `postMessage` vào iframe hiển thị hiệu ứng.

**Luật bảo mật**
- ⚠️ **Kết quả game không bao giờ được tin từ trình duyệt.** `postMessage({action:"GAME_OVER"})` từ iframe chỉ dùng để cập nhật UI. Việc trả thưởng **chỉ** dựa trên webhook có chữ ký từ game server. Nếu cho client gửi kết quả lên, người chơi sửa JS là tự nhận thưởng được.
- Next.js khi nhận `message` phải kiểm tra `event.origin` đúng domain của provider; khi gửi phải chỉ định `targetOrigin`, không dùng `"*"`.
- iframe dùng `sandbox="allow-scripts allow-same-origin"` **chỉ khi** game nằm khác origin với web chính.

**Luật trả thưởng** *(chưa làm; code hiện tại trả đúng số `reward_coin` mà game server gửi lên, không giới hạn)*

Chữ ký HMAC chỉ trả lời "tin này đến từ ai", không trả lời "số tiền này có hợp lý không". Game server bị hack (hoặc lộ secret) vẫn ký đúng được. Vì vậy cần nhiều lớp:

| Lớp | Luật | Vai trò |
|---|---|---|
| Nghiệp vụ (chính) | **Backend tự quyết số thưởng**: game server chỉ báo thứ hạng/điểm; backend tra bảng thưởng của từng game (`game_reward_rules`) ra số COIN | Chặn từ gốc |
| Nghiệp vụ | **Trần thưởng**: mỗi kết quả, mỗi phiên, mỗi user/ngày, mỗi game/ngày. Vượt trần → kết quả ở `PENDING`, chờ admin duyệt (`VALIDATED`/`REJECTED`), không trả ngay | Giới hạn mỗi lần bị lạm dụng |
| Sổ cái (lưới an toàn) | **`SYSTEM_REWARD` không được âm.** Admin nạp ngân sách định kỳ bằng giao dịch `ADJUSTMENT`: `PROMO_BUDGET` −X → `SYSTEM_REWARD` +X. Tách ví `PROMO_BUDGET` để doanh thu (`PLATFORM_REVENUE`) không bị lẫn với chi phí khuyến mãi. Hết ngân sách thì DB tự chặn *(đã làm ở phase 1; dev nạp bằng `db/scripts/dev_fund_reward_budget.sql`)* | Thiệt hại tối đa = ngân sách còn lại, kể cả khi code có bug |
| Xác thực | **Mỗi provider một secret riêng**, xoay vòng được; danh sách IP được phép gọi webhook | Lộ một secret không ảnh hưởng game khác |
| Giám sát | Cảnh báo khi tổng thưởng/giờ tăng bất thường hoặc một user nhận thưởng liên tục; đối soát sổ cái hằng ngày | Phát hiện nhanh |

Ví dụ kịch bản phải chặn được: kẻ gian có secret, tự mở phòng, cho 16 tài khoản phụ lên ghế, gửi kết quả thưởng 1.000.000 COIN cho cả 16 người, lặp lại nhiều phiên.

## 8. Đánh giá nghiệp vụ gốc — những điểm đã chỉnh

| Bản mô tả gốc | Vấn đề | Đã chỉnh trong tài liệu này |
|---|---|---|
| "Lưu trữ thông qua GORM", `tx.Clauses(clause.Locking...)` | Dự án thực tế dùng **sqlc + pgx**, không dùng GORM. Hai tài liệu mâu thuẫn. | Dùng sqlc + pgx; khóa bằng `UPDATE ... WHERE coin + delta >= 0` (atomic, ít round-trip hơn `SELECT FOR UPDATE`). |
| JWT lưu LocalStorage | LocalStorage đọc được bằng JS → lộ token khi bị XSS. | Access token giữ trong bộ nhớ, refresh token trong cookie `HttpOnly; Secure; SameSite`. |
| JWT trên query param WebSocket | Token lọt vào log của proxy/server. | Lấy **ws-ticket** dùng một lần (sống 30 giây) qua REST rồi mới mở WebSocket với ticket đó. |
| PK lưu Redis với TTL = thời lượng trận | Key hết hạn đúng lúc cần chốt điểm → mất dữ liệu; restart server là mất trận. | Postgres lưu `ends_at` và điểm; Redis chỉ để hiển thị. |
| Đếm ngược "đến mili-giây" qua server | Tốn băng thông, không chính xác do độ trễ mạng. | Gửi `ends_at`, client tự đếm. |
| Điểm PK chỉ `HINCRBY` Redis | Ghi 2 nơi không cùng transaction → có thể lệch. | Ghi Postgres trong transaction quà, Redis là bản sao. |
| Iframe gửi `GAME_OVER` → Next.js gọi API trả thưởng | Client tự giả kết quả được. | Webhook có chữ ký từ game server. |
| Không nhắc nạp tiền, rút tiền | DB có bảng nhưng nghiệp vụ không có luồng. | Bổ sung mục 5.1 và 5.3. |
| Level "dựa trên tiền nạp và tiêu" | Không nói lưu ở đâu, cập nhật khi nào. | `exp` cập nhật cùng transaction. |
| Agency | Có role nhưng không có nghiệp vụ. | Chuyển sang giai đoạn 2. |

## 9. Quyết định nghiệp vụ

### 9.1 Đã chốt

| # | Câu hỏi | Quyết định | Chi tiết |
|---|---|---|---|
| 1 | Gem rate | Mặc định 50%; host agency tối đa 60% | 5.2 |
| 2 | Tỷ giá, rút tiền | Web 100đ = 1 COIN; IAP ≈ 70 COIN / 10.000đ; 1 GEM = 100đ khi rút; tối thiểu 5.000 GEM; nền tảng chịu phí chuyển khoản | 5.0, 5.3 |
| 3 | PK hòa / phạt | Hòa: không ai mất chuỗi thắng. Thua: phạt 1–2 phút | 6 |
| 4 | Rời PK giữa trận | Xử thua sau 30 giây không quay lại; cả hai rời hoặc nền tảng lỗi thì hủy trận | 6 |
| 5 | Quà cho người chơi game | Được, qua `target_user_id`; chỉ host nhận GEM | 5.2, 7 |
| 6 | Kho quà | MVP chỉ tặng thẳng; kho quà để giai đoạn 2 | 5.2 |
| 7 | Level | 1 COIN tiêu = 1 EXP; bảng ngưỡng cố định; đặc quyền: màu tên, hiệu ứng vào phòng, quà độc quyền | 2 |
| 8 | Hoàn tiền | Không có API cho client; admin hoàn COIN thủ công khi lỗi hệ thống, không đụng GEM host | 5.4 |
| 9 | Giá IAP | Tách bảng giá theo kênh; user chịu phí store | 5.0 |
| 10 | Chargeback | Trừ coin còn có, phần thiếu ghi nợ, user `RESTRICTED`; lạm dụng thì `BANNED`; host giữ GEM | 5.5 |
| 11 | Quà lúc phạt | Không cộng điểm PK; vẫn trả GEM và EXP | 6 |
| 12 | Thời lượng PK | 300 giây, đếm ngược 3 giây, phạt 60 giây; chỉ host thắng được tha phạt sớm | 6 |
| 13 | Ngưỡng chargeback | Lần 1: `RESTRICTED`; lần 2 trong 180 ngày hoặc nợ ≥ 100.000 COIN: `BANNED`; lưu trong cấu hình | 5.5 |
| 14 | Bảng EXP | Level 1 = 0, level 2 = 100, ×1,778/cấp tới level 10, ×1,168/cấp tới level 50 | 2 |
| 15 | Rút tiền (phần kỹ thuật) | Mức thuế và ngưỡng lưu trong cấu hình rồi snapshot vào `cashout_requests`; tối đa 1 lần rút/tuần; xét ngưỡng thuế theo tổng cộng dồn trong tháng; KYC bằng CCCD, tên chủ tài khoản ngân hàng phải trùng tên trên CCCD | 5.3 |
| 16 | Quyền gửi data trên LiveKit | Chỉ HOST/CO_HOST; viewer chat qua WebSocket *(đã sửa trong code)* | 3 |
| 17 | Kick | Đổi trạng thái thành viên + `RemoveParticipant` + đóng WebSocket qua sự kiện; token LiveKit hạn ~10 phút | 3 |
| 18 | Ghi chat | Đồng bộ (DB trước, Redis sau) cho MVP; có API lấy lại tin bị lỡ | 4 |
| 19 | Thưởng game | Backend tự tính thưởng theo bảng; có trần, vượt trần chờ duyệt; `SYSTEM_REWARD` có ngân sách, không được âm; secret riêng từng provider | 7 |

### 9.2 Còn mở

- [ ] **Con số trần thưởng game:** tối đa bao nhiêu COIN mỗi kết quả, mỗi user/ngày, mỗi game/ngày; ngân sách `SYSTEM_REWARD` nạp bao nhiêu mỗi tháng.

- [ ] **Thuế TNCN (phần pháp lý):** kế toán xác nhận mức 10% cho khoản từ 2.000.000đ còn áp dụng sau đợt sửa luật thuế TNCN, và việc dùng số CCCD làm mã số thuế cá nhân (áp dụng từ 7/2025). Code không phải chờ, vì các con số nằm trong cấu hình.
