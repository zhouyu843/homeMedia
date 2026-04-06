# HomeMedia

一个最小可用的私有照片和视频上传/查看/下载项目。

当前版本已增加基础登录保护，媒体相关页面和上传/下载接口都需要登录后访问。

当前版本聚焦 MVP，只提供几条核心链路：
- 上传图片和视频
- 列表页整页拖拽上传
- 浏览媒体列表
- 列表缩略图预览（图片和视频）
- 查看单个媒体详情并直接预览图片/视频
- 下载原始文件
- 精确内容去重（基于文件内容 SHA-256）
- 登录/登出（单管理员账号）

详情页行为说明：
- 访问 `/media/:id` 进入详情页，页面内嵌预览图片或视频。
- 访问 `/media/:id/thumbnail` 获取媒体缩略图（JPEG）。
- 访问 `/media/:id/download` 下载原始文件。

删除媒体行为说明：
- 列表页和详情页都提供“移入回收站”入口，提交后会先做逻辑删除，不会立即移除宿主机原始文件。
- 已删除媒体不会再出现在主列表，也不能通过详情、预览、下载和缩略图接口继续访问。
- 回收站页面位于 `GET /trash`，支持恢复、单项彻底删除、清空回收站；这三种操作都会先弹出确认对话框，取消后不会提交。
- 单项彻底删除或清空回收站时，若该媒体文件没有被其他活跃记录引用，会同时删除宿主机上的原始文件。
- 为兼容历史数据，如果还有其他活跃记录复用同一个 `storage_path`，本次只删除当前回收站记录，不删除共享物理文件。
- 删除操作需要已登录会话，并通过会话 CSRF token 校验。
- 自动过期清理和定时回收任务暂未实现，当前由用户显式管理回收站。

上传去重行为说明：
- 上传时会基于文件二进制内容计算 SHA-256 做精确去重，不按文件名判重。
- 内容相同但文件名不同：复用已有媒体资源，不重复保存物理文件。
- 文件名相同但内容不同：视为不同媒体，允许共存。
- JSON 上传接口 `POST /api/uploads` 在新建时返回 `201`，命中重复内容时返回 `200`，响应体会包含 `existing` 布尔字段。
- 如果相同内容的资源已经在回收站中，React 上传增强会收到 `409` + `trashed_duplicate`，并提示用户选择“恢复旧项”或“继续新建”。
- 普通表单上传不做这一步交互；命中回收站同内容时默认按“继续新建”处理。
- 前端上传增强区域会把“新上传成功”和“文件已存在，已复用”区分展示；命中已存在资源时不会重复插入列表卡片。
- 前端上传增强区域在命中回收站同内容时，会把“等待选择”“已恢复旧项”与普通上传成功分开反馈。
- 前端上传增强区域在上传成功后会自动把成功项目从待处理列表移除，失败项目保留以便重试。
- 拖拽加入的待处理文件列表按单项单行展示，便于连续检查每个文件状态。
- 历史数据如果尚未写入 `content_hash`，系统会在后续遇到同大小文件上传时尝试按内容懒匹配并回填哈希。

修改代码后的生效步骤：
- 如果容器还没启动，直接执行 `docker compose up --build` 即可；服务会按当前代码和配置启动。
- 如果开发环境已经在运行，Go 代码、模板、migration 等修改通常不需要重建 Docker 镜像，因为 `app` 服务通过 `./:/app` 挂载了本地代码。
- 改了数据库 migration 后，先执行 `docker compose run --rm migrate`。
- 改了 Go 后端代码后，执行 `docker compose restart app` 让应用进程重新启动并加载最新代码。
- 改了前端静态资源或 React 岛屿代码后，需要重新构建前端资源，例如执行 `make frontend-build`。
- 改了 Go 依赖、Dockerfile、系统包或基础镜像后，执行 `docker compose up -d --build` 重新构建并启动容器。
- 变更完成后，建议执行 `docker compose run --rm app go test ./...`，前端改动可额外执行 `make frontend-test` 做回归验证。

常见的增量更新命令：

```bash
docker compose run --rm migrate
docker compose restart app
docker compose run --rm app go test ./...
```

鉴权行为说明：
- 访问 `/login` 打开登录页。
- 访问 `/media`、`/trash`、`/media/:id`、`/media/:id/view`、`/media/:id/thumbnail`、`/media/:id/download`、`/uploads` 需要已登录会话。
- 前端上传增强接口：`GET /api/media`、`POST /api/uploads`（同样需要已登录会话）。
- 退出登录使用 `POST /logout`。

## 技术栈

- Go 1.24
- Gin
- PostgreSQL 16
- 本地文件存储
- Docker Compose
- React 18（局部交互岛屿）
- TypeScript + Vite（前端子工程构建）

## 目录说明

- `cmd/server`：应用入口
- `internal/config`：环境变量配置
- `internal/media`：媒体领域模型和业务服务
- `internal/http`：Gin 路由和处理器
- `internal/repository/postgres`：PostgreSQL 仓储实现
- `internal/storage/local`：本地文件存储实现
- `migrations`：数据库迁移脚本
- `web/templates`：服务端渲染页面
- `web/frontend`：React + TypeScript 前端子工程（上传交互增强）
- `web/static`：Gin 托管的静态资源（包含前端构建产物）

## 快速开始

1. 复制环境变量文件：

```bash
cp .env.example .env
```

2. 上传文件会保存到项目根目录下的 `data/uploads/`，目录不存在时请先创建：

```bash
mkdir -p data/uploads
```

3. 启动开发环境：

```bash
docker compose up --build
```

4. 打开浏览器：

```text
http://127.0.0.1:8080/login
```

5. 使用 `.env` 中的管理员账号密码登录。

6. （可选）构建 React 岛屿前端资源：

```bash
make frontend-install
make frontend-build
```

说明：列表页会尝试加载 `/static/react/upload-island.js`。如果未执行前端构建，核心 SSR 功能仍可用，但上传区 React 增强不会生效。
前端开发服务器端口默认为 `5175`，对应容器端口映射 `5175:5175`。

## 环境变量

基础运行配置：
- `APP_PORT`：服务端口（默认 `8080`）
- `DATABASE_URL`：PostgreSQL 连接串
- `MAX_UPLOAD_SIZE_MB`：上传大小上限（MB）

认证与会话配置：
- `ADMIN_USERNAME`：管理员用户名（默认 `admin`）
- `ADMIN_PASSWORD`：管理员密码（必填）
- `SESSION_SECRET`：会话签名密钥（必填）
- `SESSION_TTL_HOURS`：会话有效时长（小时，默认 `24`）

## 常用命令

运行测试：

```bash
docker compose run --rm app go test ./...
```

运行前端测试：

```bash
make frontend-test
```

安装前端依赖：

```bash
make frontend-install
```

构建前端资源：

```bash
make frontend-build
```

启动前端开发服务器（容器内）：

```bash
make frontend-dev
```

格式化代码：

```bash
docker compose run --rm app sh -c 'gofmt -w ./cmd ./internal'
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

## 当前能力边界

当前不包含这些功能：
- 多用户账号体系
- 开放注册和找回密码
- 对象存储
- 搜索、标签、相册
- 分享链接
- 异步任务系统

前端集成边界（当前阶段）：
- 仍以 Go + SSR 为主体，不做整站 SPA。
- React 仅用于高交互区域（当前从列表页上传区开始）。
- 已支持上传岛屿增强版：多文件选择、拖拽高亮、图片本地预览/视频占位、客户端类型/大小校验、逐文件上传状态、总体进度、失败重试、上传成功后列表即时插入、最近上传结果反馈、命中回收站重复内容时的恢复/继续新建二选一。
- 列表页支持将文件拖到整个页面范围内触发上传，成功项会自动退出待处理队列。
- 会话认证、CSRF、防刷限流仍由后端负责。

## 安全说明（当前版本）

- 已启用基础会话认证、CSRF 校验（登录/上传/登出表单）和登录/上传接口限流。
- 已移除详情页中的内部存储路径展示。
- 推荐通过 Tailscale/ZeroTier 或受控反向代理访问，不建议直接裸露到公网。

## 开发说明

- PostgreSQL 只存元数据，不存文件二进制。
- 原始文件固定保存到宿主机的 `./data/uploads/`，容器内固定路径为 `/data/uploads`。
- 应用容器包含 `ffmpeg`，用于生成图片和视频缩略图。
- 当前开发环境以 Docker Compose 为准。
- 宿主机如果没有 Go 工具链，可以直接通过容器执行测试和格式化。
