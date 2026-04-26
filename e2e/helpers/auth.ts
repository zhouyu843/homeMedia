import { type Page } from "@playwright/test";

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8080";

/**
 * 通过 HTTP API 完成登出。
 * 登录改用 global-setup + storageState，不再每个测试单独登录，避免触发限流。
 */
export async function logoutViaAPI(page: Page): Promise<void> {
  const statusRes = await page.request.get(`${BASE_URL}/api/auth/status`);
  const status = await statusRes.json() as { csrfToken?: string };
  const csrfToken = status.csrfToken ?? "";

  await page.request.post(`${BASE_URL}/api/logout`, {
    headers: { "X-CSRF-Token": csrfToken },
  });
}
