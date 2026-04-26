import { expect, test } from "@playwright/test";

test("删除媒体后主列表不可见，回收站可见，恢复后重回主列表", async ({ page }) => {
  await page.goto("/media");

  // 等待至少一个卡片
  const firstCard = page.locator(".media-card").first();
  await expect(firstCard).toBeVisible({ timeout: 10_000 });

  // 记录卡片内文件名（通过 img alt）
  const thumbImg = firstCard.locator("img.card-thumb");
  const filename = await thumbImg.getAttribute("alt");
  expect(filename).toBeTruthy();

  // 设置 window.confirm 自动确认
  page.on("dialog", (dialog) => void dialog.accept());

  // 点击删除按钮
  const deleteBtn = firstCard.locator('button[aria-label="移入回收站"]');
  await deleteBtn.click();

  // 主列表中该文件消失（或列表为空）
  await expect(page.locator(".media-card").filter({ has: page.locator(`img[alt="${filename}"]`) })).toHaveCount(0, {
    timeout: 8_000,
  });

  // 去回收站
  await page.getByRole("link", { name: "回收站" }).click();
  await expect(page).toHaveURL(/\/trash/);

  // 等待回收站加载
  await expect(page.locator(".empty-state").filter({ hasText: "正在加载" })).toHaveCount(0, { timeout: 8_000 });

  // 文件出现在回收站
  const trashItem = page.locator(".trash-item").filter({ has: page.locator(`h2:text("${filename}")`) });
  await expect(trashItem).toBeVisible({ timeout: 5_000 });

  // 恢复
  const restoreBtn = trashItem.locator("button", { hasText: "恢复" });
  await restoreBtn.click();

  // 回收站中消失
  await expect(trashItem).toHaveCount(0, { timeout: 8_000 });

  // 返回媒体库，文件重新出现
  await page.getByRole("link", { name: "媒体库" }).click();
  await expect(page).toHaveURL(/\/media/);
  await expect(page.locator(`img[alt="${filename}"]`)).toBeVisible({ timeout: 8_000 });
});

test("彻底删除回收站条目", async ({ page }) => {
  await page.goto("/media");

  const firstCard = page.locator(".media-card").first();
  await expect(firstCard).toBeVisible({ timeout: 10_000 });

  const thumbImg = firstCard.locator("img.card-thumb");
  const filename = await thumbImg.getAttribute("alt");
  expect(filename).toBeTruthy();

  page.on("dialog", (dialog) => void dialog.accept());
  await firstCard.locator('button[aria-label="移入回收站"]').click();

  // 进入回收站
  await page.getByRole("link", { name: "回收站" }).click();
  await expect(page.locator(".empty-state").filter({ hasText: "正在加载" })).toHaveCount(0, { timeout: 8_000 });

  const trashItem = page.locator(".trash-item").filter({ has: page.locator(`h2:text("${filename}")`) });
  await expect(trashItem).toBeVisible({ timeout: 5_000 });

  // 彻底删除
  const permDeleteBtn = trashItem.locator("button.danger", { hasText: "彻底删除" });
  await permDeleteBtn.click();

  // 回收站中消失
  await expect(trashItem).toHaveCount(0, { timeout: 8_000 });
});
