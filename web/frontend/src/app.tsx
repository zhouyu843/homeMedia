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
        <p className="auth-copy">前端已切换为 React + TypeScript，认证与会话仍由 Go 负责。</p>
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

function MediaListPage({
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
    <PageLayout session={session} onSessionChange={onSessionChange} title="媒体库" subtitle="上传、预览、下载与删除都由 API 驱动。">
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
              <button type="button" className="card-action" onClick={() => void handleDelete(asset)}>
                回收
              </button>
              <Link to={asset.detailUrl} className="card-link">
                <figure className="card-thumb-wrap">
                  <img src={asset.thumbnailUrl} alt={asset.originalFilename} className="card-thumb" loading="lazy" />
                  {asset.mediaType === "video" && <span className="card-badge">VIDEO</span>}
                </figure>
                <div className="card-body">
                  <h2>{asset.originalFilename}</h2>
                  <p>{formatBytes(asset.sizeBytes)}</p>
                </div>
              </Link>
            </article>
          ))}
        </section>
      )}
    </PageLayout>
  );
}

function MediaDetailPage({
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
    <PageLayout session={session} onSessionChange={onSessionChange} title="媒体详情" subtitle="保留原始文件流接口，页面由 React 渲染。">
      {error && <p className="page-error">{error}</p>}
      {loading ? (
        <div className="empty-state">正在加载详情…</div>
      ) : !asset ? (
        <div className="empty-state">未找到媒体。</div>
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
            <p className="eyebrow">{asset.mediaType === "image" ? "Image" : "Video"}</p>
            <h2>{asset.originalFilename}</h2>
            <p>{formatBytes(asset.sizeBytes)}</p>
            <p>{new Date(asset.createdAt).toLocaleString()}</p>
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
    <PageLayout session={session} onSessionChange={onSessionChange} title="回收站" subtitle="恢复、彻底删除和清空操作全部走 JSON API。">
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
  subtitle,
  children
}: {
  session: SessionState;
  onSessionChange: React.Dispatch<React.SetStateAction<SessionState>>;
  title: string;
  subtitle: string;
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
          <p className="page-subtitle">{subtitle}</p>
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
