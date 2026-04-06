export type ApiAsset = {
  id: string;
  originalFilename: string;
  mediaType: "image" | "video";
  mimeType: string;
  sizeBytes: number;
  createdAt: string;
  deletedAt?: string;
  detailUrl: string;
  viewUrl: string;
  thumbnailUrl: string;
  downloadUrl: string;
};

export type AuthStatusResponse = {
  authenticated: boolean;
  username?: string;
  csrfToken: string;
  maxUploadBytes: number;
  allowedMimeTypes: string[];
};

export type UploadResponse = {
  asset: ApiAsset;
  existing: boolean;
  restored?: boolean;
};

export type UploadDuplicateAction = "restore" | "new";

export type UploadConfig = {
  csrfToken: string;
  maxUploadBytes: number;
  allowedMimeTypes: Set<string>;
  uploadUrl: string;
};

type ApiEnvelope<T> = T & {
  code?: string;
  message?: string;
};

type UploadError = Error & {
  code?: string;
  asset?: ApiAsset;
  status?: number;
};

export class ApiClientError extends Error {
  code?: string;
  status: number;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiClientError";
    this.status = status;
    this.code = code;
  }
}

export function buildUploadConfig(status: AuthStatusResponse): UploadConfig {
  return {
    csrfToken: status.csrfToken,
    maxUploadBytes: status.maxUploadBytes,
    allowedMimeTypes: new Set(status.allowedMimeTypes),
    uploadUrl: "/api/uploads"
  };
}

export async function getAuthStatus(): Promise<AuthStatusResponse> {
  return requestJSON<AuthStatusResponse>("/api/auth/status");
}

export async function login(username: string, password: string, csrfToken: string): Promise<AuthStatusResponse> {
  return requestJSON<AuthStatusResponse>("/api/login", {
    method: "POST",
    body: JSON.stringify({ username, password, csrfToken })
  });
}

export async function logout(csrfToken: string): Promise<void> {
  await requestJSON<{ ok: boolean }>("/api/logout", {
    method: "POST",
    headers: {
      "X-CSRF-Token": csrfToken
    }
  });
}

export async function listMedia(): Promise<ApiAsset[]> {
  const response = await requestJSON<{ assets: ApiAsset[] }>("/api/media");
  return response.assets;
}

export async function getMedia(id: string): Promise<ApiAsset> {
  const response = await requestJSON<{ asset: ApiAsset }>(`/api/media/${id}`);
  return response.asset;
}

export async function listTrash(): Promise<ApiAsset[]> {
  const response = await requestJSON<{ assets: ApiAsset[] }>("/api/trash");
  return response.assets;
}

export async function deleteMedia(id: string, csrfToken: string): Promise<void> {
  await postWithCSRF(`/api/media/${id}/delete`, csrfToken);
}

export async function restoreMedia(id: string, csrfToken: string): Promise<void> {
  await postWithCSRF(`/api/media/${id}/restore`, csrfToken);
}

export async function permanentlyDeleteMedia(id: string, csrfToken: string): Promise<void> {
  await postWithCSRF(`/api/media/${id}/permanent-delete`, csrfToken);
}

export async function emptyTrash(csrfToken: string): Promise<void> {
  await postWithCSRF("/api/trash/empty", csrfToken);
}

export async function uploadFile(
  config: UploadConfig,
  file: File,
  onProgress: (progress: number) => void,
  duplicateAction?: UploadDuplicateAction
): Promise<UploadResponse> {
  const formData = new FormData();
  formData.append("file", file);
  formData.append("csrf_token", config.csrfToken);
  if (duplicateAction) {
    formData.append("trashed_duplicate_action", duplicateAction);
  }

  return new Promise<UploadResponse>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", config.uploadUrl, true);
    xhr.withCredentials = true;
    xhr.responseType = "json";
    xhr.setRequestHeader("X-CSRF-Token", config.csrfToken);
    xhr.setRequestHeader("Accept", "application/json");

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0) {
        onProgress(Math.round((event.loaded / event.total) * 100));
      }
    };

    xhr.onerror = () => {
      reject(withDetails(new Error("network error"), "network_error", undefined, xhr.status));
    };

    xhr.onload = () => {
      const response = xhr.response as ApiEnvelope<{
        asset?: ApiAsset;
        existing?: boolean;
        restored?: boolean;
      }> | null;
      if (xhr.status >= 200 && xhr.status < 300 && response?.asset) {
        onProgress(100);
        resolve({
          asset: response.asset,
          existing: response.existing === true,
          restored: response.restored === true
        });
        return;
      }

      const message = response?.message ?? `upload failed with status ${xhr.status}`;
      reject(withDetails(new Error(message), response?.code ?? "upload_failed", response?.asset, xhr.status));
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

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: "same-origin",
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
      ...(init?.headers ?? {})
    }
  });

  const payload = (await response.json().catch(() => null)) as ApiEnvelope<T> | null;
  if (!response.ok) {
    throw new ApiClientError(payload?.message ?? `request failed with status ${response.status}`, response.status, payload?.code);
  }

  return (payload ?? {}) as T;
}

async function postWithCSRF(url: string, csrfToken: string): Promise<void> {
  await requestJSON<{ ok: boolean }>(url, {
    method: "POST",
    headers: {
      "X-CSRF-Token": csrfToken
    }
  });
}

function withDetails(error: Error, code: string, asset?: ApiAsset, status?: number): UploadError {
  const uploadError = error as UploadError;
  uploadError.code = code;
  uploadError.asset = asset;
  uploadError.status = status;
  return uploadError;
}
