# TamiLive — livestream backend

Backend Go cho nền tảng livestream: tài khoản, phòng live (LiveKit), chat realtime, quà tặng với sổ cái kế toán kép, PK, mini game.

Tài liệu: [nghiệp vụ](docs/BUSINESS.md) · [database](docs/DATABASE.md) · [kiến trúc](docs/TECHSTACK.md) · [lộ trình](docs/ROADMAP.md)

## Yêu cầu

| Công cụ | Phiên bản | Cài |
|---|---|---|
| Docker + Compose | Compose v2+ (`docker compose` hoặc `docker-compose`) | |
| Go | theo `go.mod` | |
| sqlc | **v1.31.1** (CI dùng đúng bản này) | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1` |
| golang-migrate | **v4.20.1** | `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1` |

## Chạy lần đầu

```bash
cp .env.example .env            # rồi điền JWT_SECRET và MINIGAME_HMAC_SECRET (openssl rand -hex 32)
make up                         # Postgres :5440, Redis :6390, LiveKit :7980
make migrate                    # tạo schema cho DB dev
make run                        # API ở http://localhost:8080
```

`make help` liệt kê mọi lệnh.

## Test

```bash
make test
```

- Test chạy trên **Postgres thật** (database `livestream_test`), vì phần lớn luật về tiền nằm trong DB (ràng buộc, trigger cân sổ, khóa dòng). `make test` tạo lại DB test sạch trước mỗi lần chạy.
- Test **commit thật** chứ không rollback, vì trigger cân sổ cái chỉ chạy lúc COMMIT. Dữ liệu được xóa trước mỗi test; sau mỗi test, bất biến sổ cái được kiểm tra tự động (tổng số dư = 0, mỗi ví khớp sổ cái).
- Test từ chối chạy nếu `TEST_DATABASE_URL` trỏ vào DB có tên không kết thúc bằng `_test`.
- Trước khi push, `make check` chạy các kiểm tra giống CI: gofmt, vet, build, code sqlc không lệch.

## Cấu trúc

```text
main.go                    lắp ráp config, DB, Redis, service, router
db/migration/              golang-migrate (nguồn sự thật của schema)
db/query/                  SQL cho sqlc → sinh ra internal/db
db/scripts/                script dev (VD nạp ngân sách thưởng)
docker/                    cấu hình Postgres, LiveKit cho docker-compose
internal/delivery/http/    handler, middleware, WebSocket
internal/service/          luật nghiệp vụ, transaction, sổ cái (+ test tích hợp)
internal/testutil/         kết nối DB test, reset dữ liệu, kiểm tra bất biến sổ cái
```
