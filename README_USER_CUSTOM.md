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
- 管理员后台和普通用户后台的使用记录列表不再提供“请求内容关键字”过滤；旧客户端传入 `content_keyword` 会被后端忽略。
- 历史 usage log 无法补回请求/响应详情，可能显示 `historical` 或 `not_captured`。

### 用户资料限制

- 普通用户资料页中的用户名改为只读展示，不再允许用户自行修改用户名。
- 后端也拒绝普通用户通过资料更新接口提交用户名变更。
- 管理员仍可在后台用户管理中修改用户名。
- 可信注册或身份同步流程仍可通过专用后端路径写入用户名，不走普通资料页更新逻辑。

### 运维监控磁盘容量

- 管理员“运维监控”的系统健康区域新增磁盘容量卡片。
- 后端会采集应用进程可见的多个文件系统挂载点，记录总容量、已用空间、可用空间和使用率。
- 磁盘卡片优先展示应用数据目录（默认 `/app/data`，或 `DATA_DIR` 指定目录）的容量口径，并可打开明细查看已采集挂载点。
- Docker Desktop/macOS 下，应用数据目录可能显示为 `fakeowner`、`/run/host_mark/...` 等主机绑定挂载；该数据目录会保留展示，并使用 `df -PB1` 口径校正容量，其他无关主机桥接挂载会被过滤。
- Ubuntu Linux 等普通部署下，`overlay`、`ext4`、`xfs` 和常规 bind mount 继续使用默认 `statfs`/gopsutil 口径。
- 该功能仅用于展示，不新增磁盘告警规则，也不改变现有 CPU、内存、数据库和 Redis 告警逻辑。

### 运维快捷监控

- 管理员“系统设置 > 功能开关”新增“运维快捷监控”，默认关闭。
- 开启后管理员需要配置一个单段地址后缀，例如 `ABCD1234`；配置成功后可通过 `/{suffix}` 访问无需登录的只读后台页面，并默认进入 `/{suffix}/dashboard`。
- 地址后缀只允许 4-64 位字母、数字、短横线和下划线；`admin`、`api`、`login`、`usage` 等现有路由或保留路径不能使用。
- 快捷入口包含只读的仪表盘、运维监控、用户管理、分组管理、订阅管理、使用记录和用量简报页面。
- 快捷入口不复用管理员鉴权，也不暴露新增、编辑、删除、禁用、清理、触发生成、队列管理、AI 配置等写操作。
- 快捷监控整套页面复用原管理员后台的背景、侧边栏和内容密度，并支持中英文切换、深色模式和侧边栏收起/展开。
- 快捷用户、分组、订阅、使用记录和运维监控页面尽量复用原管理员后台的表格密度、筛选区、统计列和详情/预览入口，只删除会改变数据的操作入口。
- 快捷分组管理保留可用账号、限流账号、总账号、容量/限流、今日用量和总用量等关键列。
- 快捷使用记录页保留原管理员使用记录页的统计卡、时间范围、粒度、模型分布、端点分布、分组分布、Token 趋势、用量明细和错误请求页签；详情与筛选均为只读。
- 快捷用量简报支持按日期或按用户分组展示；按用户分组时组标题优先显示用户名，邮箱作为辅助信息；搜索只匹配用户名和邮箱，不匹配报告标题。
- 快捷仪表盘中的用户消费榜和最近使用 Top 数据优先显示用户名，避免在公开展示场景直接暴露用户邮箱。

### 用量简报

- 管理员系统设置的“功能开关”中新增“用量简报”开关，默认关闭。
- 开启后管理员和普通用户侧边栏会显示“用量简报”入口。
- 简报只面向普通用户生成；管理员用户不参与自动生成。
- 普通用户只能只读查看自己的日报、周报和月报。
- 管理员可分页、分组查看所有普通用户正式报告，并可按用户邮箱搜索、日期范围和日报/周报/月报筛选、编辑或删除单篇报告，也可按当前筛选范围删除整组报告。
- 自动任务会为所有普通用户生成简报：日报基于当天 usage 数据和请求 JSON，周报基于周一到周日的 7 天日报，月报按严格自然月生成并使用自然月内日报校准边界。
- 生成的 Markdown 简报定位为普通用户工作总结，新生成报告标题为“工作日报”“工作周报”“工作月报”，重点凸显完成了什么模块、做了哪些设计或实现、贡献度、风险和建议；token、成本和模型等用量信息只在“用量概括”中用一句话概括。
- 如果某个周期没有采集到任何使用记录，生成的工作日报、周报或月报会简洁说明“暂无工作情况”，不会扩写空洞内容。
- 查看当天、当周或当月报告时，页面会提示“请在明天后查看”“请在下周后查看”或“请在下月后查看”。
- 报告正在排队或生成时，页面会显示“报告生成中”。
- 管理员可配置 AI 生成接口，目前使用 OpenAI Responses API：Base URL、API Key、模型名称、上下文 token、输出保留 token 和生成并发。
- 默认模型为 `gpt-5.5`，默认上下文为 `400000` tokens，默认保留 `128000` tokens 给输出 Markdown，默认生产队列并发为 `4`，管理员可配置的最大并发为 `32`。
- 管理员界面可分页查看生产与测试生成批次，默认显示全部队列，默认每页 50 条；一次手动触发或一次自动触发会创建一个批次，批次内包含每个普通用户的生成任务。
- 批次可展开查看组内每个用户任务的状态、阶段、分片进度、估算 token 进度、实际模型 input/output token 和已重试次数。
- 批次支持刷新、暂停、恢复、取消、重置、删除和手动触发生成；生产手动触发同样面向所有普通用户，不按单个用户缩小范围。
- 管理员界面提供测试生成，可选择用户、分组和时间范围，把对应请求数据加入测试队列；测试任务可随时刷新、取消、重置或删除，生成成功后的 Markdown 结果可在队列中查看。
- 测试生成也会创建批次，默认只包含当前选中的一个普通用户任务；测试生成结果只在队列批次展开后的任务中查看，不写入正式报告列表。
- 日报在生成最终 Markdown 前会读取同一用户前 7 天已经生成的日报作为连续工作背景参考；当天事实仍以当天 usage 分片摘要为准。
- 日报和测试简报的 source 分片准备完成后不再自动暂停，会直接进入 AI 分片总结；管理员仍可手动暂停、取消或重新生成任务。
- 每个 source 分片 AI 总结失败时会记录详细错误、最近错误时间和分片级重试次数，并在管理员“查看分片”中显示。
- source 分片首次调用失败后最多再重试 15 次，每次默认间隔 15 秒；重试 15 次后仍失败的分片标记为失败并跳过，后续分片继续生成。
- 如果至少一个 source 分片成功，任务会继续合并成功分片生成最终 Markdown；存在跳过分片时，任务和报告状态为 `partial`，报告顶部会提示部分分片被跳过。
- 如果所有 source 分片都失败，则任务失败，不会生成空报告。
- AI 生成遇到网络错误、超时、429 或 5xx 等可重试错误时会自动重试，不限制重试次数；每次失败默认 15 秒后再试，管理员仍可取消、恢复或重新生成。
- 超出上下文预算时不再粗暴截断输入，而是按 usage record 或下级报告边界分片总结，再把分片摘要递归合并成最终 Markdown。
- 如果服务或容器在任务运行中被关闭，下一次启动会把未主动取消的运行中任务恢复为待执行状态，并复用已成功的分片继续生成。
- 生产与测试队列中的运行中任务或批次执行删除时，会采用“取消后隐藏”的语义：页面立即不再显示，后台 worker 检测到取消后停止生成。
- 管理员手动编辑后的报告会标记为已编辑，后续自动任务不会覆盖该报告。

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

详情响应会包含：

- `available`：详情是否可用。
- `reason`：不可用原因，例如 `historical`、`not_captured` 或 `disabled`。
- `request_messages` / `response_messages`：从请求和响应中解析出的消息内容。
- `request_payload_json` / `response_payload_json`：脱敏后的原始 JSON 文本。
- `request_truncated` / `response_truncated`：是否发生截断。

### 用量简报 API

普通用户接口：

- `GET /api/v1/usage-brief/reports`：查看自己的历史简报列表。
- `GET /api/v1/usage-brief/reports/:id`：查看自己的单篇简报。
- `GET /api/v1/usage-brief/period?period_type=...&date=...`：按日报、周报或月报周期查看当前可用状态和报告内容。

管理员接口：

- `GET /api/v1/admin/usage-brief/settings`：查看 AI 生成配置。
- `PUT /api/v1/admin/usage-brief/settings`：更新 Base URL、API Key、模型、上下文、输出保留和并发设置。
- `GET /api/v1/admin/usage-brief/reports`：分页查看所有普通用户正式报告，支持 `page`、`page_size`、`user_id`、`period_type` 等查询参数。
- `GET /api/v1/admin/usage-brief/reports/:id`：查看单篇报告。
- `PUT /api/v1/admin/usage-brief/reports/:id`：编辑报告标题和 Markdown 内容。
- `DELETE /api/v1/admin/usage-brief/reports/:id`：删除报告。
- `GET /api/v1/admin/usage-brief/report-groups`：按日期周期或用户分组分页查看正式报告。
- `DELETE /api/v1/admin/usage-brief/report-groups?group_key=...`：删除当前筛选条件下的正式报告分组；只影响正式报告列表，不影响测试生成结果和生产/测试队列。
- `GET /api/v1/admin/usage-brief/batches`：分页查看生产和测试生成批次，支持 `page`、`page_size`、`scope`、`status` 等查询参数。
- `GET /api/v1/admin/usage-brief/batches/:id/jobs`：查看某个批次内的用户生成任务。
- `POST /api/v1/admin/usage-brief/batches/:id/pause`：暂停批次，暂停后未领取的任务不会继续被 worker 消费。
- `POST /api/v1/admin/usage-brief/batches/:id/resume`：恢复批次。
- `POST /api/v1/admin/usage-brief/batches/:id/cancel`：取消批次；排队任务直接取消，运行中任务会请求取消。
- `POST /api/v1/admin/usage-brief/batches/:id/reset`：恢复批次中失败、取消或排队的任务，保留并复用输入未变化的成功分片。
- `POST /api/v1/admin/usage-brief/batches/:id/rerun`：重新生成批次，清空批次内任务已保存的分片后从头执行。
- `DELETE /api/v1/admin/usage-brief/batches/:id`：删除批次；运行中任务会先请求取消并从列表隐藏。
- `GET /api/v1/admin/usage-brief/jobs`：分页查看生产和测试生成任务，支持 `page`、`page_size`、`scope`、`status`、`user_id`、`batch_id` 等查询参数；主要用于兼容和批次内任务查询。
- `GET /api/v1/admin/usage-brief/jobs/:id/chunks`：分页查看任务 AI 输入分片，返回分片状态、估算 token、实际 input/output token、`retry_count`、`last_error_at`、错误信息和分片 JSON。
- `GET /api/v1/admin/usage-brief/jobs/:id/conversations`：分页查看任务去重后的请求/响应合并对话 JSON；该列表与 AI 输入分片分开保存和展示。
- `POST /api/v1/admin/usage-brief/jobs/production`：手动触发生产日报、周报或月报生成。
- `POST /api/v1/admin/usage-brief/jobs/test`：创建测试生成任务。
- `POST /api/v1/admin/usage-brief/jobs/:id/cancel`：取消任务。
- `POST /api/v1/admin/usage-brief/jobs/:id/reset`：恢复任务回到待执行状态，保留并复用输入未变化的成功分片。
- `POST /api/v1/admin/usage-brief/jobs/:id/rerun`：重新生成任务，清空该任务已保存的分片后从头执行。
- `DELETE /api/v1/admin/usage-brief/jobs/:id`：删除队列任务；运行中任务会先请求取消并从列表隐藏。

### 运维快捷监控 API

开启“运维快捷监控”后，后端会注册无需登录但需要匹配配置后缀的只读接口，前缀为：

- `/api/v1/quick-monitor/:suffix`

当前只读接口覆盖：

- `GET /api/v1/quick-monitor/:suffix/status`：检查后缀是否启用。
- `GET /api/v1/quick-monitor/:suffix/dashboard/...`：仪表盘统计、趋势、模型、分组、用户排行等只读查询。
- `GET /api/v1/quick-monitor/:suffix/ops/...`：运维监控概览、系统资源、错误日志等只读查询；不暴露最近请求列表接口。
- `GET /api/v1/quick-monitor/:suffix/users...`：用户列表和用户详情相关只读查询。
- `GET /api/v1/quick-monitor/:suffix/accounts...`：账号列表和账号详情相关只读查询，用于使用记录等页面的只读筛选。
- `GET /api/v1/quick-monitor/:suffix/groups...`：分组列表、详情和统计相关只读查询。
- `GET /api/v1/quick-monitor/:suffix/subscriptions...`：订阅列表、详情和进度相关只读查询。
- `GET /api/v1/quick-monitor/:suffix/usage...`：使用记录列表、统计、搜索和详情相关只读查询。
- `POST /api/v1/quick-monitor/:suffix/dashboard/users-usage`、`/api-keys-usage`：仪表盘批量用量查询，语义为只读统计查询，不修改数据。
- `GET /api/v1/quick-monitor/:suffix/usage-brief/report-groups`、`/reports`、`/reports/:id`：只读查看正式用量简报报告。

不会注册管理员写接口，例如用户创建/编辑/禁用、分组编辑、订阅分配或撤销、使用记录清理、运维日志清理、用量简报 AI 配置、手动触发、队列取消/重置/删除、报告编辑/删除等。

### 数据库迁移

新增迁移文件：

- `backend/migrations/082_add_chat_tables.sql`：新增聊天会话、消息和附件表。
- `backend/migrations/083_add_chat_message_tool_fields.sql`：为聊天消息增加工具事件和引用字段。
- `backend/migrations/084_add_usage_log_details.sql`：新增 `usage_log_details` 表，用于保存 usage log 的请求/响应详情。
- `backend/migrations/154_enable_pg_trgm.sql`：启用 PostgreSQL `pg_trgm` 扩展。
- `backend/migrations/155_usage_log_details_request_payload_trgm_notx.sql`：历史迁移，曾为 `usage_log_details.request_payload_json` 创建 trigram GIN 索引。
- `backend/migrations/156_add_ops_system_metrics_disk_mounts.sql`：为运维监控系统指标快照增加 `disk_mounts_json`，保存可见挂载点的磁盘容量快照。
- `backend/migrations/157_add_usage_brief.sql`：新增用量简报基础表 `usage_brief_reports` 和 `usage_brief_jobs`。
- `backend/migrations/158_usage_brief_jobs_soft_delete.sql`：为用量简报任务增加软删除字段，并调整生产任务唯一索引。
- `backend/migrations/159_add_usage_brief_batches_chunks_retry.sql`：新增 `usage_brief_batches` 和 `usage_brief_job_chunks`，并为任务补充批次关联、自动重试、分片进度、估算 token 和实际 input/output token 字段。
- `backend/migrations/162_drop_usage_log_details_request_payload_trgm_notx.sql`：删除使用记录请求内容关键字过滤对应的 `idx_usage_log_details_request_payload_trgm` 索引；不删除 `usage_log_details` 表，也不删除 `pg_trgm` 扩展。
- `backend/migrations/163_usage_brief_job_chunks_conversation_json.sql`：为旧分片表补充临时对话 JSON 字段，用于迁移前兼容。
- `backend/migrations/164_add_usage_brief_job_conversations.sql`：新增 `usage_brief_job_conversations`，将去重后的请求/响应合并对话从 AI 输入分片中拆出并单独分页保存。
- `backend/migrations/165_usage_brief_partial_chunk_retry.sql`：允许任务和报告使用 `partial` 状态，并为 `usage_brief_job_chunks` 增加 `retry_count` 和 `last_error_at`。
- 运维快捷监控只新增系统设置项和只读路由，不新增数据库表或迁移文件。

## 行为说明与限制

- 使用记录详情只保证新产生的记录可捕获详情；合并前或迁移前的历史记录不会自动补齐。
- 使用记录列表不再支持按请求内容关键字过滤；`content_keyword` 查询参数仅作为旧客户端兼容参数被忽略。
- 请求体和响应体保留已有敏感字段脱敏逻辑。
- 当前分支去掉了原有 64KB 应用层截断，目标是完整保存新记录的请求和响应内容。
- 如果详情接口请求失败或返回 404，前端会提示加载失败并关闭详情弹窗，不会误显示为“详情不可用”。
- 只有详情接口正常返回 `available=false` 时，前端才显示“详情不可用”及其历史/未捕获原因。
- 管理员使用记录清理任务仍按时间、用户、API Key、账号、分组、模型、请求类型和计费类型等现有条件执行。
- 运维监控磁盘容量显示的是应用进程可见的文件系统；Docker 部署下只包含容器内可见的根文件系统、绑定挂载和 volume，不会显示未挂载进容器的宿主机磁盘。
- 磁盘卡片主指标优先选择应用数据目录，避免 Docker overlay 根文件系统口径掩盖实际持久化数据目录的可用空间。
- 磁盘采集会过滤 `/proc`、`/sys`、`/dev`、`/run` 等伪文件系统和运行时挂载，避免监控页面被无意义挂载点占满。
- 应用数据目录如果是 Docker Desktop/macOS 的 `fakeowner` 或 `/run/host_mark/...` 等桥接挂载，会优先使用容器内 `df -PB1` 输出，避免 `statfs` block size 异常导致 TB 级误报；如果 `df` 不可用会回退到默认采集。
- 非应用数据目录的 `fakeowner` 主机绑定挂载，以及设备路径为 `/run/host_mark/...`、`/run/desktop/mnt/host/...`、`/host_mnt/...` 的挂载会被隐藏，避免无关挂载污染明细。
- macOS Finder 可能把可清理空间计入“可用空间”；后台磁盘容量以容器内 `df`/`statfs` 口径为准。
- 部署新版本后，磁盘明细中的旧快照可能短时间保留在数据库里；等待下一次运维指标采集或刷新页面后会以新的角色标记和过滤结果展示。
- 运维快捷监控入口本质上是公开只读页面，安全性依赖“功能开关关闭默认不可用”和“后缀不可猜”；生产环境建议使用随机长后缀，并只分享给需要查看的人。
- 运维快捷监控页面不要求登录，也不会获得管理员会话；所有可编辑入口在前端隐藏，后端也只注册只读路由，旧管理员写接口不会挂到快捷入口下。
- 如果管理员关闭“运维快捷监控”或修改后缀，旧快捷地址会在下一次页面访问时失效。
- 用量简报功能关闭时，侧边栏入口隐藏，自动生成调度和后台队列消费停止；已有报告和队列记录保留在数据库中。
- 用量简报页面刷新时会先确认功能开关状态，避免 public settings 缓存未就绪时误跳转到系统设置。
- 用量简报只为普通用户生成，管理员账号不会产生日报、周报或月报。
- 功能入口仍叫“用量简报”；新生成的日报、周报、月报内容按用户工作总结组织，历史已生成报告不会被批量改写。
- 日报生成依赖 usage log 和 `usage_log_details.request_payload_json`；迁移前或未捕获详情的历史请求无法补齐请求内容。
- 周报按周一到周日生成，未结束的当前周不可查看。
- 月报周期为严格自然月，未结束的当前月不可查看。
- 周报和月报生成前会等待同一用户仍在排队或运行中的下级日报/周报，避免周一或月初并发任务生成不完整汇总。
- 如果 AI API Key 未配置或接口调用失败，队列任务会进入失败状态；管理员可修正配置后重置任务。
- 任务级可重试的 AI 调用失败会自动重试，不限制重试次数；source 分片级总结失败会先在分片内重试 15 次，超过后跳过该分片并继续处理剩余分片。
- 分片跳过后生成的报告状态为 `partial`，普通用户仍可只读查看，管理员也可在报告列表和测试队列中查看结果。
- 管理员“查看分片”默认分页展示 source 分片，分片列表默认收起；失败分片的行标题会显示“失败已跳过”、重试次数和“查看错误原因”，展开后可查看详细错误和分片 JSON。
- 管理员“查看对话”与“查看分片”是两个独立入口：对话列表展示去重后的请求/响应合并 JSON，分片列表只展示 AI 输入分片。
- 任务的估算 token 进度来自请求 JSON、下级报告和分片摘要的粗略估算；实际 input/output token 以模型接口返回的 usage 累计为准。
- 删除队列任务或批次不会删除已经生成的正式报告；报告仍通过报告列表中的删除入口管理。
- 管理员编辑后的报告不会被自动生成任务覆盖；如需重新生成，需要删除已编辑报告或手动调整内容。
- 测试生成任务只用于管理员验证指定用户、分组和时间范围内的数据生成效果，生成结果在批次展开后的任务中查看，不会写入正式报告列表。
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
