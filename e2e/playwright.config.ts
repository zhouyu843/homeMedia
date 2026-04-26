import { defineConfig, devices } from "@playwright/test";

import { STORAGE_STATE_PATH } from "./global-setup";

export default defineConfig({
  globalSetup: "./global-setup",
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  retries: 1,
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]],
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:8080",
    storageState: STORAGE_STATE_PATH,
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  outputDir: "test-results",
});
