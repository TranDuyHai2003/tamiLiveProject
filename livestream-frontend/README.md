# TamiLive — web app

Next.js 16 (App Router, Cache Components) + TypeScript + Tailwind 4 + Zustand + LiveKit React SDK.

> Next.js 16 khác nhiều so với bản cũ. Trước khi viết code, đọc tài liệu đi kèm ở
> `node_modules/next/dist/docs/` (xem `AGENTS.md`).

## Chạy

Cần backend đang chạy (xem `../livestream-backend/README.md`: `make up && make migrate && make run`).

```bash
cp .env.example .env.local
npm install
npm run dev            # http://localhost:3000
```

| Lệnh | Việc |
|---|---|
| `npm run gen:api` | Sinh kiểu TypeScript từ `../livestream-backend/api/openapi.yaml` → `src/lib/api/schema.d.ts` |
| `npm run lint` / `npm run typecheck` | ESLint / TypeScript (typecheck tự sinh kiểu route của Next trước) |
| `npm run build` | Build production |
| `npm run e2e` | Test đầu-cuối bằng Playwright (cần `make up && make migrate` ở backend; Playwright tự bật backend + frontend) |

## Cấu trúc

```text
src/app/                 các trang: / (danh sách phòng), /login, /register, /live, /rooms/[id]
src/components/room/     màn hình phòng: kết nối LiveKit, khung video
src/components/chat/     chat WebSocket
src/lib/api/             client gọi API có kiểu (openapi-fetch), thông báo lỗi theo mã
src/lib/auth/            phiên đăng nhập (zustand), chặn trang cần đăng nhập
e2e/                     test Playwright
```

## Quy ước

- **Gọi API chỉ qua `api` trong `src/lib/api/client.ts`**: kiểu sinh từ hợp đồng, backend đổi API là lỗi biên dịch.
- **Xử lý lỗi theo `code`** (`ApiError.code`), hiển thị qua `errorMessage()` / `fieldMessage()`; không so `message` của server.
- **Kết nối dài (WebSocket, LiveKit) mở trong `useEffect` và đóng ở hàm dọn dẹp**: Next.js 16 ẩn route khi chuyển trang thay vì gỡ bỏ, nên kết nối không dọn sẽ bị treo.
- **Không gọi `Date.now()`, `crypto.randomUUID()` lúc render** (Cache Components báo lỗi khi build); gọi trong hàm xử lý sự kiện hoặc effect.
- **Đọc URL động (`params`, `useSearchParams`) trong `<Suspense>`.**

## Nợ kỹ thuật đã biết

- Token lưu trong `localStorage` → bước 8 chuyển sang cookie `HttpOnly` + token trong bộ nhớ.
- WebSocket nhận token qua query string → bước 8 thay bằng vé dùng một lần.
- Chat chưa tự kết nối lại / lấy lại tin bị lỡ → bước 7.
