import React from "react";
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";

import {
  ApiClientError,
  buildUploadConfig,
  deleteMedia,
  emptyTrash,
  formatBytes,
  getAuthStatus,
  getMedia,
  listMedia,
  listTrash,
  login,
  logout,
  permanentlyDeleteMedia,
  restoreMedia,
  type ApiAsset,
  type ApiMediaType,
  type AuthStatusResponse,
  type UploadResponse
} from "./api-client";
import { UploadIslandApp } from "./upload-island";

type SessionState = {
  loading: boolean;
  status?: AuthStatusResponse;
  error?: string;
};

export function App() {
  const [session, setSession] = React.useState<SessionState>({ loading: true });

  const refreshSession = React.useCallback(async () => {
    try {
      const status = await getAuthStatus();
      setSession({ loading: false, status });
      return status;
    } catch (error) {
      setSession({ loading: false, error: error instanceof Error ? error.message : "加载会话失败" });
      return undefined;
    }
  }, []);

  React.useEffect(() => {
    void refreshSession();
  }, [refreshSession]);

  if (session.loading) {
    return <div className="app-shell loading-state">正在加载 HomeMedia…</div>;
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/login"
          element={<LoginPage session={session} onAuthenticated={(status) => setSession({ loading: false, status })} />}
        />
        <Route
          path="/media"
          element={
            <ProtectedPage session={session}>
              <MediaListPage session={session} onSessionChange={setSession} />
            </ProtectedPage>
          }
        />
        <Route
          path="/media/:id"
          element={
            <ProtectedPage session={session}>
              <MediaDetailPage session={session} onSessionChange={setSession} />
            </ProtectedPage>
          }
        />
        <Route
          path="/trash"
          element={
            <ProtectedPage session={session}>
              <TrashPage session={session} onSessionChange={setSession} />
            </ProtectedPage>
          }
        />
        <Route path="*" element={<Navigate to={session.status?.authenticated ? "/media" : "/login"} replace />} />
      </Routes>
    </BrowserRouter>
  );
}

function ProtectedPage({ session, children }: { session: SessionState; children: React.ReactNode }) {
  if (!session.status?.authenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

function LoginPage({
  session,
  onAuthenticated
}: {
  session: SessionState;
  onAuthenticated: (status: AuthStatusResponse) => void;
}) {
  const navigate = useNavigate();
  const [username, setUsername] = React.useState("admin");
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState<string>();
  const [submitting, setSubmitting] = React.useState(false);

  if (session.status?.authenticated) {
    return <Navigate to="/media" replace />;
  }

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!session.status?.csrfToken) {
      setError("缺少登录 CSRF token");
      return;
    }

    setSubmitting(true);
    setError(undefined);
    try {
      const status = await login(username, password, session.status.csrfToken);
      onAuthenticated(status);
      navigate("/media", { replace: true });
    } catch (loginError) {
      if (loginError instanceof ApiClientError) {
        setError(loginError.message);
      } else {
        setError("登录失败");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="auth-layout">
      <section className="auth-card">
        <p className="eyebrow">HomeMedia</p>
        <h1>媒体库登录</h1>
        <form className="auth-form" onSubmit={handleSubmit}>
          <label>
            用户名
            <input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" />
          </label>
          <label>
            密码
            <input
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              type="password"
              autoComplete="current-password"
            />
          </label>
          {error && <p className="form-error">{error}</p>}
          <button type="submit" disabled={submitting}>
            {submitting ? "登录中…" : "登录"}
          </button>
        </form>
      </section>
    </main>
  );
}

export function MediaListPage({
  session,
  onSessionChange
}: {
  session: SessionState;
  onSessionChange: React.Dispatch<React.SetStateAction<SessionState>>;
}) {
  const [assets, setAssets] = React.useState<ApiAsset[]>([]);
  const [thumbnailRatios, setThumbnailRatios] = React.useState<Record<string, number>>({});
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const nextAssets = await listMedia();
        if (!cancelled) {
          setAssets(nextAssets);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "加载媒体失败");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  const handleCardMediaLoad = React.useCallback(
    (assetId: string, event: React.SyntheticEvent<HTMLImageElement>) => {
      const element = event.currentTarget;
      const width = element.naturalWidth;
      const height = element.naturalHeight;
      if (!width || !height) {
        return;
      }

      const nextRatio = Number((width / height).toFixed(4));
      setThumbnailRatios((current) => {
        if (current[assetId] === nextRatio) {
          return current;
        }

        return {
          ...current,
          [assetId]: nextRatio
        };
      });
    },
    []
  );

  const renderCardPreview = React.useCallback(
    (asset: ApiAsset) => {
      return (
        <img
          src={asset.thumbnailUrl}
          alt={asset.originalFilename}
          className="card-thumb"
          loading="lazy"
          onLoad={(event) => handleCardMediaLoad(asset.id, event)}
        />
      );
    },
    [handleCardMediaLoad]
  );

  const handleUploadResolved = (response: UploadResponse) => {
    setAssets((current) => {
      if (current.some((asset) => asset.id === response.asset.id)) {
        return current;
      }
      return [response.asset, ...current];
    });
  };

  const handleDelete = async (asset: ApiAsset) => {
    if (!session.status?.csrfToken) {
      return;
    }
    if (!window.confirm(`确定将 ${asset.originalFilename} 移入回收站吗？`)) {
      return;
    }

    try {
      await deleteMedia(asset.id, session.status.csrfToken);
      setAssets((current) => current.filter((item) => item.id !== asset.id));
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "删除失败");
    }
  };

  return (
    <PageLayout session={session} onSessionChange={onSessionChange} title="媒体库">
      {session.status && <UploadIslandApp config={buildUploadConfig(session.status)} onUploadResolved={handleUploadResolved} />}
      {error && <p className="page-error">{error}</p>}
      {loading ? (
        <div className="empty-state">正在加载媒体列表…</div>
      ) : assets.length === 0 ? (
        <div className="empty-state">还没有上传任何媒体。</div>
      ) : (
        <section className="gallery-grid">
          {assets.map((asset) => (
            <article key={asset.id} className="media-card">
              <button
                type="button"
                className="card-action"
                onClick={() => void handleDelete(asset)}
                aria-label="移入回收站"
                title="移入回收站"
              >
                <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
                  <path d="M9 3.75h6a1 1 0 0 1 1 1V6h3a.75.75 0 0 1 0 1.5h-1.02l-.82 10.21A2.25 2.25 0 0 1 14.92 20H9.08a2.25 2.25 0 0 1-2.24-2.29L6.02 7.5H5a.75.75 0 0 1 0-1.5h3V4.75a1 1 0 0 1 1-1Zm5.5 2.25v-.75h-5V6h5Zm-6.16 1.5.8 10.09a.75.75 0 0 0 .74.66h5.84a.75.75 0 0 0 .74-.66l.8-10.09H8.34Zm2.16 2.25c.41 0 .75.34.75.75v4.75a.75.75 0 0 1-1.5 0V10.5c0-.41.34-.75.75-.75Zm3 0c.41 0 .75.34.75.75v4.75a.75.75 0 0 1-1.5 0V10.5c0-.41.34-.75.75-.75Z" />
                </svg>
                <span className="sr-only">移入回收站</span>
              </button>
              {asset.mediaType === "pdf" ? (
                <a href={asset.viewUrl} className="card-link" target="_blank" rel="noreferrer noopener">
                  <figure className="card-thumb-wrap" style={{ aspectRatio: `${thumbnailRatios[asset.id] ?? 1}` }}>
                    {renderCardPreview(asset)}
                    {getMediaBadgeLabel(asset.mediaType) && <span className="card-badge">{getMediaBadgeLabel(asset.mediaType)}</span>}
                  </figure>
                  {asset.mediaType === "pdf" && (
                    <div className="card-caption" title={asset.originalFilename}>
                      {asset.originalFilename}
                    </div>
                  )}
                </a>
              ) : (
                <Link to={`/media/${asset.id}`} className="card-link">
                  <figure className="card-thumb-wrap" style={{ aspectRatio: `${thumbnailRatios[asset.id] ?? 1}` }}>
                    {renderCardPreview(asset)}
                    {getMediaBadgeLabel(asset.mediaType) && <span className="card-badge">{getMediaBadgeLabel(asset.mediaType)}</span>}
                  </figure>
                </Link>
              )}
            </article>
          ))}
        </section>
      )}
    </PageLayout>
  );
}

export function MediaDetailPage({
  session,
  onSessionChange
}: {
  session: SessionState;
  onSessionChange: React.Dispatch<React.SetStateAction<SessionState>>;
}) {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [asset, setAsset] = React.useState<ApiAsset>();
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const nextAsset = await getMedia(id);
        if (!cancelled) {
          setAsset(nextAsset);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "加载媒体详情失败");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [id]);

  const handleDelete = async () => {
    if (!asset || !session.status?.csrfToken) {
      return;
    }
    if (!window.confirm(`确定将 ${asset.originalFilename} 移入回收站吗？`)) {
      return;
    }

    try {
      await deleteMedia(asset.id, session.status.csrfToken);
      navigate("/media", { replace: true });
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "删除失败");
    }
  };

  return (
    <PageLayout session={session} onSessionChange={onSessionChange} title="媒体详情">
      {error && <p className="page-error">{error}</p>}
      {loading ? (
        <div className="empty-state">正在加载详情…</div>
      ) : !asset ? (
        <div className="empty-state">未找到媒体。</div>
      ) : asset.mediaType === "pdf" ? (
        <div className="empty-state">PDF 不提供详情页，请返回列表后直接打开原文件。</div>
      ) : (
        <section className="detail-panel">
          <div className="detail-preview">
            {asset.mediaType === "image" ? (
              <img src={asset.viewUrl} alt={asset.originalFilename} className="detail-image" />
            ) : (
              <video src={asset.viewUrl} controls className="detail-video" />
            )}
          </div>
          <div className="detail-meta">
            <p className="eyebrow">{getMediaTypeLabel(asset.mediaType)}</p>
            <h2>{asset.originalFilename}</h2>
            <p>{formatBytes(asset.sizeBytes)}</p>
            <p>{new Date(asset.createdAt).toLocaleString()}</p>
            {asset.playbackWarning && <p className="detail-warning">{asset.playbackWarning.message}</p>}
            <div className="detail-actions">
              <a href={asset.downloadUrl} className="primary-link">
                下载原始文件
              </a>
              <button type="button" onClick={() => void handleDelete()}>
                移入回收站
              </button>
            </div>
          </div>
        </section>
      )}
    </PageLayout>
  );
}

function getMediaBadgeLabel(mediaType: ApiMediaType): string | null {
  if (mediaType === "video") {
    return "VIDEO";
  }
  if (mediaType === "pdf") {
    return "PDF";
  }
  return null;
}

function getMediaTypeLabel(mediaType: ApiMediaType): string {
  if (mediaType === "image") {
    return "Image";
  }
  if (mediaType === "video") {
    return "Video";
  }
  return "PDF";
}

function TrashPage({
  session,
  onSessionChange
}: {
  session: SessionState;
  onSessionChange: React.Dispatch<React.SetStateAction<SessionState>>;
}) {
  const [assets, setAssets] = React.useState<ApiAsset[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const nextAssets = await listTrash();
        if (!cancelled) {
          setAssets(nextAssets);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "加载回收站失败");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  const csrfToken = session.status?.csrfToken ?? "";

  const handleRestore = async (asset: ApiAsset) => {
    if (!csrfToken || !window.confirm(`确定恢复 ${asset.originalFilename} 吗？`)) {
      return;
    }

    try {
      await restoreMedia(asset.id, csrfToken);
      setAssets((current) => current.filter((item) => item.id !== asset.id));
    } catch (restoreError) {
      setError(restoreError instanceof Error ? restoreError.message : "恢复失败");
    }
  };

  const handlePermanentDelete = async (asset: ApiAsset) => {
    if (!csrfToken || !window.confirm(`确定彻底删除 ${asset.originalFilename} 吗？`)) {
      return;
    }

    try {
      await permanentlyDeleteMedia(asset.id, csrfToken);
      setAssets((current) => current.filter((item) => item.id !== asset.id));
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "彻底删除失败");
    }
  };

  const handleEmpty = async () => {
    if (!csrfToken || !window.confirm("确定清空回收站吗？")) {
      return;
    }

    try {
      await emptyTrash(csrfToken);
      setAssets([]);
    } catch (emptyError) {
      setError(emptyError instanceof Error ? emptyError.message : "清空回收站失败");
    }
  };

  return (
    <PageLayout session={session} onSessionChange={onSessionChange} title="回收站">
      <div className="page-toolbar">
        <button type="button" onClick={() => void handleEmpty()} disabled={assets.length === 0}>
          清空回收站
        </button>
      </div>
      {error && <p className="page-error">{error}</p>}
      {loading ? (
        <div className="empty-state">正在加载回收站…</div>
      ) : assets.length === 0 ? (
        <div className="empty-state">回收站为空。</div>
      ) : (
        <div className="trash-list">
          {assets.map((asset) => (
            <article key={asset.id} className="trash-item">
              <img src={asset.thumbnailUrl} alt={asset.originalFilename} className="trash-thumb" loading="lazy" />
              <div className="trash-meta">
                <h2>{asset.originalFilename}</h2>
                <p>{asset.deletedAt ? new Date(asset.deletedAt).toLocaleString() : "已删除"}</p>
              </div>
              <div className="trash-actions">
                <button type="button" onClick={() => void handleRestore(asset)}>
                  恢复
                </button>
                <button type="button" className="danger" onClick={() => void handlePermanentDelete(asset)}>
                  彻底删除
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </PageLayout>
  );
}

function PageLayout({
  session,
  onSessionChange,
  title,
  children
}: {
  session: SessionState;
  onSessionChange: React.Dispatch<React.SetStateAction<SessionState>>;
  title: string;
  children: React.ReactNode;
}) {
  const navigate = useNavigate();
  const location = useLocation();

  const handleLogout = async () => {
    const csrfToken = session.status?.csrfToken;
    if (!csrfToken) {
      return;
    }

    try {
      await logout(csrfToken);
      const nextStatus = await getAuthStatus();
      onSessionChange({ loading: false, status: nextStatus });
      navigate("/login", { replace: true });
    } catch (logoutError) {
      onSessionChange((current) => ({
        ...current,
        error: logoutError instanceof Error ? logoutError.message : "退出失败"
      }));
    }
  };

  return (
    <main className="app-shell">
      <header className="page-header">
        <div>
          <p className="eyebrow">HomeMedia</p>
          <h1>{title}</h1>
        </div>
        <div className="header-actions">
          {location.pathname !== "/media" && (
            <Link to="/media" className="primary-link subtle-link">
              媒体库
            </Link>
          )}
          {location.pathname !== "/trash" && (
            <Link to="/trash" className="primary-link subtle-link">
              回收站
            </Link>
          )}
          <button type="button" onClick={() => void handleLogout()}>
            退出登录
          </button>
        </div>
      </header>
      {children}
    </main>
  );
}
