import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ApiAsset, UploadConfig } from "./api-client";

const { uploadFileMock } = vi.hoisted(() => ({
  uploadFileMock: vi.fn()
}));

vi.mock("./api-client", async () => {
  const actual = await vi.importActual<typeof import("./api-client")>("./api-client");
  return {
    ...actual,
    uploadFile: uploadFileMock
  };
});

import { UploadIslandApp } from "./upload-island";

function makeConfig(overrides?: Partial<UploadConfig>): UploadConfig {
  return {
    csrfToken: "token",
    maxUploadBytes: 10 * 1024 * 1024,
    allowedMimeTypes: new Set(["image/jpeg", "video/mp4"]),
    uploadUrl: "/api/uploads",
    ...overrides
  };
}

function makeAsset(overrides?: Partial<ApiAsset>): ApiAsset {
  return {
    id: "asset-1",
    originalFilename: "photo.jpg",
    mediaType: "image",
    mimeType: "image/jpeg",
    sizeBytes: 512,
    createdAt: "2026-04-06T10:00:00Z",
    detailUrl: "/media/asset-1",
    viewUrl: "/media/asset-1/view",
    thumbnailUrl: "/media/asset-1/thumbnail",
    downloadUrl: "/media/asset-1/download",
    ...overrides
  };
}

function makeFile(name: string, type = "image/jpeg", contents = "file-content"): File {
  return new File([contents], name, { type });
}

function createDataTransfer(files: File[]) {
  return {
    files,
    types: ["Files"],
    dropEffect: "none"
  } as unknown as DataTransfer;
}

describe("UploadIslandApp", () => {
  const createObjectURL = vi.fn(() => "blob:preview");
  const revokeObjectURL = vi.fn();

  beforeEach(() => {
    uploadFileMock.mockReset();
    createObjectURL.mockClear();
    revokeObjectURL.mockClear();
    vi.stubGlobal("URL", {
      createObjectURL,
      revokeObjectURL
    });
    document.body.innerHTML = '<div class="shell"><p class="empty">还没有上传任何媒体。</p></div>';
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.className = "";
    document.body.innerHTML = "";
  });

  it("removes successful items from queue and keeps recent result", async () => {
    uploadFileMock.mockResolvedValue({
      asset: makeAsset(),
      existing: false
    });

    const { container } = render(React.createElement(UploadIslandApp, { config: makeConfig() }));

    const input = screen.getByLabelText("选择要上传的文件") as HTMLInputElement;
    const file = makeFile("success.jpg");
    fireEvent.change(input, { target: { files: [file] } });

    expect(screen.getByText("success.jpg")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "上传待处理文件 (1)" }));

    await waitFor(() => {
      expect(container.querySelector("li strong")).toBeNull();
    });

    expect(screen.getByText("最近上传结果")).toBeTruthy();
    expect(within(screen.getByText("最近上传结果").closest("div") as HTMLElement).getByText("success.jpg")).toBeTruthy();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:preview");
    expect(document.querySelector('.gallery .card[href="/media/asset-1"]')).toBeTruthy();
  });

  it("keeps failed items in queue and allows retry", async () => {
    uploadFileMock.mockRejectedValueOnce(new Error("network error"));
    uploadFileMock.mockResolvedValueOnce({
      asset: makeAsset({ id: "asset-2", detailUrl: "/media/asset-2" }),
      existing: false
    });

    const { container } = render(React.createElement(UploadIslandApp, { config: makeConfig() }));

    const input = screen.getByLabelText("选择要上传的文件") as HTMLInputElement;
    fireEvent.change(input, { target: { files: [makeFile("retry.jpg")] } });
    fireEvent.click(screen.getByRole("button", { name: "上传待处理文件 (1)" }));

    await screen.findByText("失败");
    expect(container.querySelector("li strong")?.textContent).toBe("retry.jpg");
    expect(screen.getByText("network error")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "重试" }));

    await waitFor(() => {
      expect(container.querySelector("li strong")).toBeNull();
    });

    expect(uploadFileMock).toHaveBeenCalledTimes(2);
  });

  it("offers restore choice for trashed duplicate uploads", async () => {
    const duplicateError = Object.assign(new Error("发现回收站中的同内容文件，请选择恢复旧项或继续新建"), {
      code: "trashed_duplicate",
      asset: makeAsset({ id: "asset-deleted", detailUrl: "/media/asset-deleted" })
    });
    uploadFileMock.mockRejectedValueOnce(duplicateError);
    uploadFileMock.mockResolvedValueOnce({
      asset: makeAsset({ id: "asset-deleted", detailUrl: "/media/asset-deleted" }),
      existing: false,
      restored: true
    });

    const { container } = render(React.createElement(UploadIslandApp, { config: makeConfig() }));

    const input = screen.getByLabelText("选择要上传的文件") as HTMLInputElement;
    fireEvent.change(input, { target: { files: [makeFile("restore.jpg")] } });
    fireEvent.click(screen.getByRole("button", { name: "上传待处理文件 (1)" }));

    await screen.findByRole("button", { name: "恢复旧项" });
    expect(screen.getByRole("button", { name: "继续新建" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "恢复旧项" }));

    await waitFor(() => {
      expect(container.querySelector("li strong")).toBeNull();
    });

    expect(uploadFileMock).toHaveBeenNthCalledWith(2, expect.anything(), expect.any(File), expect.any(Function), "restore");
    expect(screen.getByText("最近上传结果")).toBeTruthy();
    expect(within(screen.getByText("最近上传结果").closest("div") as HTMLElement).getAllByText("restore.jpg").length).toBeGreaterThan(0);
  });

  it("accepts file drop from the whole page", async () => {
    render(React.createElement(UploadIslandApp, { config: makeConfig() }));

    const file = makeFile("page-drop.jpg");
    const dragData = createDataTransfer([file]);
    fireEvent.dragEnter(window, { dataTransfer: dragData });
    fireEvent.dragOver(window, { dataTransfer: dragData });

    expect(screen.getByText("松开即可上传到当前页面")).toBeTruthy();
    expect(document.body.classList.contains("upload-page-drag-active")).toBe(true);

    fireEvent.drop(window, { dataTransfer: dragData });

    await screen.findByText("page-drop.jpg");
    expect(document.body.classList.contains("upload-page-drag-active")).toBe(false);
  });

  it("ignores non-file drag events", () => {
    render(React.createElement(UploadIslandApp, { config: makeConfig() }));

    fireEvent.dragEnter(window, {
      dataTransfer: {
        files: [],
        types: ["text/plain"]
      }
    });

    expect(screen.queryByText("松开即可上传到当前页面")).toBeNull();
    expect(document.body.classList.contains("upload-page-drag-active")).toBe(false);
  });
});
