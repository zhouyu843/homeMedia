import { describe, expect, it } from "vitest";

import { summarizeUploads } from "./upload-metrics";

describe("summarizeUploads", () => {
  it("returns zero summary for empty list", () => {
    expect(summarizeUploads([])).toEqual({
      queued: 0,
      uploading: 0,
      success: 0,
      error: 0,
      overallProgress: 0
    });
  });

  it("counts statuses and computes weighted progress", () => {
    const summary = summarizeUploads([
      { status: "queued", progress: 0 },
      { status: "uploading", progress: 50 },
      { status: "success", progress: 100 },
      { status: "error", progress: 0 }
    ]);

    expect(summary.queued).toBe(1);
    expect(summary.uploading).toBe(1);
    expect(summary.success).toBe(1);
    expect(summary.error).toBe(1);
    expect(summary.overallProgress).toBe(63);
  });

  it("clamps uploading progress range", () => {
    const summary = summarizeUploads([
      { status: "uploading", progress: -20 },
      { status: "uploading", progress: 180 }
    ]);

    expect(summary.overallProgress).toBe(50);
  });
});
