import * as path from "path";

import { expect, test } from "@playwright/test";

const FIXTURE_IMAGE = path.join(__dirname, "../fixtures/test-image.png");

test("选择文件并上传后出现在媒体列表", async ({ page }) => {
  await page.goto("/media");

  // 等待上传组件渲染
  const fileInput = page.locator("input[type='file']");
  await expect(fileInput).toBeAttached();

  // 通过隐藏的 file input 注入文件
  await fileInput.setInputFiles(FIXTURE_IMAGE);

  // 等待文件进入队列（按钮 enabled）
  const uploadBtn = page.getByRole("button", { name: /上传待处理文件/ });
  await expect(uploadBtn).toBeEnabled({ timeout: 5_000 });
  await uploadBtn.click();

  // 等待上传完成：recent-box 出现表示有结果
  await expect(page.locator(".recent-box")).toBeVisible({ timeout: 15_000 });

  // 媒体列表里出现新卡片
  await expect(page.locator(".media-card").first()).toBeVisible({ timeout: 10_000 });
});

test("上传相同文件返回去重提示（existing）", async ({ page }) => {
  await page.goto("/media");

  const fileInput = page.locator("input[type='file']");
  await expect(fileInput).toBeAttached();

  await fileInput.setInputFiles(FIXTURE_IMAGE);
  const uploadBtn = page.getByRole("button", { name: /上传待处理文件/ });
  await expect(uploadBtn).toBeEnabled({ timeout: 5_000 });
  await uploadBtn.click();

  // 等待上传流程完成：recent-box 出现
  await expect(page.locator(".recent-box")).toBeVisible({ timeout: 15_000 });

  // 最近结果区域应显示「已存在」相关文字
  await expect(page.locator(".recent-box")).toContainText(/已存在|重复/, { timeout: 5_000 });
});
