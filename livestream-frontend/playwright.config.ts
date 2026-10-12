import { defineConfig, devices } from "@playwright/test";

// Test đầu-cuối: trình duyệt thật ↔ Next.js ↔ backend Go ↔ Postgres/Redis/LiveKit.
// Yêu cầu trước khi chạy: môi trường dev đã bật (`make up && make migrate` trong livestream-backend).
// Playwright tự khởi động backend và frontend nếu chưa chạy.
export default defineConfig({
  testDir: "./e2e",
  timeout: 90_000,
  workers: 1, // các test dùng chung DB dev và LiveKit
  reporter: [["list"]],
  use: {
    baseURL: "http://localhost:3000",
    trace: "retain-on-failure",
    ...devices["Desktop Chrome"],
    permissions: ["camera", "microphone"],
    launchOptions: {
      // Camera/micro giả của Chromium: phát hình test pattern + âm thanh beep,
      // không cần thiết bị thật và không hiện hộp thoại xin quyền.
      args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"],
    },
  },
  webServer: [
    {
      command: "bash -c 'set -a && . ./.env && set +a && go run .'",
      cwd: "../livestream-backend",
      url: "http://localhost:8080/readyz",
      reuseExistingServer: true,
      timeout: 120_000,
    },
    {
      command: "npm run build && npm run start",
      url: "http://localhost:3000",
      reuseExistingServer: true,
      timeout: 180_000,
    },
  ],
});
