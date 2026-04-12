import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ApiAsset, AuthStatusResponse } from "./api-client";

const { getMediaMock, listMediaMock } = vi.hoisted(() => ({
  getMediaMock: vi.fn(),
  listMediaMock: vi.fn()
}));

vi.mock("./api-client", async () => {
  const actual = await vi.importActual<typeof import("./api-client")>("./api-client");
  return {
    ...actual,
    getMedia: getMediaMock,
    listMedia: listMediaMock
  };
});

vi.mock("./upload-island", () => ({
  UploadIslandApp: () => React.createElement("div", { "data-testid": "upload-island" })
}));

import { MediaDetailPage, MediaListPage } from "./app";

function makeAsset(overrides?: Partial<ApiAsset>): ApiAsset {
  return {
    id: "asset-1",
    originalFilename: "mountain-lake.jpg",
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

function makeStatus(overrides?: Partial<AuthStatusResponse>): AuthStatusResponse {
  return {
    authenticated: true,
    username: "admin",
    csrfToken: "csrf-token",
    maxUploadBytes: 10 * 1024 * 1024,
    allowedMimeTypes: ["image/jpeg", "video/mp4", "application/pdf"],
    ...overrides
  };
}

describe("MediaListPage", () => {
  beforeEach(() => {
    getMediaMock.mockReset();
    listMediaMock.mockReset();
  });

  it("renders thumbnail-only gallery cards without filename or size text blocks", async () => {
    listMediaMock.mockResolvedValue([
      makeAsset(),
      makeAsset({
        id: "asset-2",
        originalFilename: "clip.mp4",
        mediaType: "video",
        mimeType: "video/mp4",
        sizeBytes: 2048,
        detailUrl: "/media/asset-2",
        viewUrl: "/media/asset-2/view",
        thumbnailUrl: "/media/asset-2/thumbnail",
        downloadUrl: "/media/asset-2/download"
      })
    ]);

    render(
      React.createElement(
        MemoryRouter,
        { initialEntries: ["/media"] },
        React.createElement(MediaListPage, {
          session: { loading: false, status: makeStatus() },
          onSessionChange: vi.fn()
        })
      )
    );

    await waitFor(() => {
      expect(listMediaMock).toHaveBeenCalledTimes(1);
    });

    const imageThumb = screen.getByAltText("mountain-lake.jpg");
    expect(imageThumb).toBeTruthy();
    expect(imageThumb.closest("a")?.getAttribute("href")).toBe("/media/asset-1");

    const videoThumb = screen.getByAltText("clip.mp4");
    expect(videoThumb).toBeTruthy();
    expect(screen.getByText("VIDEO")).toBeTruthy();

    expect(screen.queryByRole("heading", { name: "mountain-lake.jpg" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "clip.mp4" })).toBeNull();
    expect(screen.queryByText("512 B")).toBeNull();
    expect(screen.queryByText("2.0 KB")).toBeNull();
    expect(screen.getAllByRole("button", { name: "移入回收站" })).toHaveLength(2);

    const imageFigure = imageThumb.closest("figure");
    expect(imageFigure?.getAttribute("style")).toContain("aspect-ratio: 1");

    Object.defineProperty(imageThumb, "naturalWidth", { configurable: true, value: 1600 });
    Object.defineProperty(imageThumb, "naturalHeight", { configurable: true, value: 900 });
    fireEvent.load(imageThumb);

    await waitFor(() => {
      expect(imageFigure?.getAttribute("style")).toContain("aspect-ratio: 1.7778");
    });
  });

  it("shows pdf badge and filename caption for pdf cards", async () => {
    listMediaMock.mockResolvedValue([
      makeAsset({
        id: "asset-pdf",
        originalFilename: "manual.pdf",
        mediaType: "pdf",
        mimeType: "application/pdf",
        detailUrl: "/media/asset-pdf",
        viewUrl: "/media/asset-pdf/view",
        thumbnailUrl: "/media/asset-pdf/thumbnail",
        downloadUrl: "/media/asset-pdf/download"
      })
    ]);

    render(
      React.createElement(
        MemoryRouter,
        { initialEntries: ["/media"] },
        React.createElement(MediaListPage, {
          session: { loading: false, status: makeStatus() },
          onSessionChange: vi.fn()
        })
      )
    );

    await waitFor(() => {
      expect(listMediaMock).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByAltText("manual.pdf")).toBeTruthy();
    expect(screen.getByText("PDF")).toBeTruthy();
    expect(screen.getByText("manual.pdf")).toBeTruthy();
  });
});

describe("MediaDetailPage", () => {
  beforeEach(() => {
    getMediaMock.mockReset();
  });

  it("renders pdf assets inside an embedded document frame", async () => {
    getMediaMock.mockResolvedValue(
      makeAsset({
        id: "asset-pdf",
        originalFilename: "manual.pdf",
        mediaType: "pdf",
        mimeType: "application/pdf",
        detailUrl: "/media/asset-pdf",
        viewUrl: "/media/asset-pdf/view",
        thumbnailUrl: "/media/asset-pdf/thumbnail",
        downloadUrl: "/media/asset-pdf/download"
      })
    );

    render(
      React.createElement(
        MemoryRouter,
        { initialEntries: ["/media/asset-pdf"] },
        React.createElement(
          Routes,
          null,
          React.createElement(Route, {
            path: "/media/:id",
            element: React.createElement(MediaDetailPage, {
              session: { loading: false, status: makeStatus() },
              onSessionChange: vi.fn()
            })
          })
        )
      )
    );

    const frame = await screen.findByTestId("detail-pdf-frame");
    expect(frame.tagName).toBe("IFRAME");
    expect(frame.getAttribute("src")).toBe("/media/asset-pdf/view");
    expect(screen.getAllByText("PDF").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "下载原始文件" }).getAttribute("href")).toBe("/media/asset-pdf/download");
  });
});