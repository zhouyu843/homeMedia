import React from "react";
import { createRoot } from "react-dom/client";

function UploadIslandApp() {
  return (
    <div
      style={{
        marginTop: "0.5rem",
        padding: "0.55rem 0.7rem",
        border: "1px solid #cbd5e1",
        borderRadius: "10px",
        fontSize: "0.85rem",
        color: "#334155",
        background: "#f8fafc"
      }}
      role="status"
      aria-live="polite"
    >
      React 上传岛屿已启用。下一步将接管拖拽上传、进度和失败重试交互。
    </div>
  );
}

const rootElement = document.getElementById("upload-island-root");
if (rootElement) {
  const root = createRoot(rootElement);
  root.render(
    <React.StrictMode>
      <UploadIslandApp />
    </React.StrictMode>
  );
}
