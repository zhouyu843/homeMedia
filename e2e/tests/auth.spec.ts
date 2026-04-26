import { expect, test } from "@playwright/test";

import { logoutViaAPI } from "../helpers/auth";

// 这两个 case 需要在未登录状态下运行，覆盖重定向逻辑
test("未登录访问 /media 重定向到 /login", async ({ page, context }) => {
  await context.clearCookies();
  await page.goto("/media");
  await expect(page).toHaveURL(/\/login/);
});

test("未登录访问 /trash 重定向到 /login", async ({ page, context }) => {
  await context.clearCookies();
  await page.goto("/trash");
  await expect(page).toHaveURL(/\/login/);
});

// 以下 case 依赖 global setup 的 storageState（已登录）
test("已登录访问 /media 停留在媒体库", async ({ page }) => {
  await page.goto("/media");
  await expect(page).toHaveURL(/\/media/);
  await expect(page.locator("h1")).toContainText("媒体库");
});

test("登出后访问 /media 重定向到 /login", async ({ page }) => {
  // 先确认登录态正常
  await page.goto("/media");
  await expect(page).toHaveURL(/\/media/);

  // API 登出
  await logoutViaAPI(page);

  // 重新导航，应被重定向
  await page.goto("/media");
  await expect(page).toHaveURL(/\/login/);
});

test("登录页表单正常渲染", async ({ page, context }) => {
  await context.clearCookies();
  await page.goto("/login");
  await expect(page.locator("input[autocomplete='username']")).toBeVisible();
  await expect(page.locator("input[autocomplete='current-password']")).toBeVisible();
  await expect(page.locator("button[type='submit']")).toBeVisible();
});
