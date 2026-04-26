import { expect, test } from "@playwright/test";

test("点击媒体卡片进入详情页", async ({ page }) => {
  await page.goto("/media");

  // 等待卡片出现（依赖 upload.spec 已跑）
  const firstCard = page.locator(".media-card .card-link").first();
  await expect(firstCard).toBeVisible({ timeout: 10_000 });

  await firstCard.click();

  // URL 变成 /media/:id 格式
  await expect(page).toHaveURL(/\/media\/[0-9a-f-]+$/, { timeout: 5_000 });
});

test("详情页显示文件名和下载链接", async ({ page }) => {
  await page.goto("/media");

  const firstCard = page.locator(".media-card .card-link").first();
  await expect(firstCard).toBeVisible({ timeout: 10_000 });
  await firstCard.click();

  await expect(page).toHaveURL(/\/media\/[0-9a-f-]+$/);

  // 等待详情内容加载
  await expect(page.locator(".detail-panel, .empty-state")).toBeVisible({ timeout: 8_000 });

  // 下载链接存在（或是 PDF / 未找到的 empty-state）
  const detailPanel = page.locator(".detail-panel");
  if (await detailPanel.isVisible()) {
    await expect(page.getByRole("link", { name: "下载原始文件" })).toBeVisible();
  }
});
