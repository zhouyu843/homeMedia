# HomeMedia

一个以长期维护为导向的私有照片和视频管理项目。

当前版本已经移除服务端模板渲染页面，前端统一为 React + TypeScript 单页应用，后端专注于认证、JSON API、媒体文件流、缩略图和本地存储。项目仍保持单体部署：Go 作为唯一运行时入口，负责托管前端构建产物。

## 当前能力

- 管理员登录与登出
- React SPA 登录页、媒体列表页、详情页、回收站页
- 图片、视频和 PDF 上传
- 列表页整页拖拽上传
- 缩略图优先的相册式媒体总览，按原始横竖比例同高展示，单张宽度不超过容器半宽；视频卡片在列表页显示静态首帧缩略图；PDF 使用第一页真实缩略图
- 上传时自动生成压缩预览：图片压缩为 1280px 宽 JPEG，视频压缩为 720p H.264 MP4
- 详情页默认展示压缩预览，提供「查看原始图片/视频」按钮可在新标签打开原始文件，也可直接下载原始文件
- 视频详情页会在检测到 HEVC/H.265 编码时显示兼容性提示，便于排查 Linux Chrome 只有声音没有画面的场景
- 原始文件下载
- 逻辑删除、恢复、彻底删除、清空回收站
- 基于文件内容 SHA-256 的精确去重

## 架构概览

前端：
- React 18
- TypeScript
- Vite
- React Router

后端：
- Go 1.24
- Gin
- PostgreSQL 16
- 本地文件存储
- Docker Compose

职责分界：
- React 负责页面渲染、路由切换、上传交互、列表与详情展示。
- Go 负责 Cookie Session、CSRF、限流、媒体领域逻辑、JSON API、预览/下载/缩略图文件流、上传时同步生成并持久化压缩预览与缩略图、前端静态资源托管。
- PostgreSQL 只保存元数据；原始文件固定保存在挂载目录。

## 路由说明

SPA 页面路由：
- `GET /login`
- `GET /media`
- `GET /media/:id`（仅图片和视频详情页）
- `GET /trash`

这些路由中，`/login`、`/media`、`/trash` 与图片/视频的 `/media/:id` 都返回前端 SPA 入口文件，由 React Router 接管页面渲染。PDF 不再提供 `/media/:id` 详情页。

文件流、预览与缩略图路由：
- `GET /media/:id/view`（原始文件）
- `GET /media/:id/preview`（压缩预览；无预览时 302 跳转到 `/view`）
- `GET /media/:id/download`
- `GET /media/:id/thumbnail`
- `GET /trash/:id/thumbnail`

说明：
- 上传时同步生成压缩预览（图片：1280px JPEG；视频：720p H.264 MP4）并持久化保存，PDF 跳过压缩预览。
- 详情页默认加载 `/preview`，「查看原始」按钮指向 `/view`（新标签）。
- 缩略图统一输出 JPEG，并在上传时优先持久化保存；历史资源或生成失败的场景会在首次请求时按需生成并懒回填。
- PDF 缩略图由服务端提取第一页；提取失败时自动回退为占位图。

JSON API：
- `GET /api/auth/status`
- `POST /api/login`
- `POST /api/logout`
- `GET /api/media`
- `GET /api/media/:id`
- `GET /api/trash`
- `POST /api/uploads`
- `POST /api/media/:id/delete`
- `POST /api/media/:id/restore`
- `POST /api/media/:id/permanent-delete`
- `POST /api/trash/empty`

## 鉴权与安全

- 页面和 API 继续使用 Go 侧 Cookie Session。
- 登录前通过 `GET /api/auth/status` 获取登录 CSRF token。
- 已登录状态下通过同一接口获取会话 CSRF token。
- 上传、登出、删除、恢复、彻底删除、清空回收站都要求有效会话与 CSRF token。
- 登录与上传接口仍保留 IP 限流。
- 预览、下载和缩略图接口同样受登录态保护，PDF 新窗口查看也复用同一受保护文件流。

## 上传与去重行为

- 上传时基于二进制内容计算 SHA-256 做精确去重。
- 当前支持的主要类型包括常见图片、视频以及 `application/pdf`。
- 前端拖拽上传时，如果浏览器没有给 PDF 文件提供 MIME type，会按 `.pdf` 扩展名先加入上传队列，最终仍以后端内容探测结果为准。
- 内容相同但文件名不同：复用已有媒体资源，不重复保存物理文件。
- 文件名相同但内容不同：视为不同媒体，允许共存。
- `POST /api/uploads`：
  - 新建资源返回 `201`
  - 命中活跃重复内容返回 `200` 且 `existing=true`
  - 命中回收站重复内容返回 `409` 且 `code=trashed_duplicate`
- 前端上传面板支持在命中回收站重复内容时选择“恢复旧项”或“继续新建”。
- 历史数据如果尚未写入 `content_hash`，系统会在后续遇到同大小文件上传时尝试按内容懒匹配并回填哈希。

## 删除与回收站行为

- 删除操作为逻辑删除，先移入回收站，不立即删除宿主机原始文件。
- 已删除媒体不会再出现在主列表，也不能通过详情、预览、下载和主缩略图接口访问。
- 回收站支持恢复、单项彻底删除、清空回收站。
- 彻底删除时，如果该物理文件没有被其他活跃记录引用，会同时删除宿主机文件。
- 如果还有其他活跃记录复用同一个 `storage_path`，只删除当前回收站记录，不删除共享物理文件。

## 目录说明

- `cmd/server`：应用入口
- `cmd/backfill-previews`：历史媒体压缩预览批量生成命令
- `internal/config`：环境变量配置
- `internal/http`：Gin 路由、认证、API handler
- `internal/media`：媒体领域模型与业务服务
- `internal/repository/postgres`：PostgreSQL 仓储实现
- `internal/storage/local`：本地文件存储实现
- `migrations`：数据库迁移脚本
- `web/frontend`：React + TypeScript 前端工程
- `web/static`：Gin 托管的静态资源与前端构建产物
- `web/static/app`：Vite SPA 构建输出目录
- `e2e`：Playwright E2E 测试工程

运行依赖补充：
- 缩略图与压缩预览均依赖 `ffmpeg` 处理图片/视频，并默认持久化到本地存储。
- PDF 首页缩略图依赖 `pdftoppm`（`poppler-utils`）。

## 历史数据压缩预览回填

已有文件默认没有压缩预览，可通过以下命令批量生成：

```bash
make backfill-previews
```

该命令会遍历所有尚无 `preview_storage_path` 的活跃媒体记录（PDF 跳过），依次调用 ffmpeg 生成并保存压缩预览，期间会打印进度和最终统计。视频文件较大时耗时较长，建议在低峰时段运行。

## E2E 测试

E2E 测试使用 Playwright，测试代码在独立的 `e2e/` 目录，通过 Docker Compose `e2e` service 运行（基于官方 Playwright 镜像），不影响日常开发启动。

### 覆盖场景

- 登录 / 登出 / 未登录重定向
- 文件上传（含去重）
- 媒体列表加载
- 媒体详情页
- 删除 / 回收站 / 恢复 / 彻底删除

### 运行方式

先确保开发栈已启动：

```bash
docker compose up -d
```

再运行 E2E 测试（Makefile 快捷方式）：

```bash
make e2e
```

等价命令：

```bash
docker compose --profile e2e run --rm e2e
```

### 配置说明

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `BASE_URL` | `http://app:8080` | 测试目标地址（容器内 app service） |
| `ADMIN_USERNAME` | 读自 `.env` | 管理员用户名 |
| `ADMIN_PASSWORD` | 读自 `.env` | 管理员密码 |

Playwright 报告会生成到 `e2e/playwright-report/`（已加入 `.gitignore`）。

## 快速开始

1. 复制环境变量文件：

```bash
cp .env.example .env
```

2. 创建上传目录：

```bash
mkdir -p data/uploads
```

3. 启动开发环境：

```bash
docker compose up --build
```

4. 安装前端依赖并构建 SPA：

```bash
make frontend-install
make frontend-build
```

5. 打开浏览器：

```text
http://127.0.0.1:8018/login
```

6. 使用 `.env` 中的管理员账号密码登录。

说明：
- Go 服务会托管 `web/static/app` 下的前端构建产物。
- 如果修改了前端代码，需要重新执行 `make frontend-build`。
- 前端开发服务器默认端口为 `5175`，映射为 `5175:5175`。

## 开发工作流

常见增量命令：

```bash
docker compose run --rm migrate
docker compose restart app
docker compose run --rm app go test ./...
make frontend-test
make frontend-build
```

适用场景：
- 修改数据库 migration：执行 `docker compose run --rm migrate`
- 修改 Go 后端代码：执行 `docker compose restart app`
- 修改 React/TypeScript 前端代码：执行 `make frontend-build`
- 修改 Go 依赖、Dockerfile、系统包或基础镜像：执行 `docker compose up -d --build`

## 常用命令

运行后端测试：

```bash
docker compose run --rm app go test ./...
```

运行前端测试：

```bash
make frontend-test
```

为现有视频补齐缺失缩略图：

```bash
make backfill-video-thumbnails
```

安装前端依赖：

```bash
make frontend-install
```

构建前端：

```bash
make frontend-build
```

启动前端开发服务器：

```bash
make frontend-dev
```

单独执行迁移：

```bash
docker compose run --rm migrate
```

进入 PostgreSQL 容器：

```bash
docker compose exec postgres sh
```

进入 PostgreSQL 命令行：

```bash
docker compose exec postgres sh -lc 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

停止环境：

```bash
docker compose down
```

说明：当前 `app` 容器内未验证 `gofmt` 可用，因此不要默认依赖 `make fmt` 风格的容器化格式化步骤，优先以测试和构建通过为准，必要时再补齐工具链。

端口约定：
- 本地开发对外固定使用 `8018` 访问应用。
- 本地开发对外固定使用 `51731` 访问 PostgreSQL，容器内也监听 `51731`。

## 环境变量

基础运行配置：
- `DATABASE_URL`：PostgreSQL 连接串
- `MAX_UPLOAD_SIZE_MB`：上传大小上限（MB），默认 `2048`（2GB）

认证与会话配置：
- `ADMIN_USERNAME`：管理员用户名，默认 `admin`
- `ADMIN_PASSWORD`：管理员密码，必填
- `SESSION_SECRET`：会话签名密钥，必填
- `SESSION_TTL_HOURS`：会话有效时长，默认 `24`

## 当前能力边界

当前不包含这些功能：
- 多用户账号体系
- 开放注册和找回密码
- 对象存储
- 搜索、标签、相册
- 分享链接
- 异步任务系统

## 安全说明

- 已启用基础会话认证、CSRF 校验和登录/上传接口限流。
- 已移除详情页中的内部存储路径展示。
- 推荐通过 Tailscale、ZeroTier 或受控反向代理访问，不建议直接裸露到公网。

## 开发说明

- PostgreSQL 只存元数据，不存文件二进制。
- 原始文件固定保存到宿主机 `./data/uploads/`，容器内路径为 `/data/uploads`。
- 应用容器包含 `ffmpeg`，用于生成图片和视频缩略图。
- 当前开发环境以 Docker Compose 为准。

## 生产部署

生产环境继续使用 Docker Compose，但使用单独的生产配置和部署脚本：

- 生产镜像会在 Docker build 阶段完成前端 SPA 构建，并编译 Go 二进制。
- 运行时容器不再挂载源码，也不再使用 `go run`。
- 上传文件仍保存在宿主机 `./data/uploads/`，数据库数据保存在 Docker volume `postgres_data`。
- 开发环境对外固定使用 `8018`，生产环境对外固定使用 `8118`。
- 开发环境对外固定使用 `51731` 访问 PostgreSQL。
- 生产环境 PostgreSQL 仅在 Compose 内部网络监听 `51731`，不再映射宿主机端口；`app` 和 `migrate` 通过 `postgres:51731` 访问数据库。
- 生产默认端口为 `8118`，对外访问地址示例：`http://127.0.0.1:8118/login`。

部署步骤：

1. 准备环境变量文件：

```bash
cp .env.production.example .env.production
```

2. 至少修改这些生产值：

- `ADMIN_PASSWORD`
- `SESSION_SECRET`
- `POSTGRES_PASSWORD`
- `DATABASE_URL`

3. 执行部署脚本：

```bash
./scripts/deploy.sh
```

脚本会自动完成这些步骤：

- 检查 `.env.production` 是否存在
- 如果当前目录是干净的 git 工作区，则执行 `git pull --ff-only`
- 如果 git 不可用、当前目录不是 git 工作区，或工作区有未提交改动，则跳过 `git pull` 并打印警告
- 启动 PostgreSQL
- 执行数据库迁移
- 构建生产镜像并启动应用
- 检查应用容器是否成功进入运行态
- 输出当前服务状态

如果启动失败，脚本会自动输出：

- `docker compose ps`
- `app` 最近 100 行日志
- `postgres` 最近 50 行日志

常用生产维护命令：

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml logs -f app
docker compose --env-file .env.production -f docker-compose.prod.yml restart app
docker compose --env-file .env.production -f docker-compose.prod.yml down
```
- 宿主机如果没有 Go 工具链，可以通过容器执行测试和前端命令。
