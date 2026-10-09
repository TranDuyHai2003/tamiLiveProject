# Lộ trình triển khai — TamiLive backend

> Một nguồn duy nhất cho câu hỏi "đang làm gì, làm tiếp gì, xong khi nào".
> Luật nghiệp vụ xem [BUSINESS.md](BUSINESS.md), schema xem [DATABASE.md](DATABASE.md) (mã S1–S13), kiến trúc xem [TECHSTACK.md](TECHSTACK.md).

## Cách làm mỗi phase

| Bước | Việc | Ai |
|---|---|---|
| 1. Giải thích | Phase này giải quyết vấn đề nghiệp vụ gì, sửa những gì, vì sao | Claude |
| 2. Chốt | Các quyết định còn mở trong phase | Bạn |
| 3. Làm | Sửa migration → `sqlc generate` → sửa code | Claude |
| 4. Kiểm chứng | Chạy đúng các tiêu chí "Xong khi" của phase trên DB tạm | Claude, báo kết quả |
| 5. Tổng kết và commit | Đã đổi gì, luật nào giờ được bảo vệ ở đâu | Claude giải thích, bạn duyệt |

Mỗi phase kết thúc ở trạng thái **build được, chạy được, đã commit**. Không phase nào để code dở.

## Tổng quan

| Phase | Tên | Mở khóa cho sản phẩm | Trạng thái |
|---|---|---|---|
| 0 | Nền móng | Đăng ký, đăng nhập, mở phòng, vào phòng, chat, tặng quà | ✅ Xong |
| 1 | Luật nền của dữ liệu tài chính | (không có tính năng mới, chỉ siết luật) | ✅ Xong |
| **2** | **Tặng quà theo luật đã chốt** | Rate theo host, level/EXP, quà độc quyền | 🔜 **Tiếp theo** |
| 3 | Nạp tiền | User có coin thật (sandbox) → **xong MVP-2** | Chờ |
| 4 | Realtime hoàn chỉnh | Kick, lấy lại tin bị lỡ, sự kiện phòng | Chờ |
| 5 | PK | Mời, đấu, chốt trận, phạt, chuỗi thắng → **xong MVP-3** | Chờ |
| 6 | Mini game an toàn | Thưởng có trần, host duyệt ghế, mic → **xong MVP-4** | Chờ |
| 7 | Rút tiền | Host rút GEM ra tiền, KYC, thuế → giai đoạn 2 | Chờ |

**Vì sao thứ tự này:** đi từ **luật** (phase 1) → **tiền** (phase 2, 3) → **tương tác** (phase 4, 5, 6) → **chi trả** (phase 7). Phase sau xây trên phase trước: PK cộng điểm bên trong transaction tặng quà (phase 2), mini game trả thưởng qua sổ cái và ngân sách (phase 1), rút tiền cần ví escrow không được âm (phase 1).

---

## Phase 0 — Nền móng ✅

**Đã có:** auth JWT + bcrypt; ví tạo cùng lúc đăng ký; sổ cái kế toán kép (`postLedger`); tặng quà có idempotency, khóa theo thứ tự, cộng điểm PK trong cùng transaction; phòng LIVE, vào phòng, token LiveKit do server tính quyền; chat WebSocket qua Redis; PK và mini game ở tầng service; 6 migration gộp đủ ràng buộc; tài liệu nghiệp vụ.

**Cần hiểu:** Phần 1–4 của chuỗi giải thích (bức tranh hệ thống, luồng tiền, phòng và chat, PK và mini game).

---

## Phase 1 — Luật nền của dữ liệu tài chính ✅

**Vấn đề nghiệp vụ:** dữ liệu tiền phải trả lời đúng ba câu hỏi, và DB phải **ép** được, không chỉ dựa vào code:
1. Tiền này chuyển đi vì lý do gì? → **S7** loại giao dịch
2. Có được làm mất biên lai không? → **S11** chính sách xóa
3. Ví nào là quỹ có hạn, ví nào là nguồn phát hành? → **S12** ví được âm

**Phạm vi**

| Lớp | Thay đổi |
|---|---|
| DB | 000002: enum loại giao dịch mới, hoàn tiền phải có giao dịch gốc + chỉ hoàn 1 lần, cột `wallets.allow_negative` + CHECK khóa cứng ví USER, ví mới `PROMO_BUDGET`, `wallets.user_id` RESTRICT. 000004 + 000006: CASCADE → RESTRICT ở bảng biên lai |
| Query | `ApplyWalletCoinDelta` / `ApplyWalletGemDelta` đọc `allow_negative` |
| Code | `GIFT_SEND` → `GIFT`; thưởng game hết ngân sách báo rõ "game reward budget exhausted" |
| Dev | `db/scripts/dev_fund_reward_budget.sql`: `PROMO_BUDGET` → `SYSTEM_REWARD` |

**Đã chốt:** dùng cột `allow_negative` + CHECK khóa cứng ví USER; `PLATFORM_REVENUE` được âm (để luôn hoàn được tiền khi nền tảng gây lỗi); ngân sách thưởng lấy từ ví riêng `PROMO_BUDGET`, không trừ doanh thu.

**Cần hiểu:** phân biệt ví phát hành / ví quỹ; vì sao đặt chốt chặn RESTRICT ở biên lai chứ không ở bảng cha; vì sao một luật không nên nằm ở hai nơi.

**Xong khi:**
- [x] `migrate down -all` → `up` sạch (không sót function/enum/bảng); `sqlc generate`; `go build`
- [x] Xóa một món quà đã từng được tặng → DB chặn (quà chưa ai tặng thì vẫn xóa được). Xóa user, phòng, game, phiên game có biên lai → DB chặn
- [x] Hoàn tiền cùng một giao dịch hai lần → DB chặn; hoàn tiền không ghi giao dịch gốc → DB chặn
- [x] Trả thưởng game khi `SYSTEM_REWARD` hết ngân sách → 402, không phải 500; nạp ngân sách rồi gửi lại → trả thưởng được. Bật `allow_negative` cho ví USER → DB chặn
- [x] Tặng quà vẫn chạy (loại `GIFT`); 30 request song song + idempotency không lỗi; tổng số dư = 0; số dư mọi ví khớp sổ cái

---

## Phase 2 — Tặng quà theo luật đã chốt

**Vấn đề nghiệp vụ:** `SendGift` hiện chạy theo luật cũ: co-host nhận GEM, rate chung cho mọi host, chưa có level, chưa chặn user đang nợ.

**Phạm vi**

| Lớp | Thay đổi |
|---|---|
| DB | S1 rate theo host · S2 snapshot rate + `target_user_id` · S3 `exp` + bảng ngưỡng level · S4 `gifts.min_level` · S5 `RESTRICTED`, `coin_debt`, ví `CHARGEBACK_LOSS` |
| Code | `SendGift`: khóa `users` trước (thứ tự khóa chung) → kiểm tra `RESTRICTED`, `min_level` → chỉ host nhận GEM → rate của host → cộng EXP/level. API danh mục quà `GET /gifts` |
| Dev | Script sinh bảng ngưỡng level (seed) |

**Cần hiểu:** thứ tự khóa users → rooms → wallets và vì sao vi phạm sẽ deadlock; snapshot (lưu rate vào biên lai) để lịch sử không đổi khi cấu hình đổi; receiver khác target.

**Xong khi:**
- [ ] Co-host không nhận GEM; buff cho người chơi thì GEM vẫn về host
- [ ] Host rate 60% nhận đúng 60%; `gift_logs` lưu đúng rate đã áp dụng
- [ ] User chưa đủ level không tặng được quà độc quyền
- [ ] User `RESTRICTED` không tặng được
- [ ] EXP/level tăng đúng, lên level đúng mốc
- [ ] Test đồng thời: cùng user vừa tặng quà vừa bị đổi trạng thái → không deadlock

---

## Phase 3 — Nạp tiền (xong MVP-2)

**Vấn đề nghiệp vụ:** user chưa có cách nào có coin, ngoài SQL tay.

**Phạm vi**

| Lớp | Thay đổi |
|---|---|
| DB | S6 `coin_packages` (theo kênh WEB/IOS/ANDROID), `topup_orders` |
| Code | Tạo đơn → webhook cổng thanh toán (sandbox) có xác minh chữ ký → `TOPUP` + trả nợ chargeback trong cùng transaction. Webhook chargeback → luồng `CHARGEBACK` |
| API | `GET /coin-packages`, `POST /topup/orders`, `POST /webhooks/payment/:provider` |

**Cần hiểu:** vì sao không bao giờ cộng tiền theo callback từ client; `provider_txn_id` unique để webhook gọi lặp không cộng hai lần; luồng nợ qua `CHARGEBACK_LOSS`.

**Xong khi:**
- [ ] Nạp sandbox → coin vào ví; gọi lại webhook → không cộng thêm
- [ ] Chargeback khi user đã tiêu hết → nợ ghi đúng, user `RESTRICTED`; nạp tiếp → trả nợ trước, về `ACTIVE`
- [ ] Đối soát: `−CHARGEBACK_LOSS = SUM(coin_debt)`; tổng số dư = 0

---

## Phase 4 — Realtime hoàn chỉnh

**Vấn đề nghiệp vụ:** host chưa kick được ai; người xem rớt mạng mất tin; app chưa biết khi có quà, khi phòng kết thúc.

**Phạm vi**

| Lớp | Thay đổi |
|---|---|
| Code | API kick (3 bước: trạng thái + `RemoveParticipant` + đóng WebSocket); `GET /rooms/:id/messages?after=`; phát sự kiện `gift.sent`, `room.ended`, `member.kicked`; token LiveKit hạn ~10 phút; close frame khi tắt server |
| Thư viện | Thêm `livekit/server-sdk-go` |
| Cân nhắc | ws-ticket thay token trên URL; hub WebSocket (mỗi server subscribe 1 lần/phòng) |

**Cần hiểu:** ba kênh (REST, WebSocket, WebRTC) có ba cơ chế thu hồi quyền khác nhau; Redis Pub/Sub không lưu tin; "bị bắt chờ" khác "được kiểm tra lại".

**Xong khi:**
- [ ] Kick → người đó mất video, mất chat ngay, không vào lại được
- [ ] Rớt mạng 10 giây → kết nối lại và lấy đủ tin bị lỡ
- [ ] Tặng quà → mọi người trong phòng nhận `gift.sent`

---

## Phase 5 — PK (xong MVP-3)

**Vấn đề nghiệp vụ:** PK mới có ở tầng service: không mời được, không tự hết giờ, host tắt live lại được xử thắng.

**Phạm vi**

| Lớp | Thay đổi |
|---|---|
| DB | S8 `PUNISHMENT`, `ends_at`, `end_reason` + CHECK khớp kết quả, `disconnected_at` · S9 `host_pk_stats` |
| Code | Mời/chấp nhận/hủy; `finishBattleTx` nhận "phe bị xử thua"; `EndRoom` xử thua TKO; phase phạt; chuỗi thắng |
| Job | `pk_settlement` (theo `ends_at`), hết hạn lời mời, đếm 30 giây rời trận; khóa Redis để chỉ một server chạy |
| Realtime | `pk.started`, `pk.score`, `pk.punishment`, `pk.ended` |
| Thư viện | Webhook LiveKit (`participant_left`) |

**Cần hiểu:** máy trạng thái của trận; FOR SHARE / FOR UPDATE khi chốt trận; vì sao Postgres là trọng tài, Redis chỉ để hiển thị; job chạy nhiều server.

**Xong khi:**
- [ ] Hai host đấu 300 giây, có người thắng, phạt 60 giây, chuỗi thắng cập nhật
- [ ] Host tắt live khi đang dẫn → xử thua
- [ ] Restart server giữa trận → trận vẫn kết thúc đúng giờ
- [ ] Quà trong lúc phạt không cộng điểm nhưng vẫn trả GEM

---

## Phase 6 — Mini game an toàn (xong MVP-4)

**Vấn đề nghiệp vụ:** thưởng game không giới hạn, một secret dùng chung, ai xin cũng được ghế, người chơi chưa nói được.

**Phạm vi**

| Lớp | Thay đổi |
|---|---|
| DB | S13 secret riêng từng game, bảng luật thưởng, trần thưởng |
| Code | `SubmitResult` tự tính thưởng theo bảng, áp trần, vượt trần → `PENDING`; host duyệt ghế; game token cho iframe; quyền chỉ mic qua `UpdateParticipant` |

**Cần hiểu:** ranh giới tin cậy; chữ ký chỉ trả lời "tin từ ai", không trả lời "số tiền có hợp lý không"; nhiều lớp phòng thủ.

**Xong khi:**
- [ ] Provider gửi thưởng vượt trần → `PENDING`, không trả tiền
- [ ] Secret của game A không ký được kết quả cho game B
- [ ] Hết ngân sách thưởng → DB chặn

**Chờ chốt:** con số trần thưởng, ngân sách tháng.

---

## Phase 7 — Rút tiền (giai đoạn 2)

**Phạm vi:** S10 (VNĐ gross/thuế/net, snapshot tỷ giá); `CASHOUT_HOLD` / `PAYOUT` / `RELEASE`; KYC host; giới hạn 1 lần/tuần; CMS admin duyệt.

**Chờ chốt:** kế toán xác nhận mức thuế TNCN.

---

## Việc xuyên suốt (làm dần ở mọi phase)

- **Test tích hợp** với Postgres thật cho mọi luồng tiền, gồm test đồng thời.
- `docker-compose.yml` cho dev: Postgres, Redis, LiveKit.
- `GET /healthz`, `GET /readyz`; log có cấu trúc (`slog`), gồm cả lỗi 4xx của luồng tiền.
- Job đối soát sổ cái hằng ngày.
