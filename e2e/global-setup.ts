import * as fs from "fs";
import * as path from "path";

import { chromium, type FullConfig } from "@playwright/test";

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8080";
const ADMIN_USERNAME = process.env.ADMIN_USERNAME ?? "admin";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";

export const STORAGE_STATE_PATH = path.join(__dirname, ".auth/storage-state.json");

/**
 * 全局 setup：登录一次，把 cookie 持久化到 .auth/storage-state.json。
 * 所有测试通过 storageState 复用，避免重复调登录接口触发限流。
 */
export default async function globalSetup(_config: FullConfig) {
  fs.mkdirSync(path.dirname(STORAGE_STATE_PATH), { recursive: true });

  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();

  // 取登录 CSRF token
  const statusRes = await page.request.get(`${BASE_URL}/api/auth/status`);
  const status = await statusRes.json() as { csrfToken?: string };
  const csrfToken = status.csrfToken ?? "";

  // 登录（CSRF token 在 body 里，与 X-CSRF-Token header 无关）
  const loginRes = await page.request.post(`${BASE_URL}/api/login`, {
    headers: { "Content-Type": "application/json" },
    data: JSON.stringify({ username: ADMIN_USERNAME, password: ADMIN_PASSWORD, csrfToken }),
  });

  // 先读 body，再判断状态（读过一次后 response 会被 dispose）
  const loginBody = await loginRes.text();
  if (!loginRes.ok()) {
    await browser.close();
    throw new Error(`globalSetup login failed: ${loginRes.status()} ${loginBody}`);
  }

  // 保存 cookie / storage state
  await context.storageState({ path: STORAGE_STATE_PATH });
  await browser.close();
}
