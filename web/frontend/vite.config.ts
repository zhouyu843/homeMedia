/// <reference types="vitest/config" />

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig(({ mode }) => ({
  define: {
    "process.env.NODE_ENV": JSON.stringify(mode === "test" ? "test" : "production"),
    "process.env": JSON.stringify({ NODE_ENV: mode === "test" ? "test" : "production" }),
    process: JSON.stringify({ env: { NODE_ENV: mode === "test" ? "test" : "production" } })
  },
  plugins: [react()],
  base: "/static/app/",
  build: {
    outDir: "../static/app",
    emptyOutDir: true,
    sourcemap: true
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"]
  }
}));
