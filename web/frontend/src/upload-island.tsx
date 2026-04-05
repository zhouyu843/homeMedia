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

type UploadStatus = "queued" | "uploading" | "success" | "error";

type UploadItem = {
  id: string;
  file: File;
  status: UploadStatus;
  progress: number;
  error?: string;
};

function UploadIslandApp({ config }: { config: UploadConfig }) {
  const [items, setItems] = React.useState<UploadItem[]>([]);
  const [isUploading, setIsUploading] = React.useState(false);

  const queuedCount = items.filter((item) => item.status === "queued").length;

  const addFiles = React.useCallback(
    (files: FileList | null) => {
      if (!files || files.length === 0) {
        return;
      }

      const nextItems: UploadItem[] = [];
      Array.from(files).forEach((file) => {
        const validationError = validateFile(config, file);
        nextItems.push({
          id: makeId(),
          file,
          status: validationError ? "error" : "queued",
          progress: 0,
          error: validationError ?? undefined
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
          item.id === itemId ? { ...item, status: "uploading", progress: 0, error: undefined } : item
        )
      );

      try {
        const asset = await uploadFile(config, target.file, (progress) => {
          setItems((current) =>
            current.map((item) => (item.id === itemId ? { ...item, progress } : item))
          );
        });

        setItems((current) =>
          current.map((item) =>
            item.id === itemId ? { ...item, status: "success", progress: 100 } : item
          )
        );
        prependAssetCard(asset);
      } catch (error) {
        setItems((current) =>
          current.map((item) =>
            item.id === itemId
              ? {
                  ...item,
                  status: "error",
                  error: error instanceof Error ? error.message : "upload failed"
                }
              : item
          )
        );
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
    setItems((current) => current.filter((item) => item.id !== itemId));
  };

  return (
    <div style={panelStyle}>
      <div style={toolbarStyle}>
        <input
          type="file"
          multiple
          onChange={(event) => addFiles(event.target.files)}
          style={{ maxWidth: "340px" }}
          aria-label="选择要上传的文件"
        />
        <button type="button" onClick={() => void uploadQueued()} disabled={isUploading || queuedCount === 0}>
          {isUploading ? "上传中..." : `上传待处理文件 (${queuedCount})`}
        </button>
      </div>

      <p style={hintStyle}>
        允许类型：{Array.from(config.allowedMimeTypes).join(", ")}；单文件上限：{formatBytes(config.maxUploadBytes)}
      </p>

      {items.length > 0 && (
        <ul style={listStyle}>
          {items.map((item) => (
            <li key={item.id} style={itemStyle}>
              <div>
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
                  {item.status === "error" && (
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

  const card = document.createElement("a");
  card.className = "card";
  card.href = asset.detailUrl;
  card.innerHTML = `
    <figure class="thumb-wrap">
      <img src="${asset.thumbnailUrl}" alt="${escapeHtml(asset.originalFilename)}" loading="lazy">
      ${asset.mediaType === "video" ? '<span class="badge">VIDEO</span>' : ""}
    </figure>
    <p class="name">${escapeHtml(asset.originalFilename)}</p>
  `;

  gallery.prepend(card);
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
