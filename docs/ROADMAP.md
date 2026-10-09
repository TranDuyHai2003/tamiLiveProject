# Lộ trình xây dựng TamiLive — từ nền móng tới production

> Một nguồn duy nhất cho câu hỏi "đang làm gì, làm tiếp gì, vì sao, xong khi nào".
> Luật nghiệp vụ xem [BUSINESS.md](BUSINESS.md), schema xem [DATABASE.md](DATABASE.md) (mã S1–S13), kiến trúc xem [TECHSTACK.md](TECHSTACK.md).

## Phần A — Cách một senior fullstack sắp thứ tự công việc

Một dev nhiều năm kinh nghiệm không làm "hết backend rồi mới làm frontend", cũng không làm "hết tính năng rồi mới deploy". Họ đi theo 8 nguyên tắc:

| # | Nguyên tắc | Nghĩa là | Nếu bỏ qua |
|---|---|---|---|
| 1 | **Chạy được trước, đầy đủ sau** (walking skeleton) | Dựng sớm một lát mỏng xuyên qua **mọi tầng**: giao diện → API → DB → video → deploy. Sau đó mới đắp thêm tính năng | Mỗi tầng tự đúng, ghép lại mới lộ lỗi, lúc đó sửa rất đắt |
| 2 | **Chia theo lát dọc giá trị người dùng** | Mỗi bước xong là người dùng **làm được một việc mới** (xem live, tặng quà, đấu PK), gồm cả backend lẫn giao diện | Làm hàng tháng không có gì để thử thật |
| 3 | **Rủi ro lớn nhất làm trước** | Thứ chưa chắc chạy được (video WebRTC qua mạng thật) và thứ sai là mất tiền (sổ cái) phải được chứng minh sớm nhất | Phát hiện "video không chạy trên 4G" khi đã làm xong mọi thứ |
| 4 | **Lưới an toàn trước khi tăng tốc** | Có test tự động và CI **trước** khi sửa các luồng tiền | Mỗi lần sửa phải thử tay lại mọi thứ, hoặc không ai dám sửa |
| 5 | **Hợp đồng trước, cài đặt sau** | Chốt định dạng API (đường dẫn, request, response, mã lỗi) **trước** khi frontend dùng | Frontend và backend hiểu khác nhau, sửa qua sửa lại |
| 6 | **Deploy sớm, deploy thường xuyên** | Có môi trường staging ngay sau khi có bộ khung; từ đó mỗi bước xong đều lên staging | Lần deploy đầu tiên là lần đau nhất, và rơi đúng lúc sắp ra mắt |
| 7 | **Mỗi bước có "Xong khi" đo được** | Kết thúc bằng danh sách kiểm chứng cụ thể, ở trạng thái build được, chạy được, đã commit | "Gần xong" kéo dài mãi |
| 8 | **Không tối ưu sớm** | Hub WebSocket, chia nhỏ ví hệ thống… chỉ làm khi **đo** thấy cần | Tốn công cho vấn đề chưa tồn tại |

**Dự án này đã đi khác thứ tự đó ở đâu:** ta làm sâu phần tiền (bước 0) trước khi có bộ khung chạy được. Điều này chấp nhận được, vì tiền là rủi ro "sai là mất tiền" lớn nhất. Nhưng từ đây phải quay về đúng nguyên tắc: **dựng lưới an toàn (bước 1) → bộ khung chạy thật (bước 2–4) → rồi mới đắp tính năng**.

## Phần B — Cách làm mỗi bước

| Giai đoạn | Việc | Ai |
|---|---|---|
| 1. Giải thích | Bước này giải quyết vấn đề gì, làm những gì, vì sao | Claude |
| 2. Chốt | Các quyết định còn mở | Bạn |
| 3. Làm | Code, migration, cấu hình | Claude |
| 4. Kiểm chứng | Chạy đúng các tiêu chí "Xong khi" | Claude, báo kết quả |
| 5. Tổng kết và commit | Đã đổi gì, luật nào giờ được bảo vệ ở đâu | Claude giải thích, bạn duyệt |

## Phần C — Tổng quan các bước

| Bước | Tên | Sau bước này, người dùng / đội làm được gì | Trạng thái |
|---|---|---|---|
| 0 | Nền móng backend + luật dữ liệu tiền | (Chỉ backend) Đăng ký, phòng, chat, tặng quà; DB chặn mọi dữ liệu tiền sai | ✅ Xong |
| 1 | Môi trường dev và lưới an toàn | Một lệnh dựng đủ môi trường; test tự động bảo vệ luồng tiền; CI chạy mỗi lần push | ✅ Xong (chờ CI chạy lần đầu trên GitHub) |
| **2** | **Hợp đồng API** | Frontend biết chính xác API trả gì, lỗi trông thế nào | 🔜 **Tiếp theo** |
| 3 | Bộ khung chạy thật (MVP-1) | **Trình duyệt:** đăng nhập, xem danh sách phòng, host phát camera, viewer xem và chat | Chờ |
| 4 | Lên staging | Thử trên điện thoại, qua mạng thật, có HTTPS | Chờ |
| 5 | Tặng quà theo luật đã chốt | Tặng quà có hiệu ứng, rate theo host, level, quà độc quyền | Chờ |
| 6 | Nạp tiền (xong MVP-2) | Nạp tiền sandbox, xem số dư; chargeback | Chờ |
| 7 | Vận hành phòng | Host kick người; rớt mạng tự kết nối lại không mất tin | Chờ |
| 8 | Auth hoàn chỉnh, chống lạm dụng | Đăng nhập lâu dài an toàn, đăng xuất, giới hạn spam | Chờ |
| 9 | PK (xong MVP-3) | Hai host mời nhau đấu, điểm chạy trực tiếp, có phạt | Chờ |
| 10 | Mini game (xong MVP-4) | Chơi game trong phòng, lên ghế, nói qua mic, nhận thưởng | Chờ |
| 11 | Sẵn sàng production | Giám sát, sao lưu, chịu tải, rà soát bảo mật | Chờ |
| 12 | Rút tiền và CMS admin | Host rút GEM ra tiền; admin quản lý quà, duyệt rút | Chờ |

**Logic thứ tự:** lưới an toàn (1) → hợp đồng (2) → bộ khung chạy thật trên mọi tầng (3) → môi trường thật (4) → đắp tính năng theo giá trị: tiền (5, 6) → vận hành (7, 8) → tương tác (9, 10) → sẵn sàng ra mắt (11) → chi trả (12).

---

## Bước 0 — Nền móng backend + luật dữ liệu tiền ✅

*(Trước đây gọi là phase 0 và phase 1.)*

**Đã có:**
- Kiến trúc 4 lớp; auth JWT + bcrypt; ví tạo cùng lúc với tài khoản.
- Sổ cái kế toán kép (`postLedger`); tặng quà có idempotency, khóa theo thứ tự, cộng điểm PK trong cùng transaction.
- Phòng LIVE, token LiveKit do server tính quyền; chat WebSocket qua Redis; PK và mini game ở tầng service.
- **Luật dữ liệu tiền ở DB** (S7, S11, S12):
  - Loại giao dịch là sự kiện nghiệp vụ; hoàn tiền chỉ một lần.
  - Không xóa được biên lai.
  - Ví quỹ và ví user không được âm (cột `allow_negative`); quỹ thưởng nạp từ `PROMO_BUDGET`.

**Đã kiểm chứng:**
- Migration chạy lên/xuống sạch.
- DB chặn xóa biên lai, chặn hoàn tiền hai lần, chặn ví user âm.
- Thưởng game khi hết quỹ trả 402.
- 30 request tặng quà song song; tổng số dư bằng 0; mọi ví khớp sổ cái.

---

## Bước 1 — Môi trường dev và lưới an toàn ✅

**Để làm gì:** bất kỳ ai (kể cả bạn sau 3 tháng) clone repo về chạy được bằng **một lệnh**, và mọi luồng tiền có **test tự động** báo lỗi ngay khi ai đó làm hỏng.

**Vì sao làm lúc này:**
- Các bước 5, 6, 9 đều sửa sâu vào luồng tiền. Không có test, mỗi lần sửa phải thử tay lại mọi thứ như ta đã làm ở bước 0 (khởi động container, chạy script, đọc kết quả).
- Nguyên tắc 4: lưới an toàn trước khi tăng tốc.
- Bước 3 cần LiveKit chạy ở máy dev, mà hiện chưa có.

**Làm theo thứ tự:**
1. `docker-compose.yml`: Postgres 16, Redis 7, **LiveKit server ở chế độ dev**. Tách cổng khỏi các container của dự án khác trên máy.
2. `.env.example` liệt kê đủ biến môi trường kèm giải thích; `.env` cho dev trỏ vào docker-compose.
3. `Makefile`: `make up` (dựng môi trường), `make migrate`, `make reset-db`, `make sqlc`, `make run`, `make test`.
4. **Test tích hợp với Postgres thật** (không giả lập DB cho phần tiền):
   - `postLedger`: cân bằng, không đủ tiền, thứ tự khóa.
   - `SendGift`: thành công, replay idempotency, khác body, thiếu tiền, tự tặng, phòng không LIVE, cộng điểm PK.
   - Test đồng thời: nhiều goroutine tặng quà từ cùng một ví, số dư cuối đúng, không âm, sổ cái cân.
   - Bất biến sau mỗi test: tổng số dư = 0, mọi ví khớp sổ cái.
5. CI (GitHub Actions): `go build`, `go vet`, test, kiểm tra `sqlc generate` không tạo khác biệt, migration lên/xuống/lên. Cần đẩy repo lên GitHub.

**Xong khi:**
- [x] Máy trống chỉ cần `make up && make migrate && make run` là server chạy (Postgres :5440, Redis :6390, LiveKit :7980–7982)
- [x] `make test` chạy 22 test tiền, tất cả qua (với `-race`)
- [x] Cố tình phá code thì test báo đỏ: bỏ sắp xếp ví → deadlock 40P01; bỏ idempotency; câu SQL trừ tiền quên kiểm tra số dư (lỗi thành 500 thay vì 402); quên ghi bút toán → bất biến sổ cái báo lệch
- [x] Rào chắn: thiếu `TEST_DATABASE_URL` hoặc trỏ vào DB không có đuôi `_test` thì từ chối chạy
- [ ] CI chạy xanh trên GitHub *(mọi bước CI đã chạy thử trên máy; chờ push để chạy lần đầu)*

**Đã chốt khi làm:** compose riêng với cổng không đụng dự án khác; LiveKit dùng 1 cổng UDP cố định và khai báo `node_ip`; test commit thật rồi xóa dữ liệu (không rollback, vì trigger cân sổ là DEFERRED); `go test -p 1 -race`; ghim sqlc v1.31.1, golang-migrate v4.20.1, Postgres 16, LiveKit v1.13.9.

**Học được gì:** vì sao test tiền phải dùng DB thật; viết test đồng thời; CI là gì và chặn được gì.

---

## Bước 2 — Hợp đồng API

**Để làm gì:** frontend và backend thống nhất **một bản mô tả API duy nhất**: đường dẫn, request, response, mã lỗi. Frontend code theo bản đó, không phải đoán.

**Vì sao làm lúc này:** ngay trước khi frontend bắt đầu dùng API (nguyên tắc 5). Đổi định dạng lỗi lúc chưa có client nào thì gần như miễn phí; đổi sau khi đã có app thì phải sửa cả hai bên.

**Làm theo thứ tự:**
1. Chuẩn hóa định dạng lỗi theo TECHSTACK: `{"error": {"code": "INSUFFICIENT_BALANCE", "message": "..."}}`, với danh sách `code` cố định để frontend xử lý theo mã, không theo câu chữ.
2. Viết `openapi.yaml` cho các endpoint hiện có. Đây là "hợp đồng"; xem được bằng Swagger UI.
3. Quy ước chung: phân trang (`?limit=&cursor=`), định dạng thời gian (ISO 8601, UTC), tiền luôn là số nguyên.

**Xong khi:**
- [ ] Mọi lỗi trả đúng định dạng có `code`
- [ ] `openapi.yaml` mô tả đủ endpoint, mở được bằng Swagger UI
- [ ] Test của bước 1 kiểm tra cả `code` lỗi

**Học được gì:** API là hợp đồng giữa hai đội; vì sao lỗi cần mã máy đọc được; phân trang bằng cursor.

---

## Bước 3 — Bộ khung chạy thật (MVP-1)

**Để làm gì:** lần đầu tiên thấy sản phẩm chạy trên trình duyệt: **một người phát, một người xem và chat**.

**Vì sao làm lúc này:** đây là **rủi ro lớn nhất còn lại** (nguyên tắc 3). Video WebRTC chưa từng được chạy thử. Bộ khung đi xuyên mọi tầng nên sẽ lộ ra mọi chỗ thiếu khi ghép.

**Làm theo thứ tự:**

*Backend (vá chỗ thiếu mà frontend cần):*
1. CORS cho origin của frontend.
2. `GET /rooms?status=LIVE` có phân trang.
3. Trả `livekit_url` cùng token khi vào phòng.
4. `GET /healthz`, `GET /readyz`.

*Frontend (repo `livestream-frontend`):*

5. Dựng khung: Next.js (App Router) + TypeScript + Tailwind + Zustand; cấu trúc thư mục; một API client dùng chung, sinh kiểu từ `openapi.yaml`.
6. Đăng ký / đăng nhập. Tạm lưu token ở bộ nhớ + `localStorage` (**nợ kỹ thuật có ghi chép**, trả ở bước 8).
7. Trang chủ: danh sách phòng đang live.
8. Host: nút "Lên sóng" → tạo phòng → phát camera/mic bằng LiveKit React SDK.
9. Viewer: vào phòng → xem video.
10. Khung chat qua WebSocket: gửi, nhận, tự cuộn.
11. Host bấm "Kết thúc live".

**Xong khi:**
- [ ] Hai trình duyệt: A đăng nhập và lên sóng, B thấy phòng của A trong danh sách, vào xem có hình và tiếng
- [ ] A và B chat qua lại
- [ ] A kết thúc live thì phòng biến mất khỏi danh sách
- [ ] Viewer không phát được hình (thử sửa client cũng không được)

**Học được gì:** WebRTC/SFU hoạt động thế nào; CORS; quản lý state realtime ở frontend; kết nối REST, WebSocket và LiveKit trong một màn hình.

**Chờ chốt:** giữ stack Next.js + Zustand + Tailwind; frontend ở repo riêng `livestream-frontend` (đề xuất).

---

## Bước 4 — Lên staging

**Để làm gì:** có một địa chỉ thật (HTTPS) để thử trên **điện thoại, qua 4G, sau NAT**, những điều kiện mà máy dev không mô phỏng được.

**Vì sao làm lúc này:**
- Ngay sau khi có bộ khung (nguyên tắc 6).
- WebRTC trên trình duyệt **bắt buộc HTTPS** (trừ localhost), và gặp vấn đề NAT/firewall chỉ khi chạy mạng thật.
- Deploy khi hệ thống còn nhỏ là dễ nhất; từ đây mỗi bước xong đều lên staging.

**Làm theo thứ tự:**
1. `Dockerfile` cho backend (build nhiều tầng, image nhỏ).
2. Chọn hạ tầng: một VPS chạy docker-compose (rẻ, dễ hiểu) hoặc dịch vụ quản lý sẵn. LiveKit tự chạy (cần TURN) hoặc LiveKit Cloud.
3. Domain + HTTPS (Caddy hoặc Nginx + Let's Encrypt).
4. Chạy migration tự động khi deploy; secret nằm trong biến môi trường của server.
5. CI deploy tự động khi merge vào nhánh chính.
6. Log ra định dạng JSON (`slog`) để đọc được trên server.

**Xong khi:**
- [ ] Mở `https://staging...` trên điện thoại qua 4G, xem được live do máy tính phát
- [ ] Push code lên nhánh chính thì staging tự cập nhật
- [ ] Xem được log lỗi của staging

**Học được gì:** Docker build nhiều tầng; reverse proxy, TLS; TURN/STUN và vì sao WebRTC cần chúng; quy trình deploy tự động.

**Từ thời điểm này, quy tắc "chỉ thêm migration mới, không sửa migration cũ" có hiệu lực**, vì staging là môi trường dùng chung.

---

## Bước 5 — Tặng quà theo luật đã chốt

*(Trước đây gọi là phase 2.)*

**Để làm gì:** người xem tặng quà có hiệu ứng trên màn hình; tiền chạy đúng mọi luật kinh doanh đã chốt.

**Vì sao làm lúc này:** tặng quà là nguồn doanh thu; luồng tiền là lõi mà PK (bước 9) xây lên trên.

**Làm theo thứ tự:**

*DB (migration mới, vì đã có staging):*
1. S1 rate theo host; S2 lưu rate và `target_user_id` vào biên lai; S3 `exp` + bảng ngưỡng level (seed); S4 `gifts.min_level`; S5 `RESTRICTED`, `coin_debt`, ví `CHARGEBACK_LOSS`.

*Backend:*

2. `SendGift`: khóa `users` của người gửi trước tiên (thứ tự khóa chung) → chặn `RESTRICTED` → kiểm tra `min_level` → **chỉ host nhận GEM** → rate của host → cộng EXP/level.
3. `GET /gifts` (danh mục quà); `GET /me/wallet`.
4. Phát sự kiện `gift.sent` lên kênh phòng sau khi COMMIT.
5. Test: mọi luật trên + test đồng thời "vừa tặng quà vừa bị đổi trạng thái" không deadlock.

*Frontend:*

6. Hiển thị số dư; bảng chọn quà; nút tặng (sinh idempotency key mỗi lần bấm); hiệu ứng quà khi nhận `gift.sent`; badge level trong chat.

**Xong khi:**
- [ ] Viewer tặng quà, mọi người trong phòng thấy hiệu ứng, số dư hai bên đúng
- [ ] Co-host không nhận GEM; buff cho người chơi thì GEM vẫn về host
- [ ] Host rate 60% nhận đúng 60%; biên lai lưu đúng rate
- [ ] Chưa đủ level không tặng được quà độc quyền; user `RESTRICTED` không tặng được
- [ ] Bấm tặng hai lần do mạng chậm chỉ bị trừ tiền một lần

**Học được gì:** thứ tự khóa và deadlock; snapshot dữ liệu vào biên lai; idempotency nhìn từ phía client.

---

## Bước 6 — Nạp tiền (xong MVP-2)

*(Trước đây gọi là phase 3.)*

**Để làm gì:** user tự nạp coin (sandbox), không còn phải dùng SQL tay.

**Vì sao làm lúc này:** sau khi tiêu tiền đã đúng (bước 5) mới mở cửa cho tiền vào.

**Làm theo thứ tự:**
1. S6: `coin_packages` (theo kênh WEB/IOS/ANDROID), `topup_orders` (khóa ngoại RESTRICT theo luật S11).
2. Chọn một cổng thanh toán VN có sandbox; tạo đơn → trang thanh toán → **webhook có xác minh chữ ký** → `TOPUP`, trừ nợ chargeback nếu có, trong cùng transaction.
3. Webhook chargeback → luồng `CHARGEBACK` (ví `CHARGEBACK_LOSS`, `coin_debt`, `RESTRICTED`).
4. Frontend: trang chọn gói, chuyển sang thanh toán, quay về thấy số dư mới.

**Xong khi:**
- [ ] Nạp sandbox thì coin vào ví; webhook gọi lặp không cộng hai lần
- [ ] Chargeback khi user đã tiêu hết: nợ ghi đúng, user `RESTRICTED`; nạp tiếp thì trả nợ trước rồi về `ACTIVE`
- [ ] Đối soát: `−CHARGEBACK_LOSS = SUM(coin_debt)`; tổng số dư = 0

**Học được gì:** vì sao không bao giờ cộng tiền theo callback từ client; xác minh chữ ký webhook; luồng nợ trong sổ cái.

---

## Bước 7 — Vận hành phòng

*(Trước đây gọi là phase 4.)*

**Để làm gì:** host kiểm soát được phòng; người xem rớt mạng không mất gì.

**Làm theo thứ tự:**
1. Thêm `livekit/server-sdk-go`.
2. API kick đủ 3 việc: đổi trạng thái thành viên + `RemoveParticipant` + sự kiện `member.kicked` đóng WebSocket. Token LiveKit hạn ~10 phút.
3. `GET /rooms/:id/messages?after=`; sự kiện `room.ended`.
4. Gửi close frame cho WebSocket khi tắt server.
5. Frontend: menu kick; tự kết nối lại và lấy tin bị lỡ; màn hình "phòng đã kết thúc".

**Xong khi:**
- [ ] Bị kick thì mất video, mất chat ngay, không vào lại được
- [ ] Tắt wifi 10 giây rồi bật lại: tự kết nối lại, đủ tin
- [ ] Host kết thúc live: mọi viewer thấy thông báo

**Học được gì:** ba kênh, ba cơ chế thu hồi quyền; Redis Pub/Sub không lưu tin; thiết kế kết nối lại.

---

## Bước 8 — Auth hoàn chỉnh, chống lạm dụng

**Để làm gì:** trả nợ kỹ thuật của bước 3 và chặn spam trước khi có người dùng thật.

**Làm theo thứ tự:**
1. Refresh token: chuỗi ngẫu nhiên, lưu **hash** trong `user_sessions`, cookie `HttpOnly; Secure`, xoay vòng khi làm mới; `/auth/refresh`, `/auth/logout`. Access token ngắn (~15 phút), chỉ nằm trong bộ nhớ.
2. ws-ticket: vé dùng một lần để mở WebSocket, token thật không nằm trên URL.
3. Rate limit bằng Redis: đăng nhập, chat, tặng quà.
4. Ban user thì thu hồi toàn bộ session.
5. Luồng xin làm host (role `HOST`).

**Xong khi:**
- [ ] Đóng trình duyệt mở lại vẫn đăng nhập; đăng xuất thì token cũ vô hiệu
- [ ] Không còn token nào trong `localStorage` hay trên URL
- [ ] Gửi 10 tin/giây bị chặn

**Học được gì:** access token và refresh token khác nhau thế nào; XSS và vì sao dùng cookie HttpOnly; rate limit bằng Redis.

---

## Bước 9 — PK (xong MVP-3)

*(Trước đây gọi là phase 5.)*

**Để làm gì:** hai host mời nhau đấu, người xem tặng quà để giúp host mình thắng.

**Làm theo thứ tự:**
1. S8 (`PUNISHMENT`, `ends_at`, `end_reason`, CHECK khớp kết quả, `disconnected_at`), S9 (`host_pk_stats`).
2. API mời / chấp nhận / hủy; `finishBattleTx` nhận "phe bị xử thua"; `EndRoom` xử thua TKO; giai đoạn phạt; chuỗi thắng.
3. Job chạy nền có khóa Redis: chốt trận theo `ends_at`, hết hạn lời mời, đếm 30 giây rời trận.
4. Webhook LiveKit `participant_left`.
5. Sự kiện `pk.started`, `pk.score` (gom 5–10 lần/giây), `pk.punishment`, `pk.ended`.
6. Frontend: màn hình chia đôi, thanh điểm, đếm ngược theo `ends_at`, hiệu ứng phạt cho phe thua.

**Xong khi:**
- [ ] Hai host đấu 300 giây, có người thắng, phạt 60 giây, chuỗi thắng cập nhật
- [ ] Host tắt live khi đang dẫn thì bị xử thua
- [ ] Restart server giữa trận, trận vẫn kết thúc đúng giờ
- [ ] Quà trong lúc phạt không cộng điểm nhưng vẫn trả GEM

**Học được gì:** máy trạng thái; FOR SHARE / FOR UPDATE; job nền chạy trên nhiều server; Postgres là trọng tài, Redis để hiển thị.

---

## Bước 10 — Mini game (xong MVP-4)

*(Trước đây gọi là phase 6.)*

**Để làm gì:** người xem lên ghế chơi game trong phòng, nói chuyện qua mic, người thắng nhận thưởng.

**Làm theo thứ tự:**
1. S13: secret riêng từng game, bảng luật thưởng, trần thưởng.
2. `SubmitResult` tự tính thưởng theo bảng, áp trần, vượt trần thì `PENDING`.
3. Host duyệt ghế; game token cho iframe; quyền chỉ mic qua `UpdateParticipant`.
4. Một game mẫu đơn giản (tự viết) để thử trọn vòng.
5. Frontend: iframe game, danh sách ghế, nút xin lên ghế, mic.

**Xong khi:**
- [ ] Chơi trọn một ván, người thắng nhận COIN
- [ ] Thưởng vượt trần thì `PENDING`, không trả tiền
- [ ] Secret game A không ký được kết quả cho game B; hết ngân sách thì DB chặn

**Học được gì:** ranh giới tin cậy; iframe và `postMessage`; phòng thủ nhiều lớp.

**Chờ chốt:** con số trần thưởng, ngân sách tháng.

---

## Bước 11 — Sẵn sàng production

**Để làm gì:** đủ tự tin để mở cho người dùng thật và tiền thật.

**Làm theo thứ tự:**
1. Giám sát: metrics (Prometheus) cho số kết nối, độ trễ API, giao dịch/giây, lỗi; cảnh báo.
2. Job đối soát sổ cái hằng ngày, cảnh báo khi lệch.
3. Sao lưu Postgres tự động **và thử khôi phục**.
4. Thử tải: nhiều người xem một phòng, nhiều quà mỗi giây. Đo xem có cần hub WebSocket hay chia nhỏ ví hệ thống không (nguyên tắc 8).
5. Rà soát bảo mật: secret, CORS, kiểm tra Origin của WebSocket, quyền admin.
6. Pháp lý: điều khoản sử dụng, chính sách nội dung, chính sách của Apple/Google nếu có app.

**Xong khi:**
- [ ] Có dashboard và cảnh báo
- [ ] Đã khôi phục thành công từ bản sao lưu
- [ ] Có số liệu chịu tải và đã xử lý điểm nghẽn

---

## Bước 12 — Rút tiền và CMS admin

*(Trước đây gọi là phase 7.)*

**Để làm gì:** host đổi GEM ra tiền thật; admin vận hành hằng ngày không cần SQL.

**Làm theo thứ tự:**
1. S10 (VNĐ gộp/thuế/thực nhận, snapshot tỷ giá); KYC host.
2. `CASHOUT_HOLD` / `PAYOUT` / `RELEASE`; giới hạn 1 lần/tuần; ngưỡng thuế cộng dồn theo tháng.
3. CMS admin: quản lý quà và game, duyệt rút tiền, hoàn tiền thủ công, nạp ngân sách thưởng, xử lý vi phạm, ẩn phòng.

**Chờ chốt:** kế toán xác nhận thuế TNCN.

---

## Việc xuyên suốt mọi bước

- Mỗi luồng tiền mới đều có test tích hợp, kể cả test đồng thời.
- Mỗi endpoint mới đều được cập nhật vào `openapi.yaml`.
- Mỗi quyết định nghiệp vụ mới đều ghi vào BUSINESS.md §9.
- Từ bước 4: mỗi bước xong đều chạy được trên staging.
