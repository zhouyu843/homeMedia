import { describe, expect, it } from "vitest";

import { buildUploadConfig, formatBytes, validateFile, type AuthStatusResponse, type UploadConfig } from "./api-client";

function makeConfig(overrides?: Partial<UploadConfig>): UploadConfig {
  return {
    csrfToken: "token",
    maxUploadBytes: 1024,
    allowedMimeTypes: new Set(["image/jpeg", "application/pdf"]),
    uploadUrl: "/api/uploads",
    ...overrides
  };
}

describe("validateFile", () => {
  it("rejects unsupported mime type", () => {
    const config = makeConfig();
    const file = { type: "text/plain", size: 100 } as File;
    expect(validateFile(config, file)).toContain("不支持的文件类型");
  });

  it("rejects oversized file", () => {
    const config = makeConfig();
    const file = { type: "image/jpeg", size: 4096 } as File;
    expect(validateFile(config, file)).toContain("文件超出大小限制");
  });

  it("accepts valid file", () => {
    const config = makeConfig();
    const file = { type: "image/jpeg", size: 512 } as File;
    expect(validateFile(config, file)).toBeNull();
  });

  it("accepts pdf file when mime type is allowed", () => {
    const config = makeConfig();
    const file = { type: "application/pdf", size: 512 } as File;
    expect(validateFile(config, file)).toBeNull();
  });
});

describe("formatBytes", () => {
  it("formats byte units", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(2048)).toBe("2.0 KB");
  });
});

describe("buildUploadConfig", () => {
  it("builds upload config from auth status", () => {
    const status: AuthStatusResponse = {
      authenticated: true,
      csrfToken: "csrf-token",
      maxUploadBytes: 2048,
      allowedMimeTypes: ["image/jpeg", "video/mp4", "application/pdf"]
    };

    const config = buildUploadConfig(status);
    expect(config.csrfToken).toBe("csrf-token");
    expect(config.maxUploadBytes).toBe(2048);
    expect(config.allowedMimeTypes.has("video/mp4")).toBe(true);
    expect(config.allowedMimeTypes.has("application/pdf")).toBe(true);
  });
});
