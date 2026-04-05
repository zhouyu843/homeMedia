export type UploadProgressItem = {
  status: "queued" | "uploading" | "success" | "error";
  progress: number;
};

export type UploadSummary = {
  queued: number;
  uploading: number;
  success: number;
  error: number;
  overallProgress: number;
};

export function summarizeUploads(items: UploadProgressItem[]): UploadSummary {
  const summary: UploadSummary = {
    queued: 0,
    uploading: 0,
    success: 0,
    error: 0,
    overallProgress: 0
  };

  if (items.length === 0) {
    return summary;
  }

  let weightedProgress = 0;
  for (const item of items) {
    summary[item.status] += 1;

    if (item.status === "queued") {
      continue;
    }
    if (item.status === "uploading") {
      weightedProgress += clampPercent(item.progress);
      continue;
    }

    weightedProgress += 100;
  }

  summary.overallProgress = Math.round(weightedProgress / items.length);
  return summary;
}

function clampPercent(value: number): number {
  if (value < 0) {
    return 0;
  }
  if (value > 100) {
    return 100;
  }
  return value;
}
