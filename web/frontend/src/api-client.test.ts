import { describe, expect, it } from "vitest";

import { formatBytes, validateFile, type UploadConfig } from "./api-client";

function makeConfig(overrides?: Partial<UploadConfig>): UploadConfig {
  return {
    csrfToken: "token",
    maxUploadBytes: 1024,
    allowedMimeTypes: new Set(["image/jpeg"]),
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
});

describe("formatBytes", () => {
  it("formats byte units", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(2048)).toBe("2.0 KB");
  });
});
