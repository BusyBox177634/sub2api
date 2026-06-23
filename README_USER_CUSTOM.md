# user_custom 分支新增功能说明

本文档记录当前 `user_custom` 分支相对 `main` 分支新增或明显改变的用户可见功能、接口、数据迁移和部署影响。纯内部的测试、依赖注入、代码生成和重构细节不在这里展开。

## 新增功能概览

### 网页端聊天

- 新增网页端 `/chat` 聊天页面，可在后台侧边栏进入。
- 支持选择当前用户可用的 API Key，并按 API Key 拉取可用模型。
- 支持创建、切换、重命名和删除聊天会话。
- 支持流式回复，前端会实时渲染助手输出。
- 支持图片附件上传、删除、粘贴和拖拽上传，主要面向支持图片输入的模型。
- 聊天会话、消息、附件元数据会持久化到数据库，刷新页面后仍可查看历史会话。

### 使用记录详情

- 管理员后台和普通用户后台的使用记录增加“查看详情”入口。
- 新记录会把 API 网关收到的原始请求体，以及返回给客户端的响应体，随 usage log 写入 `usage_log_details`。
- 详情弹窗提供消息视图、请求 JSON 和响应 JSON，便于排查实际请求内容。
- 普通用户只能查看自己的使用记录详情；管理员可查看全局使用记录详情。
- 管理员后台和普通用户后台的使用记录列表新增“请求内容关键字”过滤，可按请求 JSON 内容模糊查找具体请求。
- 历史 usage log 无法补回请求/响应详情，可能显示 `historical` 或 `not_captured`。

### 用户资料限制

- 普通用户资料页中的用户名改为只读展示，不再允许用户自行修改用户名。
- 后端也拒绝普通用户通过资料更新接口提交用户名变更。
- 管理员仍可在后台用户管理中修改用户名。
- 可信注册或身份同步流程仍可通过专用后端路径写入用户名，不走普通资料页更新逻辑。

### 本地构建与部署优化

- `deploy/docker-compose.local.yml` 改为从本地源码构建 Sub2API 应用镜像，不再拉取远端 `weishaw/sub2api` 应用镜像。
- `build.context` 保持为 `..`，`dockerfile` 使用仓库根目录的 `Dockerfile`。
- 本地构建基础镜像参数改用国内 registry，降低 Docker Hub metadata 拉取失败对本地构建的影响。
- Dockerfile 支持国内 Alpine、npm/pnpm 和 Go proxy 镜像源参数。

## 接口与数据变更

### 聊天 API

新增用户态聊天接口，均需要登录认证：

- `GET /api/v1/chat/api-keys`：列出可用于聊天的 API Key。
- `GET /api/v1/chat/models?api_key_id=...`：按 API Key 拉取模型列表。
- `GET /api/v1/chat/conversations`：列出会话。
- `POST /api/v1/chat/conversations`：创建会话。
- `PATCH /api/v1/chat/conversations/:id`：更新会话标题、API Key 或模型。
- `DELETE /api/v1/chat/conversations/:id`：删除会话。
- `GET /api/v1/chat/conversations/:id/messages`：列出会话消息。
- `POST /api/v1/chat/conversations/:id/messages/stream`：发送消息并流式返回助手回复。
- `POST /api/v1/chat/conversations/:id/attachments`：上传附件。
- `DELETE /api/v1/chat/attachments/:attachmentID`：删除附件。

### 使用记录详情 API

- `GET /api/v1/usage/:id/detail`：普通用户查看自己的单条使用记录详情。
- `GET /api/v1/admin/usage/:id/detail`：管理员查看任意使用记录详情。

使用记录列表接口新增查询参数：

- `GET /api/v1/usage?content_keyword=...`：普通用户按自己的请求内容关键字过滤使用记录。
- `GET /api/v1/admin/usage?content_keyword=...`：管理员按全局请求内容关键字过滤使用记录。

`content_keyword` 只匹配 `usage_log_details.request_payload_json`，不匹配 `response_payload_json`。

详情响应会包含：

- `available`：详情是否可用。
- `reason`：不可用原因，例如 `historical`、`not_captured` 或 `disabled`。
- `request_messages` / `response_messages`：从请求和响应中解析出的消息内容。
- `request_payload_json` / `response_payload_json`：脱敏后的原始 JSON 文本。
- `request_truncated` / `response_truncated`：是否发生截断。

### 数据库迁移

新增迁移文件：

- `backend/migrations/082_add_chat_tables.sql`：新增聊天会话、消息和附件表。
- `backend/migrations/083_add_chat_message_tool_fields.sql`：为聊天消息增加工具事件和引用字段。
- `backend/migrations/084_add_usage_log_details.sql`：新增 `usage_log_details` 表，用于保存 usage log 的请求/响应详情。
- `backend/migrations/154_enable_pg_trgm.sql`：启用 PostgreSQL `pg_trgm` 扩展。
- `backend/migrations/155_usage_log_details_request_payload_trgm_notx.sql`：为 `usage_log_details.request_payload_json` 创建 trigram GIN 索引，加速请求内容关键字模糊查询。

## 行为说明与限制

- 使用记录详情只保证新产生的记录可捕获详情；合并前或迁移前的历史记录不会自动补齐。
- 请求内容关键字过滤依赖 `usage_log_details.request_payload_json`，因此同样只对已捕获详情的新记录有效。
- 请求内容关键字过滤只查请求体，不查响应体，避免按返回内容扩大匹配范围。
- 请求体和响应体保留已有敏感字段脱敏逻辑。
- 当前分支去掉了原有 64KB 应用层截断，目标是完整保存新记录的请求和响应内容。
- 如果详情接口请求失败或返回 404，前端会提示加载失败并关闭详情弹窗，不会误显示为“详情不可用”。
- 只有详情接口正常返回 `available=false` 时，前端才显示“详情不可用”及其历史/未捕获原因。
- 管理员使用记录清理弹窗不显示“请求内容关键字”过滤；清理任务仍按时间、用户、API Key、账号、分组、模型、请求类型和计费类型等现有条件执行。
- 聊天附件当前主要用于图片输入；不支持图片输入的模型会阻止图片上传。
- 普通用户不能通过个人资料页或资料更新 API 修改用户名。

## 部署注意事项

- 本地部署仍使用现有数据目录和数据库卷，不需要删除已有容器数据。
- `deploy/docker-compose.local.yml` 中 `sub2api` 使用本地源码构建：
  - `build.context: ..`
  - `dockerfile: Dockerfile`
- 本地构建基础镜像默认使用国内 registry 参数：
  - `NODE_IMAGE`
  - `GOLANG_IMAGE`
  - `ALPINE_IMAGE`
- `POSTGRES_IMAGE` 保持为 `postgres:18-alpine`。
- 构建后可用以下命令确认镜像来源和运行版本：

```bash
docker compose -f deploy/docker-compose.local.yml config --images
docker exec sub2api /app/sub2api --version
```

如果后台页面仍显示旧版本，优先强刷页面或退出重进后台，因为前端版本状态可能存在浏览器缓存。
