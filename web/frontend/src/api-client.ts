export type ApiAsset = {
  id: string;
  originalFilename: string;
  mediaType: "image" | "video";
  mimeType: string;
  sizeBytes: number;
  createdAt: string;
  detailUrl: string;
  viewUrl: string;
  thumbnailUrl: string;
  downloadUrl: string;
};

export type UploadConfig = {
  csrfToken: string;
  maxUploadBytes: number;
  allowedMimeTypes: Set<string>;
  uploadUrl: string;
};

type UploadError = Error & {
  code?: string;
};

export function parseUploadConfig(root: HTMLElement): UploadConfig {
  const csrfToken = root.dataset.csrfToken ?? "";
  const maxUploadBytes = Number(root.dataset.maxUploadBytes ?? "0");
  const allowedMimeTypes = new Set(
    (root.dataset.allowedMimeTypes ?? "")
      .split(",")
      .map((value) => value.trim())
      .filter((value) => value.length > 0)
  );
  const uploadUrl = root.dataset.uploadUrl ?? "/api/uploads";

  return {
    csrfToken,
    maxUploadBytes: Number.isFinite(maxUploadBytes) ? maxUploadBytes : 0,
    allowedMimeTypes,
    uploadUrl
  };
}

export async function uploadFile(
  config: UploadConfig,
  file: File,
  onProgress: (progress: number) => void
): Promise<ApiAsset> {
  const formData = new FormData();
  formData.append("file", file);
  formData.append("csrf_token", config.csrfToken);

  return new Promise<ApiAsset>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", config.uploadUrl, true);
    xhr.responseType = "json";
    xhr.setRequestHeader("X-CSRF-Token", config.csrfToken);

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0) {
        onProgress(Math.round((event.loaded / event.total) * 100));
      }
    };

    xhr.onerror = () => {
      reject(withCode(new Error("network error"), "network_error"));
    };

    xhr.onload = () => {
      const response = xhr.response as { asset?: ApiAsset; code?: string; message?: string } | null;
      if (xhr.status >= 200 && xhr.status < 300 && response?.asset) {
        onProgress(100);
        resolve(response.asset);
        return;
      }

      const message = response?.message ?? `upload failed with status ${xhr.status}`;
      reject(withCode(new Error(message), response?.code ?? "upload_failed"));
    };

    xhr.send(formData);
  });
}

export function validateFile(config: UploadConfig, file: File): string | null {
  if (config.allowedMimeTypes.size > 0 && !config.allowedMimeTypes.has(file.type)) {
    return `不支持的文件类型：${file.type || "未知类型"}`;
  }
  if (config.maxUploadBytes > 0 && file.size > config.maxUploadBytes) {
    return `文件超出大小限制（最大 ${formatBytes(config.maxUploadBytes)}）`;
  }
  return null;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unitIndex]}`;
}

function withCode(error: Error, code: string): UploadError {
  const uploadError = error as UploadError;
  uploadError.code = code;
  return uploadError;
}
