# TamiLive

Nền tảng livestream: phát và xem live, chat realtime, quà tặng, PK, mini game.

| Thư mục | Nội dung |
|---|---|
| [`livestream-backend/`](livestream-backend/) | API Go, Postgres, Redis; môi trường dev bằng docker-compose (cả LiveKit) |
| [`livestream-frontend/`](livestream-frontend/) | Web app Next.js |
| [`docs/`](docs/) | Tài liệu sản phẩm: [nghiệp vụ](docs/BUSINESS.md) · [database](docs/DATABASE.md) · [kiến trúc](docs/TECHSTACK.md) · [lộ trình](docs/ROADMAP.md) |

Hợp đồng API giữa hai bên: [`livestream-backend/api/openapi.yaml`](livestream-backend/api/openapi.yaml). Frontend sinh kiểu TypeScript trực tiếp từ file này.

## Chạy toàn bộ ở máy dev

```bash
# 1. Backend + Postgres + Redis + LiveKit
cd livestream-backend
cp .env.example .env          # điền JWT_SECRET, MINIGAME_HMAC_SECRET
make up && make migrate
make run                      # http://localhost:8080 (tài liệu API: /docs)

# 2. Frontend (terminal khác)
cd livestream-frontend
cp .env.example .env.local
npm install
npm run dev                   # http://localhost:3000
```

Chi tiết từng phần xem README trong thư mục tương ứng.
