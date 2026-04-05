import React from "react";
import { createRoot } from "react-dom/client";

import {
  formatBytes,
  parseUploadConfig,
  uploadFile,
  validateFile,
  type ApiAsset,
  type UploadConfig
} from "./api-client";
import { summarizeUploads } from "./upload-metrics";

type UploadStatus = "queued" | "uploading" | "success" | "error";

type UploadItem = {
  id: string;
  file: File;
  status: UploadStatus;
  progress: number;
  error?: string;
  previewKind: "image" | "video" | "none";
  previewUrl?: string;
  retryable: boolean;
};

type RecentResult = {
  id: string;
  filename: string;
  status: "success" | "error" | "info";
  message: string;
};

function UploadIslandApp({ config }: { config: UploadConfig }) {
  const [items, setItems] = React.useState<UploadItem[]>([]);
  const [isUploading, setIsUploading] = React.useState(false);
  const [isDragActive, setIsDragActive] = React.useState(false);
  const [recentResults, setRecentResults] = React.useState<RecentResult[]>([]);
  const fileInputRef = React.useRef<HTMLInputElement | null>(null);
  const itemsRef = React.useRef<UploadItem[]>([]);

  const summary = React.useMemo(
    () =>
      summarizeUploads(
        items.map((item) => ({
          status: item.status,
          progress: item.progress
        }))
      ),
    [items]
  );

  const queueCount = summary.queued + summary.uploading;

  React.useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  React.useEffect(() => {
    return () => {
      for (const item of itemsRef.current) {
        if (item.previewUrl) {
          URL.revokeObjectURL(item.previewUrl);
        }
      }
    };
  }, []);

  React.useEffect(() => {
    bindGalleryThumbnailFallbacks();
  }, []);

  const addFiles = React.useCallback(
    (files: FileList | null) => {
      if (!files || files.length === 0) {
        return;
      }

      const nextItems: UploadItem[] = [];
      Array.from(files).forEach((file) => {
        const preview = createPreviewForFile(file);
        const validationError = validateFile(config, file);
        nextItems.push({
          id: makeId(),
          file,
          status: validationError ? "error" : "queued",
          progress: 0,
          error: validationError ?? undefined,
          previewKind: preview.kind,
          previewUrl: preview.previewUrl,
          retryable: !validationError
        });
      });

      setItems((current) => [...current, ...nextItems]);
    },
    [config]
  );

  const uploadOne = React.useCallback(
    async (itemId: string) => {
      const target = items.find((item) => item.id === itemId);
      if (!target || (target.status !== "queued" && target.status !== "error")) {
        return;
      }

      setItems((current) =>
        current.map((item) =>
          item.id === itemId
            ? { ...item, status: "uploading", progress: 0, error: undefined, retryable: true }
            : item
        )
      );

      try {
        const response = await uploadFile(config, target.file, (progress) => {
          setItems((current) =>
            current.map((item) => (item.id === itemId ? { ...item, progress } : item))
          );
        });
        const asset = response.asset;

        if (response.existing) {
          if (target.previewUrl) {
            URL.revokeObjectURL(target.previewUrl);
          }
          setItems((current) => current.filter((item) => item.id !== itemId));
          pushRecentResult({
            id: makeId(),
            filename: target.file.name,
            status: "info",
            message: "文件已存在，未重复上传"
          });
          return;
        }

        setItems((current) =>
          current.map((item) =>
            item.id === itemId ? { ...item, status: "success", progress: 100 } : item
          )
        );
        pushRecentResult({
          id: makeId(),
          filename: target.file.name,
          status: "success",
          message: "上传完成"
        });
        prependAssetCard(asset);
        bindGalleryThumbnailFallbacks();
      } catch (error) {
        const message = error instanceof Error ? error.message : "upload failed";
        setItems((current) =>
          current.map((item) =>
            item.id === itemId
              ? {
                  ...item,
                  status: "error",
                  error: message,
                  retryable: true
                }
              : item
          )
        );
        pushRecentResult({
          id: makeId(),
          filename: target.file.name,
          status: "error",
          message
        });
      }
    },
    [config, items]
  );

  const uploadQueued = React.useCallback(async () => {
    const queue = items.filter((item) => item.status === "queued");
    if (queue.length === 0) {
      return;
    }

    setIsUploading(true);
    for (const item of queue) {
      // eslint-disable-next-line no-await-in-loop
      await uploadOne(item.id);
    }
    setIsUploading(false);
  }, [items, uploadOne]);

  const removeItem = (itemId: string) => {
    setItems((current) => {
      const target = current.find((item) => item.id === itemId);
      if (target?.previewUrl) {
        URL.revokeObjectURL(target.previewUrl);
      }
      return current.filter((item) => item.id !== itemId);
    });
  };

  const openFileDialog = () => {
    fileInputRef.current?.click();
  };

  const onDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setIsDragActive(false);
    addFiles(event.dataTransfer.files);
  };

  const onDragOver = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
    setIsDragActive(true);
  };

  const onDragLeave = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setIsDragActive(false);
  };

  const onDropZoneKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      openFileDialog();
    }
  };

  const pushRecentResult = (result: RecentResult) => {
    setRecentResults((current) => [result, ...current].slice(0, 6));
  };

  return (
    <div style={panelStyle}>
      <div
        style={{ ...dropZoneStyle, ...(isDragActive ? dropZoneActiveStyle : {}) }}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
        onKeyDown={onDropZoneKeyDown}
        tabIndex={0}
        role="button"
        aria-label="拖拽文件到此区域或按回车选择文件"
      >
        {isDragActive ? "松开以上传文件" : "拖拽文件到这里，或点击下方按钮选择文件"}
      </div>

      <div style={toolbarStyle}>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          onChange={(event) => addFiles(event.target.files)}
          style={{ maxWidth: "340px" }}
          aria-label="选择要上传的文件"
        />
        <button type="button" onClick={() => void uploadQueued()} disabled={isUploading || queueCount === 0}>
          {isUploading ? "上传中..." : `上传待处理文件 (${queueCount})`}
        </button>
      </div>

      <p style={hintStyle}>
        允许类型：{Array.from(config.allowedMimeTypes).join(", ")}；单文件上限：{formatBytes(config.maxUploadBytes)}
      </p>

      <div style={summaryRowStyle}>
        <div style={summaryTextStyle}>
          总体进度：{summary.overallProgress}% · 成功 {summary.success} · 失败 {summary.error} · 队列 {summary.queued}
        </div>
        <div style={overallProgressTrackStyle}>
          <div style={{ ...overallProgressBarStyle, width: `${summary.overallProgress}%` }} />
        </div>
      </div>

      {recentResults.length > 0 && (
        <div style={recentBoxStyle}>
          <strong style={{ fontSize: "0.82rem" }}>最近上传结果</strong>
          <ul style={recentListStyle}>
            {recentResults.map((result) => (
              <li key={result.id} style={recentItemStyle}>
                <span>{result.filename}</span>
                <span
                  style={
                    result.status === "success"
                      ? recentSuccessStyle
                      : result.status === "info"
                        ? recentInfoStyle
                        : recentErrorStyle
                  }
                >
                  {result.status === "success"
                    ? "成功"
                    : result.status === "info"
                      ? result.message
                      : `失败: ${result.message}`}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {items.length > 0 && (
        <ul style={listStyle}>
          {items.map((item) => (
            <li key={item.id} style={itemStyle}>
              <div style={leftBlockStyle}>
                <div style={previewWrapStyle}>{renderPreview(item)}</div>
                <strong>{item.file.name}</strong>
                <div style={subTextStyle}>
                  {formatBytes(item.file.size)} · {item.file.type || "未知类型"}
                </div>
              </div>
              <div style={statusBlockStyle}>
                <StatusChip status={item.status} />
                {item.status === "uploading" && (
                  <div style={progressStyle}>
                    <div style={{ ...progressBarStyle, width: `${item.progress}%` }} />
                  </div>
                )}
                {item.error && <div style={errorStyle}>{item.error}</div>}
                <div style={actionRowStyle}>
                  {item.status === "error" && item.retryable && (
                    <button type="button" onClick={() => void uploadOne(item.id)} disabled={isUploading}>
                      重试
                    </button>
                  )}
                  {item.status !== "uploading" && (
                    <button type="button" onClick={() => removeItem(item.id)}>
                      移除
                    </button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function renderPreview(item: UploadItem) {
  if (item.previewKind === "image" && item.previewUrl) {
    return <img src={item.previewUrl} alt={item.file.name} style={previewImageStyle} />;
  }

  if (item.previewKind === "video") {
    return <span style={previewVideoStyle}>VIDEO</span>;
  }

  return <span style={previewFallbackStyle}>FILE</span>;
}

function createPreviewForFile(file: File): { kind: UploadItem["previewKind"]; previewUrl?: string } {
  if (file.type.startsWith("image/")) {
    return { kind: "image", previewUrl: URL.createObjectURL(file) };
  }
  if (file.type.startsWith("video/")) {
    return { kind: "video" };
  }
  return { kind: "none" };
}

function StatusChip({ status }: { status: UploadStatus }) {
  const labelMap: Record<UploadStatus, string> = {
    queued: "待上传",
    uploading: "上传中",
    success: "完成",
    error: "失败"
  };
  return <span style={chipStyle}>{labelMap[status]}</span>;
}

function prependAssetCard(asset: ApiAsset) {
  const shell = document.querySelector(".shell");
  if (!shell) {
    return;
  }

  let gallery = shell.querySelector(".gallery") as HTMLElement | null;
  if (!gallery) {
    gallery = document.createElement("section");
    gallery.className = "gallery";
    const empty = shell.querySelector(".empty");
    if (empty) {
      empty.remove();
    }
    shell.appendChild(gallery);
  }

  const existingCard = gallery.querySelector(`.card[href="${asset.detailUrl}"]`);
  if (existingCard) {
    return;
  }

  const card = document.createElement("a");
  card.className = "card";
  card.href = asset.detailUrl;
  card.innerHTML = `
    <figure class="thumb-wrap">
      <img src="${asset.thumbnailUrl}" alt="${escapeHtml(asset.originalFilename)}" loading="lazy" data-thumb-kind="${asset.mediaType}" ${asset.mediaType === "image" ? `data-fallback-src="${asset.viewUrl}"` : ""}>
      ${asset.mediaType === "video" ? '<span class="badge">VIDEO</span>' : ""}
    </figure>
    <p class="name">${escapeHtml(asset.originalFilename)}</p>
  `;

  gallery.prepend(card);
}

function bindGalleryThumbnailFallbacks() {
  const images = document.querySelectorAll<HTMLImageElement>(".gallery img[data-thumb-kind]");
  images.forEach((image) => {
    if (image.dataset.thumbBound === "true") {
      return;
    }

    image.dataset.thumbBound = "true";
    image.addEventListener(
      "error",
      () => {
        const kind = image.dataset.thumbKind;
        const fallbackSrc = image.dataset.fallbackSrc;
        if (kind === "image" && fallbackSrc && image.src !== fallbackSrc) {
          image.src = fallbackSrc;
          return;
        }

        const wrapper = image.closest(".thumb-wrap");
        if (!wrapper) {
          return;
        }

        image.remove();
        if (wrapper.querySelector(".thumb-fallback")) {
          return;
        }

        const fallback = document.createElement("div");
        fallback.className = "thumb-fallback";
        fallback.textContent = kind === "video" ? "VIDEO" : "PREVIEW";
        Object.assign(fallback.style, {
          width: "100%",
          height: "100%",
          display: "grid",
          placeItems: "center",
          color: "#475569",
          fontSize: "0.78rem",
          letterSpacing: "0.06em",
          background: "linear-gradient(135deg, #e2e8f0, #cbd5e1)"
        });
        wrapper.appendChild(fallback);
      },
      { once: true }
    );
  });
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function makeId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

const panelStyle: React.CSSProperties = {
  marginTop: "0.5rem",
  padding: "0.7rem",
  border: "1px solid #cbd5e1",
  borderRadius: "10px",
  background: "#f8fafc"
};

const toolbarStyle: React.CSSProperties = {
  display: "flex",
  gap: "0.6rem",
  flexWrap: "wrap",
  alignItems: "center"
};

const dropZoneStyle: React.CSSProperties = {
  border: "2px dashed #94a3b8",
  borderRadius: "10px",
  padding: "0.75rem",
  color: "#334155",
  background: "#f1f5f9",
  fontSize: "0.86rem",
  textAlign: "center",
  outline: "none"
};

const dropZoneActiveStyle: React.CSSProperties = {
  borderColor: "#0f766e",
  background: "#ccfbf1",
  color: "#0f172a"
};

const hintStyle: React.CSSProperties = {
  margin: "0.6rem 0",
  color: "#475569",
  fontSize: "0.82rem"
};

const listStyle: React.CSSProperties = {
  display: "grid",
  gap: "0.55rem",
  listStyle: "none",
  margin: 0,
  padding: 0
};

const leftBlockStyle: React.CSSProperties = {
  display: "grid",
  gap: "0.3rem",
  alignItems: "start"
};

const previewWrapStyle: React.CSSProperties = {
  width: "80px",
  height: "60px",
  borderRadius: "8px",
  overflow: "hidden",
  border: "1px solid #dbe1ea",
  background: "#e2e8f0",
  display: "grid",
  placeItems: "center"
};

const previewImageStyle: React.CSSProperties = {
  width: "100%",
  height: "100%",
  objectFit: "cover"
};

const previewVideoStyle: React.CSSProperties = {
  fontSize: "0.72rem",
  letterSpacing: "0.04em",
  borderRadius: "999px",
  border: "1px solid #334155",
  padding: "0.08rem 0.35rem",
  color: "#334155"
};

const previewFallbackStyle: React.CSSProperties = {
  fontSize: "0.72rem",
  color: "#64748b"
};

const itemStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  gap: "1rem",
  border: "1px solid #dbe1ea",
  borderRadius: "10px",
  padding: "0.55rem 0.7rem",
  background: "#ffffff"
};

const statusBlockStyle: React.CSSProperties = {
  minWidth: "180px",
  display: "grid",
  gap: "0.25rem",
  justifyItems: "end"
};

const subTextStyle: React.CSSProperties = {
  fontSize: "0.8rem",
  color: "#64748b"
};

const chipStyle: React.CSSProperties = {
  fontSize: "0.74rem",
  borderRadius: "999px",
  border: "1px solid #94a3b8",
  padding: "0.08rem 0.45rem"
};

const progressStyle: React.CSSProperties = {
  width: "160px",
  height: "7px",
  borderRadius: "999px",
  background: "#e2e8f0",
  overflow: "hidden"
};

const progressBarStyle: React.CSSProperties = {
  height: "100%",
  background: "#0f766e",
  transition: "width 120ms ease"
};

const errorStyle: React.CSSProperties = {
  fontSize: "0.76rem",
  color: "#b91c1c",
  textAlign: "right"
};

const actionRowStyle: React.CSSProperties = {
  display: "flex",
  gap: "0.35rem"
};

const summaryRowStyle: React.CSSProperties = {
  display: "grid",
  gap: "0.3rem",
  marginBottom: "0.6rem"
};

const summaryTextStyle: React.CSSProperties = {
  fontSize: "0.82rem",
  color: "#334155"
};

const overallProgressTrackStyle: React.CSSProperties = {
  width: "100%",
  height: "8px",
  borderRadius: "999px",
  background: "#cbd5e1",
  overflow: "hidden"
};

const overallProgressBarStyle: React.CSSProperties = {
  height: "100%",
  background: "#0f766e",
  transition: "width 120ms ease"
};

const recentBoxStyle: React.CSSProperties = {
  border: "1px solid #dbe1ea",
  borderRadius: "10px",
  background: "#ffffff",
  padding: "0.55rem 0.7rem",
  marginBottom: "0.6rem",
  display: "grid",
  gap: "0.3rem"
};

const recentListStyle: React.CSSProperties = {
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "grid",
  gap: "0.2rem"
};

const recentItemStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  gap: "0.6rem",
  fontSize: "0.78rem"
};

const recentSuccessStyle: React.CSSProperties = {
  color: "#0f766e"
};

const recentInfoStyle: React.CSSProperties = {
  color: "#0f766e"
};

const recentErrorStyle: React.CSSProperties = {
  color: "#b91c1c"
};

const rootElement = document.getElementById("upload-island-root");
if (rootElement) {
  const config = parseUploadConfig(rootElement);
  const root = createRoot(rootElement);
  root.render(
    <React.StrictMode>
      <UploadIslandApp config={config} />
    </React.StrictMode>
  );
}
