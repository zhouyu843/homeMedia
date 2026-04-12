import React from "react";

import {
  formatBytes,
  uploadFile,
  validateFile,
  type UploadConfig,
  type UploadDuplicateAction,
  type UploadResponse
} from "./api-client";
import { summarizeUploads } from "./upload-metrics";

const PAGE_DRAG_CLASS = "upload-page-drag-active";

type UploadStatus = "queued" | "uploading" | "success" | "error";

type UploadItem = {
  id: string;
  file: File;
  status: UploadStatus;
  progress: number;
  error?: string;
  errorCode?: string;
  previewKind: "image" | "video" | "pdf" | "none";
  previewUrl?: string;
  retryable: boolean;
};

type RecentResult = {
  id: string;
  filename: string;
  status: "success" | "error" | "info";
  message: string;
};

function hasFilePayload(dataTransfer: DataTransfer | null): boolean {
  if (!dataTransfer?.types) {
    return false;
  }

  return Array.from(dataTransfer.types).includes("Files");
}

export function UploadIslandApp({
  config,
  onUploadResolved
}: {
  config: UploadConfig;
  onUploadResolved?: (response: UploadResponse) => void;
}) {
  const [items, setItems] = React.useState<UploadItem[]>([]);
  const [isUploading, setIsUploading] = React.useState(false);
  const [isDragActive, setIsDragActive] = React.useState(false);
  const [recentResults, setRecentResults] = React.useState<RecentResult[]>([]);
  const fileInputRef = React.useRef<HTMLInputElement | null>(null);
  const itemsRef = React.useRef<UploadItem[]>([]);
  const dragDepthRef = React.useRef(0);

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
    document.body.classList.toggle(PAGE_DRAG_CLASS, isDragActive);
    return () => {
      document.body.classList.remove(PAGE_DRAG_CLASS);
    };
  }, [isDragActive]);

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

  React.useEffect(() => {
    const resetDragState = () => {
      dragDepthRef.current = 0;
      setIsDragActive(false);
    };

    const onWindowDragEnter = (event: DragEvent) => {
      if (!hasFilePayload(event.dataTransfer)) {
        return;
      }

      dragDepthRef.current += 1;
      setIsDragActive(true);
    };

    const onWindowDragOver = (event: DragEvent) => {
      if (!hasFilePayload(event.dataTransfer)) {
        return;
      }

      event.preventDefault();
      if (event.dataTransfer) {
        event.dataTransfer.dropEffect = "copy";
      }
      setIsDragActive(true);
    };

    const onWindowDragLeave = (event: DragEvent) => {
      if (!hasFilePayload(event.dataTransfer)) {
        return;
      }

      dragDepthRef.current = Math.max(0, dragDepthRef.current - 1);
      if (dragDepthRef.current === 0) {
        setIsDragActive(false);
      }
    };

    const onWindowDrop = (event: DragEvent) => {
      if (!hasFilePayload(event.dataTransfer)) {
        return;
      }

      event.preventDefault();
      addFiles(event.dataTransfer?.files ?? null);
      resetDragState();
    };

    window.addEventListener("dragenter", onWindowDragEnter);
    window.addEventListener("dragover", onWindowDragOver);
    window.addEventListener("dragleave", onWindowDragLeave);
    window.addEventListener("drop", onWindowDrop);
    window.addEventListener("dragend", resetDragState);

    return () => {
      window.removeEventListener("dragenter", onWindowDragEnter);
      window.removeEventListener("dragover", onWindowDragOver);
      window.removeEventListener("dragleave", onWindowDragLeave);
      window.removeEventListener("drop", onWindowDrop);
      window.removeEventListener("dragend", resetDragState);
      resetDragState();
    };
  }, [addFiles]);

  const uploadOne = React.useCallback(
    async (itemId: string, duplicateAction?: UploadDuplicateAction) => {
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
        const response = await uploadFile(
          config,
          target.file,
          (progress) => {
            setItems((current) =>
              current.map((item) => (item.id === itemId ? { ...item, progress } : item))
            );
          },
          duplicateAction
        );

        if (target.previewUrl) {
          URL.revokeObjectURL(target.previewUrl);
        }
        setItems((current) => current.filter((item) => item.id !== itemId));

        if (response.existing) {
          pushRecentResult({
            id: makeId(),
            filename: target.file.name,
            status: "info",
            message: "文件已存在，未重复上传"
          });
          return;
        }

        if (response.restored) {
          pushRecentResult({
            id: makeId(),
            filename: target.file.name,
            status: "success",
            message: "已从回收站恢复旧项"
          });
          onUploadResolved?.(response);
          return;
        }

        pushRecentResult({
          id: makeId(),
          filename: target.file.name,
          status: "success",
          message: "上传完成"
        });
        onUploadResolved?.(response);
      } catch (error) {
        const uploadError = error as Error & { code?: string };
        const message = error instanceof Error ? error.message : "upload failed";
        setItems((current) =>
          current.map((item) =>
            item.id === itemId
              ? {
                  ...item,
                  status: "error",
                  error: message,
                  errorCode: uploadError.code,
                  retryable: uploadError.code !== "trashed_duplicate"
                }
              : item
          )
        );
        pushRecentResult({
          id: makeId(),
          filename: target.file.name,
          status: uploadError.code === "trashed_duplicate" ? "info" : "error",
          message: uploadError.code === "trashed_duplicate" ? "命中回收站重复内容，等待你选择" : message
        });
      }
    },
    [config, items, onUploadResolved]
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

  const onDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    dragDepthRef.current = 0;
    setIsDragActive(false);
    addFiles(event.dataTransfer.files);
  };

  const onDragOver = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "copy";
    setIsDragActive(true);
  };

  const onDragLeave = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) {
      return;
    }
    dragDepthRef.current = 0;
    setIsDragActive(false);
  };

  const onDropZoneKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      fileInputRef.current?.click();
    }
  };

  const pushRecentResult = (result: RecentResult) => {
    setRecentResults((current) => [result, ...current].slice(0, 6));
  };

  return (
    <div className="upload-panel">
      {isDragActive && (
        <div className="page-drop-overlay" aria-hidden="true">
          <div className="page-drop-message">松开即可上传到当前页面</div>
        </div>
      )}
      <div
        className={`drop-zone ${isDragActive ? "is-active" : ""}`}
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

      <div className="upload-toolbar">
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={Array.from(config.allowedMimeTypes).join(",")}
          onChange={(event) => addFiles(event.target.files)}
          aria-label="选择要上传的文件"
        />
        <button type="button" onClick={() => void uploadQueued()} disabled={isUploading || queueCount === 0}>
          {isUploading ? "上传中..." : `上传待处理文件 (${queueCount})`}
        </button>
      </div>

      <p className="upload-hint">
        允许类型：{Array.from(config.allowedMimeTypes).join(", ")}；单文件上限：{formatBytes(config.maxUploadBytes)}
      </p>

      <div className="summary-row">
        <div className="summary-text">
          总体进度：{summary.overallProgress}% · 成功 {summary.success} · 失败 {summary.error} · 队列 {summary.queued}
        </div>
        <div className="summary-track">
          <div className="summary-bar" style={{ width: `${summary.overallProgress}%` }} />
        </div>
      </div>

      {recentResults.length > 0 && (
        <div className="recent-box">
          <strong>最近上传结果</strong>
          <ul className="recent-list">
            {recentResults.map((result) => (
              <li key={result.id} className="recent-item">
                <span>{result.filename}</span>
                <span className={`recent-tag recent-${result.status}`}>
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
        <ul className="upload-list">
          {items.map((item) => (
            <li key={item.id} className="upload-item">
              <div className="upload-left">
                <div className="upload-preview-wrap">{renderPreview(item)}</div>
                <div className="upload-file-info">
                  <strong>{item.file.name}</strong>
                  <div className="upload-subtext">
                    {formatBytes(item.file.size)} · {item.file.type || "未知类型"}
                  </div>
                </div>
              </div>
              <div className="upload-status-block">
                <StatusChip status={item.status} />
                {item.status === "uploading" && (
                  <div className="upload-progress">
                    <div className="upload-progress-bar" style={{ width: `${item.progress}%` }} />
                  </div>
                )}
                {item.error && <div className="upload-error">{item.error}</div>}
                <div className="upload-actions">
                  {item.status === "error" && item.retryable && (
                    <button type="button" onClick={() => void uploadOne(item.id)} disabled={isUploading}>
                      重试
                    </button>
                  )}
                  {item.status === "error" && item.errorCode === "trashed_duplicate" && (
                    <>
                      <button type="button" onClick={() => void uploadOne(item.id, "restore")} disabled={isUploading}>
                        恢复旧项
                      </button>
                      <button type="button" onClick={() => void uploadOne(item.id, "new")} disabled={isUploading}>
                        继续新建
                      </button>
                    </>
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
    return <img src={item.previewUrl} alt={item.file.name} className="upload-preview-image" />;
  }

  if (item.previewKind === "video") {
    return <span className="upload-preview-pill">VIDEO</span>;
  }

  if (item.previewKind === "pdf") {
    return <span className="upload-preview-pill">PDF</span>;
  }

  return <span className="upload-preview-fallback">FILE</span>;
}

function createPreviewForFile(file: File): { kind: UploadItem["previewKind"]; previewUrl?: string } {
  if (file.type.startsWith("image/")) {
    return { kind: "image", previewUrl: URL.createObjectURL(file) };
  }
  if (file.type.startsWith("video/")) {
    return { kind: "video" };
  }
  if (file.type === "application/pdf" || file.name.toLowerCase().endsWith(".pdf")) {
    return { kind: "pdf" };
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
  return <span className="upload-status-chip">{labelMap[status]}</span>;
}

function makeId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}
