import { expect, test } from "@playwright/test";

test("媒体列表页加载并显示 gallery", async ({ page }) => {
  await page.goto("/media");

  await expect(page.locator("h1")).toContainText("媒体库");

  // 等待 loading 消失
  await expect(page.locator(".empty-state").filter({ hasText: "正在加载" })).toHaveCount(0, {
    timeout: 10_000,
  });

  // 至少有一个媒体卡片（依赖 upload.spec.ts 已跑且数据库有数据）
  await expect(page.locator(".media-card").first()).toBeVisible({ timeout: 10_000 });
});

test("导航栏显示回收站链接", async ({ page }) => {
  await page.goto("/media");
  await expect(page.getByRole("link", { name: "回收站" })).toBeVisible();
});
