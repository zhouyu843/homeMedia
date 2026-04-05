/// <reference types="vitest/config" />

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: "src/upload-island.tsx",
      formats: ["es"],
      fileName: () => "upload-island.js"
    },
    outDir: "../static/react",
    emptyOutDir: true,
    sourcemap: true
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"]
  }
});
