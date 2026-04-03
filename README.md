# HomeMedia

一个最小可用的私有照片和视频上传/查看/下载项目。

当前版本聚焦 MVP，只提供四条核心链路：
- 上传图片和视频
- 浏览媒体列表
- 查看单个媒体详情
- 下载原始文件

## 技术栈

- Go 1.24
- Gin
- PostgreSQL 16
- 本地文件存储
- Docker Compose

## 目录说明

- `cmd/server`：应用入口
- `internal/config`：环境变量配置
- `internal/media`：媒体领域模型和业务服务
- `internal/http`：Gin 路由和处理器
- `internal/repository/postgres`：PostgreSQL 仓储实现
- `internal/storage/local`：本地文件存储实现
- `migrations`：数据库迁移脚本
- `web/templates`：服务端渲染页面

## 快速开始

1. 复制环境变量文件：

```bash
cp .env.example .env
```

2. 启动开发环境：

```bash
docker compose up --build
```

3. 打开浏览器：

```text
http://127.0.0.1:8080/media
```

## 常用命令

运行测试：

```bash
docker compose run --rm app go test ./...
```

格式化代码：

```bash
docker compose run --rm app sh -c 'gofmt -w ./cmd ./internal'
```

单独执行迁移：

```bash
docker compose run --rm migrate
```

停止环境：

```bash
docker compose down
```

## 当前能力边界

当前不包含这些功能：
- 鉴权和多用户
- 对象存储
- 缩略图或转码
- 搜索、标签、相册
- 分享链接
- 异步任务系统

## 开发说明

- PostgreSQL 只存元数据，不存文件二进制。
- 原始文件保存到本地挂载目录。
- 当前开发环境以 Docker Compose 为准。
- 宿主机如果没有 Go 工具链，可以直接通过容器执行测试和格式化。
