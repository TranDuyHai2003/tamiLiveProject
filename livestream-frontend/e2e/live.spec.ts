import { expect, test, type Browser, type Page } from "@playwright/test";

// MVP-1 (docs/ROADMAP.md bước 3): một người phát, một người xem và chat với nhau.

async function newUser(browser: Browser, name: string): Promise<Page> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto("/register");
  await page.getByLabel("Tên đăng nhập").fill(name);
  await page.getByLabel("Email").fill(`${name}@e2e.test`);
  await page.getByLabel("Mật khẩu").fill("password123");
  await page.getByRole("button", { name: "Tạo tài khoản" }).click();
  await expect(page.getByTestId("current-user")).toHaveText(name);
  return page;
}

// Video thật sự đang nhận khung hình (không chỉ là thẻ <video> rỗng)
async function expectVideoPlaying(page: Page) {
  const video = page.getByTestId("stage-video");
  await expect(video).toBeVisible({ timeout: 30_000 });
  await expect
    .poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState >= 2 && v.videoWidth > 0), { timeout: 30_000 })
    .toBe(true);
}

async function chat(page: Page, text: string) {
  await expect(page.getByTestId("chat-status")).toHaveText("đã kết nối", { timeout: 15_000 });
  await page.getByTestId("chat-input").fill(text);
  await page.getByTestId("chat-input").press("Enter");
}

test("host goes live, viewer watches and they chat", async ({ browser }) => {
  const run = Date.now().toString(36);
  const title = `E2E live ${run}`;
  const host = await newUser(browser, `host_${run}`);
  const viewer = await newUser(browser, `viewer_${run}`);

  // Host lên sóng → vào trang phòng với vai trò host, thấy camera của mình
  await host.goto("/live");
  await host.getByLabel("Tiêu đề").fill(title);
  await host.getByRole("button", { name: "Bắt đầu phát" }).click();
  await host.waitForURL(/\/rooms\/[0-9a-f-]+$/);
  const roomId = host.url().split("/").pop()!;
  await expect(host.getByTestId("end-live")).toBeVisible();
  await expectVideoPlaying(host);

  // Viewer thấy phòng trên trang chủ, vào xem và NHẬN được video của host qua LiveKit
  await viewer.goto("/");
  const card = viewer.locator(`[data-room-id="${roomId}"]`);
  await expect(card).toContainText(title);
  await card.click();
  await expect(viewer.getByTestId("leave-room")).toBeVisible();
  await expectVideoPlaying(viewer);

  // Chat hai chiều qua WebSocket + Redis
  await chat(viewer, "xin chào host");
  await expect(host.getByTestId("chat-messages")).toContainText("xin chào host");
  await chat(host, "chào bạn nhé");
  await expect(viewer.getByTestId("chat-messages")).toContainText("chào bạn nhé");

  // Host kết thúc live → phòng biến khỏi danh sách
  await host.getByTestId("end-live").click();
  await host.waitForURL("/");
  await viewer.goto("/");
  await expect(viewer.getByRole("heading", { name: "Đang live" })).toBeVisible();
  await expect(viewer.locator(`[data-room-id="${roomId}"]`)).toHaveCount(0);
});

test("form errors are shown per field from the API error format", async ({ page }) => {
  await page.goto("/register");
  await page.getByLabel("Tên đăng nhập").fill("x!");
  await page.getByLabel("Email").fill("not-an-email");
  await page.getByLabel("Mật khẩu").fill("password123");
  await page.getByRole("button", { name: "Tạo tài khoản" }).click();
  // 422 VALIDATION_ERROR với fields=[{field: "email", rule: "email"}] → hiện dưới ô Email
  await expect(page.getByText("Email không hợp lệ.")).toBeVisible();
});

test("going live requires login", async ({ page }) => {
  await page.goto("/live");
  await page.waitForURL(/\/login\?next=%2Flive/);
});
