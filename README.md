# EKBDA：企业知识库开发助手

EKBDA 是一个面向企业研发场景的 AI 协作平台。当前初版已实现知识文档录入与增量导入、可信身份下的权限过滤与混合检索、带引用问答与拒答、脱敏问答轨迹和标准评测闭环，以及受企业知识、研发规范和 Git 快照约束的只读实施规划。

产品与实施设计请参阅：

- [企业知识库开发助手-产品方案.md](./企业知识库开发助手-产品方案.md)
- [企业知识库开发助手-实施方案.md](./企业知识库开发助手-实施方案.md)

## 当前能力

- `GET /healthz`：服务健康检查。
- `POST /api/v1/knowledge/documents`：录入知识文档，需要 `knowledge_admin` 角色。
- `POST /api/v1/knowledge/imports`：从受限目录导入文档与代码，并执行增量更新。
- `GET /api/v1/knowledge/imports/{id}`：查询异步导入任务进度和逐文件结果。
- `GET /api/v1/knowledge/documents/{id}/versions`：查询文档历史版本。
- `POST /api/v1/knowledge/answers`：基于授权知识生成带引用回答，证据不足时拒答。
- `GET /api/v1/observability/answer-traces/{id}`：管理员查询脱敏问答轨迹。
- `POST /api/v1/observability/answer-traces/prune`：管理员按保留天数清理过期问答轨迹。
- `GET /api/v1/observability/answer-metrics`：管理员查询问答成功、拒答、耗时、Token 和成本汇总指标。
- `POST /api/v1/evaluations/answers`：管理员使用标准用例运行真实知识库问答评测。
- `POST /api/v1/evaluations/suites`：管理员发布不可变、自动递增版本的企业评测套件。
- `GET /api/v1/evaluations/suites`：按名称查询评测套件版本历史。
- `POST /api/v1/evaluations/runs`：基于固定套件版本异步执行质量门禁。
- `GET /api/v1/evaluations/runs`：查询评测运行历史与门禁结果。
- `POST /api/v1/evaluations/runs/{id}/cancel`：取消等待中或执行中的评测。
- `POST /api/v1/evaluations/runs/{id}/retry`：为执行失败或已取消的评测创建受控重试。
- `POST /api/v1/standards/packages`：管理员发布不可变、自动递增版本的企业规范包。
- `GET /api/v1/standards/packages`：按名称、范围和选择器查询规范包历史。
- `POST /api/v1/standards/validations`：根据项目文件清单执行适用规范并生成审计报告。
- `GET /api/v1/standards/validations`：管理员查询项目规范校验历史。
- `POST /api/v1/workspaces/validations`：只读扫描受控 Git 仓库并执行适用规范。
- `GET /api/v1/workspaces/validations`：管理员查询 Git 快照与规范门禁历史。
- `POST /api/v1/access/projects`：管理员发布不可变、自动递增版本的项目与仓库访问策略。
- `GET /api/v1/access/projects/{project}`：管理员查询项目当前生效的访问策略。
- `GET /api/v1/access/projects/{project}/versions`：管理员查询项目访问策略历史版本。
- `POST /api/v1/repositories/syncs`：同步受控、干净 Git 仓库的已提交内容到项目知识库。
- `GET /api/v1/repositories/syncs`：管理员查询仓库知识同步历史。
- `GET /api/v1/repositories/syncs/{id}`：管理员查询单次同步、提交差异和脱敏统计。
- `POST /api/v1/planning/sessions`：创建只读规划会话，检索授权知识、适用规范和 Git 元数据，并生成澄清问题或实施计划。
- `POST /api/v1/planning/sessions/{id}/clarifications`：会话创建者提交完整澄清答案并生成实施计划。
- `POST /api/v1/planning/sessions/{id}/role-reviews`：并行运行产品调研、产品经理、后端、前端和运维五个只读评审，并汇总共识与冲突。
- `POST /api/v1/planning/sessions/{id}/resolutions`：审批角色逐项处理多角色评审产生的待决策事项，禁止创建者自行决议。
- `POST /api/v1/planning/sessions/{id}/decision`：由 `project_approver` 或 `knowledge_admin` 审批或拒绝计划，禁止创建者自批。
- `GET /api/v1/planning/sessions`：按项目查询规划会话；单会话和不可变事件流分别通过 `/{id}` 与 `/{id}/events` 查询。
- `POST /api/v1/project-packages`：从已批准规划会话发布不可变、自动递增版本的新项目立项包。
- `GET /api/v1/project-packages`：按项目和包名查询立项包版本历史；单个版本通过 `/{id}` 查询。
- `POST /api/v1/project-packages/{id}/reviews`：审批角色针对单个立项工件追加评审意见，并校验目标版本定义哈希。
- `GET /api/v1/project-packages/{id}/reviews`：项目成员按工件查询不可变评审历史。
- `GET /api/v1/project-packages/{id}/export`：将固定版本导出为 Markdown 或 DOCX。
- `POST /api/v1/agent-tasks/role-reviews`：异步执行五角色评审与协调。
- `POST /api/v1/agent-tasks/project-packages`：异步生成新的立项包版本。
- `GET /api/v1/agent-tasks`：按项目、类型和状态查询任务历史；单任务通过 `/{id}` 查询。
- `POST /api/v1/agent-tasks/{id}/cancel`：任务创建者或管理员取消待执行/运行中任务。
- `POST /api/v1/agent-tasks/{id}/retry`：为失败或已取消步骤创建受控重试。
- `GET /api/v1/knowledge/search`：按关键词、项目和用户角色检索知识，结果带来源引用。
- 搜索同时使用关键词分数和向量余弦相似度，并通过 RRF 融合排序。
- `restricted` 文档仅对 `allowed_roles` 中的角色可见。
- 支持本地开发身份头和企业 JWT Bearer 两种身份模式；JWT 模式下用户与角色仅来自验签后的声明。
- 默认使用内存存储，也可切换到 PostgreSQL 持久化。

## 阶段进展

### 阶段一：最小知识服务闭环——已完成

本阶段建立了可运行、可测试的 Go 服务骨架，并完成首个知识服务纵向闭环：

- 建立 `cmd`、`internal/config`、`internal/httpapi` 和 `internal/knowledge` 分层结构。
- 实现知识文档领域模型、输入校验和内存仓库。
- 实现知识录入、关键词检索、项目过滤和结果排序。
- 检索结果提供文档 ID、标题和来源 URI，可追溯到原始知识。
- 实现 `public`、`internal`、`restricted` 三种知识分级。
- 对受限知识执行基于 `allowed_roles` 的角色过滤。
- 增加健康检查、请求大小限制、开发阶段身份头和服务优雅关闭。
- 增加 Windows PowerShell 5.1 UTF-8 演示脚本，避免中文请求体被转换成问号。

### 阶段二：PostgreSQL 知识持久化——代码完成

本阶段将知识存储从仅内存模式扩展为可选 PostgreSQL 持久化：

- 增加 `memory` 和 `postgres` 两种存储驱动，可通过环境变量切换。
- 使用 `pgx` 驱动接入 PostgreSQL。
- 增加 `knowledge_documents` 表、约束和查询索引。
- 服务启动时自动执行幂等数据库迁移。
- 持久化文档内容、业务域、项目、知识分级、允许角色和更新时间。
- 增加数据库连接检测、启动失败处理和连接关闭流程。
- 提供 PostgreSQL 16 的 Docker Compose 本地开发环境。
- 增加可选的 PostgreSQL 往返集成测试。

### 阶段三 A：同步文件导入与增量索引——已完成

- 通过 `EKBDA_IMPORT_ROOT` 限制服务器可以读取的根目录。
- 支持 Markdown、文本和常见代码、配置文件格式。
- 自动忽略 `.git`、`node_modules`、`vendor`、`dist`、`build` 等目录。
- 跳过符号链接，拒绝绝对路径和越出导入根目录的相对路径。
- 单文件限制为 2 MiB，只接收合法 UTF-8 文本。
- 使用 SHA-256 内容哈希识别新建、更新和未变化文件。
- 文件更新后文档版本自动递增，未变化文件不重复写入。
- 文档按最多 80 行或约 4000 字符切片，并记录切片序号和起止行。
- 检索结果引用增加文档版本、切片序号和起止行。
- 导入响应包含扫描、新建、更新、跳过、失败数量及逐文件结果。

本阶段先用同步导入稳定数据语义，后续的任务持久化、异步执行、删除失效和历史版本已在阶段三 B 补齐。

### 阶段三 B：异步任务与知识生命周期——已完成

- `POST /api/v1/knowledge/imports` 调整为异步创建任务，返回 HTTP `202 Accepted`。
- 内存模式保存进程内任务状态，PostgreSQL 模式持久化任务、计数和逐文件结果。
- 导入执行过程中持续更新 `pending`、`running`、`completed`、`completed_with_errors` 或 `failed` 状态。
- `GET /api/v1/knowledge/imports/{id}` 可查询实时进度和结果。
- 目录同步时，已入库但本次扫描中不存在的文件会标记为 `deleted`，并停止参与检索。
- 已删除文件重新出现后会恢复为 `active`，沿用原文档 ID 并提升版本号。
- 每次创建、更新、删除和恢复都会写入不可变文档历史版本。
- `GET /api/v1/knowledge/documents/{id}/versions` 按版本倒序返回历史记录。

当前异步执行器是单进程工作协程，适合本地开发和单实例部署。多实例任务领取、租约、重试和崩溃恢复将在平台规模化阶段引入消息队列或数据库任务锁。

### 阶段四 A：应用层混合检索——已完成

- 增加统一 Embedding Provider 接口。
- 默认使用无需外部依赖的 `local-hash-v1` 向量器，便于本地开发和自动化测试。
- 支持通过 OpenAI-compatible `/embeddings` 接口接入企业批准的嵌入模型。
- 文档创建或增量更新时批量生成切片向量。
- PostgreSQL 使用 JSONB 持久化向量和 Provider 标识。
- Provider 或模型标识变化后，下一次导入会自动重新生成向量并提升文档版本。
- 查询时分别计算关键词相关度和向量余弦相似度。
- 使用 Reciprocal Rank Fusion（RRF）融合两路排序结果。
- 搜索结果返回综合分数、关键词分数和向量分数，方便后续评测与调优。

`local-hash-v1` 是词法特征向量器，不具备真正的大模型语义理解能力。生产或真实效果测试应配置企业批准的 Embedding 服务。当前向量检索在应用层执行，适合 MVP；数据规模扩大后需要切换到 pgvector 或专用向量数据库执行近似最近邻检索。

### 阶段五 A：带引用问答与拒答——已完成

- 增加独立 Answer Provider 接口，检索和生成职责分离。
- 默认提供 `local-extractive`，无需 LLM 即可验证完整的检索、引用和权限链路。
- 支持 OpenAI-compatible `/chat/completions` 接口。
- 仅将已通过用户权限过滤的检索结果发送给问答 Provider。
- 使用 `E1`、`E2` 等临时证据编号建立引用白名单。
- 模型返回的引用必须存在于本次证据集合；引用越界时系统强制拒答。
- 非拒答内容必须至少具有一个有效引用。
- 无检索结果或证据分数不足时不调用生成模型，直接返回 `insufficient_evidence`。
- 提示词明确将检索内容视为不可信数据，忽略知识文档中的指令，降低间接提示注入风险。
- 返回引用包含来源、版本、切片序号和起止行。

当前实现验证了“检索—生成—引用校验—拒答”安全闭环，但还不能仅凭提示词抵御所有提示注入或模型错误。生产使用前必须建立问答评测集、敏感问题策略和模型输出审计。

### 阶段五 B：问答评测与可观测性——已完成

- 每次问答生成唯一 `trace_id`，回答、评测结果与审计轨迹可以相互定位。
- 轨迹记录用户、项目、Provider、成功或失败状态、稳定错误码、拒答原因、证据数、引用数、耗时和 Token 用量。
- 原始问题不写入轨迹，仅保存规范化问题的 SHA-256 摘要和字符数，降低日志中的业务数据暴露风险。
- 可观测性写入采用失败关闭策略：轨迹无法持久化时不向调用方返回未审计的正常回答。
- 内存模式保存进程内轨迹；PostgreSQL 模式自动创建并持久化 `answer_traces` 表及查询索引。
- 管理端可按轨迹 ID 查询明细，也可按 `project` 汇总成功数、失败数、拒答数、平均耗时、Token 总量和 Provider 分布。
- 新增标准评测运行器，单次支持 1 至 100 个用例，可校验预期拒答、必需引用来源和答案必含文本。
- 每个评测用例复用真实检索、权限过滤、生成、引用校验和轨迹链路，避免测试替身掩盖生产链路问题。
- 提供 `evaluations/answer_cases.example.json` 基线评测集和 `scripts/evaluate.ps1` UTF-8 执行脚本。

即时评测接口适合开发阶段调试；需要形成正式审计记录和发布结论时，应使用下方版本化套件与异步门禁接口。Token 指标直接使用 Provider 返回值；`local-extractive` 不调用模型，因此 Token 为 0。

### 阶段五 C：评测版本化与 CI 质量门禁——已完成

- 同名评测套件发布时自动递增版本，已发布版本不提供修改和删除接口。
- 每个套件保存定义 SHA-256、最低通过率、用例快照、发布人和发布时间，可复现历史门禁条件。
- PostgreSQL 使用事务级名称锁分配版本号，避免并发发布产生重复版本。
- 评测运行绑定套件 ID、版本、定义哈希和阈值快照，后续新版本不会改变历史报告。
- 运行任务采用 `pending`、`running`、`completed`、`failed` 状态，结果持久化后可按套件查询历史。
- 质量门禁状态独立为 `pending`、`passed`、`failed`、`error`；通过率达到套件阈值才允许通过。
- 评测运行异步执行，单个 HTTP 请求不需要等待整套模型调用完成。
- 内存与 PostgreSQL 两种模式均实现套件、运行和报告存储。
- 提供套件发布脚本和 CI 门禁脚本；门禁失败、执行错误、等待超时分别返回非零退出码。

阶段五 C 建立了异步门禁与历史报告，但当时的进程内直接执行还不具备崩溃恢复、取消和受控重试能力；这些可靠性能力已在阶段五 D 补齐。

### 阶段五 D：可恢复评测与成本治理——已完成

- 评测运行先写入持久化队列，再由工作器原子领取；HTTP 请求结束不影响任务生命周期。
- 运行记录保存工作器、租约到期时间和尝试次数；执行期间周期续租。
- 服务退出时不伪造取消或失败结论；租约过期后，新实例会自动重新领取并恢复执行。
- PostgreSQL 使用带条件的原子 `UPDATE ... RETURNING` 领取任务，避免多个实例同时执行同一运行。
- 支持取消等待中和执行中的任务，取消结果保留为 `canceled`，不会删除审计历史。
- 仅允许重试 `failed` 或 `canceled` 运行，最多三次；质量门禁真实失败不能通过重复运行规避。
- 每次重试创建新运行并记录 `retry_of_run_id` 与 `attempt`，原始运行保持不可变。
- 问答轨迹保存当时的输入/输出 Token 单价快照及分项、总计美元成本。
- 项目指标增加 `total_cost_usd`；费率未配置或使用本地抽取模式时成本为 0。
- 管理员可按 1～3650 天保留期显式清理过期轨迹，默认不会自动删除现有审计记录。
- CI 门禁脚本识别 `canceled` 状态并返回非零退出码。

当前工作器使用数据库租约实现恢复与互斥，但仍是应用内轮询执行器，不等同于完整的企业消息队列。大规模并发下还需要任务优先级、退避、死信、并发配额和队列级监控。

### 阶段五 E：pgvector 检索下推与 Rerank——已完成

- PostgreSQL 开发环境切换为固定版本的 pgvector 镜像，启动迁移自动启用 `vector` 扩展。
- `knowledge_chunks` 同时保存兼容旧数据的 JSONB 向量和用于检索的 `embedding_vector`，迁移会回填已有非空向量。
- 使用按 `EKBDA_EMBEDDING_DIMENSION` 配置的表达式 HNSW 余弦索引；默认维度为 384。
- PostgreSQL 模式在数据库内完成项目、文档状态、知识分级、角色、Embedding Provider 和向量维度过滤，然后分别取得向量与关键词候选，避免未授权内容进入应用层和外部模型。
- 两路候选先通过 RRF 融合，再交给可插拔 Rerank Provider 生成最终排序；候选集采用限额过采样，不会把全库切片发送给重排服务。
- 默认 `local-weighted-v1` 使用融合分、关键词分、向量分和精确命中执行确定性重排，无需外部服务。
- 支持通用 HTTP `/rerank` 服务；远端不可用或响应非法时自动回退到本地重排，检索仍可用。
- 检索结果和问答引用增加 `fusion_score`、`rerank_score` 与 `reranker`，便于调优、评测和审计。
- 内存模式继续使用应用层候选计算，保持无 PostgreSQL 的本地开发体验。

本阶段完成了检索执行层的规模化基础，但索引维度必须与 Embedding 服务实际输出严格一致。切换模型、Provider 或维度后，应重新导入全部有效知识并完成检索回归，不能混用不同维度的历史向量。

### 阶段六 A：企业 JWT 身份可信化——已完成

- 增加统一 Authenticator 和请求身份上下文，业务处理器不再直接信任或读取用户身份请求头。
- `dev_headers` 模式保留 `X-User-ID`、`X-User-Roles`，只用于本地开发和自动化测试。
- `jwt` 模式只接受 `Authorization: Bearer <token>`；即使请求同时伪造 `X-User-*`，也不会覆盖或提升验签身份。
- JWT 限定 RS256，强制校验签名、Issuer、Audience、过期时间和用户标识，并校验令牌中的 `iat`、`nbf`（存在时）。
- 服务启动时通过 HTTPS 拉取 JWKS，只接受至少 2048 位的 RSA 签名公钥；未知 `kid` 触发受控刷新，以支持身份平台轮换密钥并抑制恶意高频刷新。
- 支持通过点号路径配置用户和角色声明，例如 `sub`、`employee_id`、`roles` 或 `realm_access.roles`；角色统一规范为小写并去重。
- 未认证请求统一返回 HTTP 401；JWT 模式附带 `WWW-Authenticate: Bearer`。已认证但角色不足返回 HTTP 403。
- 问答轨迹用户、评测套件发布人、运行触发人和重试人均来自可信身份上下文。
- PowerShell 导入、问答、评测与 CI 门禁脚本支持 `EKBDA_ACCESS_TOKEN`；没有令牌时才使用开发身份头。

本阶段实现的是 API 资源服务器侧的 JWT 验证，不负责浏览器登录、授权码交换、Token 刷新或用户目录管理；这些能力应由企业身份平台、API Gateway 或专门的登录前端承担。实现遵循 [OpenID Connect 的 Issuer、Audience、签名和过期校验要求](https://openid.net/specs/openid-connect-core-1_0-18.html)，JWKS 数据结构遵循 [RFC 7517](https://www.rfc-editor.org/rfc/rfc7517.html)。

### 阶段六 B：企业项目规范中心——已完成

- 规范包按 `common`、`technology`、`project` 三层发布，分别表达企业公共规范、技术栈规范和项目专属规范。
- 同一 `scope + selector + name` 每次发布自动递增版本，历史版本不可修改；定义保存 SHA-256，支持审计和复现。
- 每条规则包含稳定 ID、标题、描述、负责人、适用范围、类别和等级；类别覆盖 `directory`、`naming`、`comment`、`testing`、`workflow`。
- 规则等级包括 `guidance`、`template`、`check`、`block`；只有 `block` 违规导致质量门禁失败，`check` 违规作为非阻断问题返回。
- 首版规则引擎提供 `required_path`、`forbidden_path`、`path_pattern`、`content_required`、`minimum_matches` 五种确定性检查。
- 校验时自动选择企业公共规范、对应技术栈规范和对应项目规范中每个包的最新版本，并把实际包 ID、版本和定义哈希写入结果。
- 适用规范中出现重复规则 ID 时失败关闭，避免不同规范包对同一规则产生不透明覆盖。
- 校验输入拒绝绝对路径、父目录越界、重复路径、非法 UTF-8 和超限内容；正则使用 Go RE2，不执行规范包中的 Shell、脚本或任意代码。
- 报告记录项目、技术栈、输入 SHA-256、实际规范版本、违规项、阻断数、执行人和时间；不持久化提交的文件正文。
- 内存和 PostgreSQL 模式均支持规范包与校验报告；PostgreSQL 使用事务锁安全分配并发版本号。
- 提供可执行的 Go 服务规范包、合规项目清单以及发布和 CI 校验脚本。

本阶段建立了规范治理和确定性门禁基础。规范审批、废弃、例外申请、项目级授权、真实仓库文件扫描以及调用外部 Lint/测试/安全扫描工具将在后续阶段接入。

### 阶段六 C：受控 Git 工作区与规范门禁——已完成

- 增加独立 `EKBDA_WORKSPACE_ROOT`，Git 能力未配置时失败关闭，不自动继承知识导入目录权限。
- 请求只能指定工作区根目录下的相对仓库路径；拒绝绝对路径、`..` 越界、根目录外符号链接和把普通子目录冒充仓库根目录。
- 使用 Git 稳定机器接口读取受版本控制文件、未跟踪且未忽略文件、HEAD、当前分支和工作区状态。
- 文件名通过 NUL 分隔解析，支持空格等特殊字符；忽略规则由 Git `--exclude-standard` 统一执行。
- 已删除的受版本控制文件不会进入规范清单，因此删除 `required_path` 会正确触发阻断违规。
- 每次最多扫描 1000 个现存条目，单文件最多 1 MiB，总文本最多 5 MiB；二进制文件只记录路径和数量，不把正文送入规则引擎。
- Git 子进程具有 15 秒超时和输出上限；清除继承的 `GIT_*` 环境变量、隔离全局/系统配置、禁用 fsmonitor、自动维护和可选锁。
- 不修改或放宽 Git `safe.directory`，仓库应由运行 EKBDA 的服务账号拥有；不执行仓库脚本、Hooks、构建命令、测试、Checkout、Add、Commit 或 Push。
- 工作区快照保存仓库相对路径、HEAD、分支、脏状态、跟踪/未跟踪/二进制数量、文件级状态、输入哈希、规范报告 ID、执行人与时间。
- 内存和 PostgreSQL 模式均支持快照审计；管理员可按项目查询历史并还原对应的完整规范报告。
- 提供 `validate-workspace.ps1`，阻断规范失败时返回退出码 `1`，可以直接接入 CI。

本阶段对应 Agent 权限 L1：读取授权仓库并生成规范报告。没有授予修改代码、创建分支或执行仓库命令的 L2/L3 权限。

### 阶段六 D：用户—项目—仓库 ACL——已完成

- 新增 `disabled` 和 `enforced` 两种项目授权模式；默认 `disabled` 保持本地开发兼容，生产必须显式启用 `enforced`。
- 项目策略以稳定项目键为边界，定义用户白名单、角色白名单和仓库相对路径白名单；用户或角色任一命中后才取得项目访问权。
- 强制模式采用失败关闭：项目为空、策略缺失、用户与角色均未命中，统一返回 HTTP `403 project access denied`，不向调用方泄露项目是否存在。
- Git 工作区校验在项目成员授权之上继续执行仓库精确匹配；路径统一转换为 `/`，拒绝绝对路径、驱动器路径和 `..` 越界。
- 授权在知识录入、目录导入、知识搜索、问答、规范校验、Git 工作区校验和仓库知识同步的 API 边界统一执行，并先于检索、模型调用或仓库扫描。
- `knowledge_admin` 保留治理旁路，用于首次发布策略、修复误配置和执行企业审计；普通用户不能读取策略内容或历史。
- 同一项目每次发布自动创建新版本，历史不可修改；最新版本立即成为活动策略，可通过发布新版本完成授权或撤权。
- 每个策略保存规范化定义 SHA-256、负责人、发布人和发布时间；内存与 PostgreSQL 模式使用相同的版本和授权语义。
- PostgreSQL 使用事务级项目锁分配版本号，避免并发发布产生重复版本；提供策略示例和 PowerShell UTF-8 发布脚本。

当前 ACL 是服务 API 边界的项目级控制，不替代知识文档自身的 `classification/allowed_roles` 二次过滤。策略发布审批、生效时间、紧急授权到期、身份目录自动同步和授权决策事件流仍属于后续治理能力。

### 阶段六 E：受控仓库知识增量同步——已完成

- 新增仓库知识同步 API，复用 `EKBDA_WORKSPACE_ROOT` 的仓库根目录、符号链接、Git 根目录、文件数量、文件大小和命令超时边界。
- 同步前统一执行用户—项目—仓库 ACL；只有命中项目成员和仓库精确白名单的可信身份才能触发同步。
- 仅同步干净工作区对应的已提交 `HEAD`；存在已跟踪修改、暂存修改或未忽略的未跟踪文件时返回 HTTP `409`，不把个人临时内容写入企业知识库。
- 文本文件使用 `git://<repository>/<path>` 作为稳定来源；脱敏后内容哈希未变化时返回 `skipped`，变化时自动提升知识文档版本。
- 每次同步全量核对受控文件清单，仓库中已删除、变成高风险文件或不再支持的来源会进入知识删除版本并停止参与检索。
- 首次同步把当前树记录为新增；后续同步基于上一个完整成功报告的 `HEAD` 生成 `A/M/D/T` 等路径级提交差异，不保存 Patch 正文。
- `.env`、私钥、证书密钥库和明确的凭据文件整文件跳过；普通文本中的密码/API Key、Bearer Token、AWS Key、GitHub Token、JWT 和私钥块使用确定性标记替换。
- 内容先脱敏，再计算 SHA-256、切片和生成向量；同步报告只保存文件路径、动作、版本、脱敏数量和稳定错误，不保存原始密钥。
- 报告记录当前/上次提交、分支、全量重建标记、增删改计数、敏感文件跳过数、脱敏总数、执行人和时间。
- 同一进程内同一项目/仓库只允许一个同步执行；内存和 PostgreSQL 均持久化报告，只有完整成功报告会推进下次提交差异基线。
- 提供示例请求和 PowerShell 5.1 UTF-8 同步脚本；`full_resync=true` 可在历史提交不可达或需要重新建立基线时显式执行全量重建。

本阶段不执行远程 Fetch/Pull、Checkout、Hook、构建、测试或仓库写操作。当前同步是同步 HTTP 调用，多实例互斥和崩溃恢复尚未建立数据库租约；生产规模化前应改造成可恢复队列任务。

### 阶段七 A：需求澄清与只读实施规划 Agent——已完成

- 建立需求澄清、计划生成和四眼审批的最小状态机；阶段七 B 在该基础上加入角色评审与冲突决议，终态仍不能继续修改。
- 创建会话时统一执行用户—项目—仓库 ACL，再以调用者角色检索项目知识，解析 common、technology、project 三层适用规范，并读取受控 Git 仓库的 HEAD、分支、脏状态和变更路径元数据。
- Agent 只有知识搜索、规范解析和仓库元数据三类只读上下文，不具备文件写入、Shell、Git 修改或部署工具。
- 默认 `local` Provider 确定性识别验收标准、硬约束和范围边界三个缺口；也可使用既有 LLM 配置切换到严格 JSON 的 `openai-compatible` Provider。
- Provider 输出必须通过问题数量、字段长度、计划结构和引用白名单校验；模型不能虚构 `K*` 企业知识引用或 `S*` 规范引用。
- 知识正文仅在一次 Provider 调用期间使用；会话持久化只保留文档 ID、版本、切片号和摘要哈希，不保存检索片段或引用正文。
- 澄清仅允许会话创建者提交，`knowledge_admin` 可治理介入；批准/拒绝要求 `project_approver` 或 `knowledge_admin`，且创建者不能审批自己的计划。
- 每次写操作带乐观锁 `revision`；创建、澄清和决策形成不可变、单调递增的事件流，内存和 PostgreSQL 存储语义一致。
- 提供规划请求、澄清、审批示例和 PowerShell 5.1 UTF-8 脚本；PostgreSQL 自动迁移规划会话及事件表。

本阶段的 `approved` 只表示计划通过人工评审，不授予代码修改、命令执行或发布权限。它为后续 Vibe Coding 执行阶段建立可信输入和审批证据，但不会自动开始开发。

### 阶段七 B：多角色只读评审与冲突决议——已完成

- 当前完整状态机为 `awaiting_clarification → awaiting_role_review → awaiting_resolution（可选）→ awaiting_approval → approved/rejected`。
- 固定五个职责角色：产品调研兼竞品分析、产品经理、后端开发、前端开发和运维；每个角色使用同一次重新授权采集的知识、规范和 Git 元数据独立评审，不读取其他角色的中间结论。
- 五个角色并行执行，分别输出总结、`approve/approve_with_conditions/reject` 建议、结构化发现、开放问题以及 `K*`、`S*` 引用。
- 协调器只读取五份评审，汇总共识和带来源角色的 `decision_items`，不得增加企业事实或替代人工决策。
- 没有待决策项时直接进入 `awaiting_approval`；存在阻断发现、开放问题或角色冲突时进入 `awaiting_resolution`，最终审批接口保持关闭。
- 冲突决议要求 `project_approver` 或 `knowledge_admin`，必须覆盖全部待决策项；会话创建者不能自行解决冲突。
- 本地 Provider 提供可重复的五角色基线评审；`openai-compatible` Provider 使用角色专属职责和严格 JSON 协议分别调用五个评审与一个协调步骤。
- 新增角色评审和决议事件、乐观锁修订、PostgreSQL 状态迁移、协议文档、请求示例和 PowerShell 5.1 UTF-8 脚本。

详细职责、输出约束和审计要求参阅 [`planning/MULTI_ROLE_REVIEW_PROTOCOL.md`](./planning/MULTI_ROLE_REVIEW_PROTOCOL.md)。本阶段仍然只评审实施计划，不生成代码、不执行命令、不访问外部竞品网站，也不批准生产发布。

### 阶段七 C：版本化新项目立项包——已完成

- 只有状态为 `approved`、已完成五角色评审且冲突全部解决的规划会话可以生成立项包。
- 每个版本固定包含 `prd`、`architecture`、`api`、`test`、`deployment`、`monitoring` 和 `risk` 七类结构化工件；缺失、重复或未知类型直接拒绝。
- 工件采用统一的标题、摘要、章节、条目和引用结构，既适合 API 消费，也可作为后续 Markdown、DOCX 或项目门户渲染输入。
- 引用按 `plan_knowledge`、`plan_standard`、`review_knowledge`、`review_standard` 和 `decision` 显式分域，避免计划快照与评审快照中的相同 `K1/S1` 被错误混用。
- 仅允许引用真实白名单和已经人工解决的决策项；Provider 虚构引用、预填未解决决策或缺少工件时返回生成失败。
- 版本身份为 `project + name`，每次发布自动递增且历史不可修改；`definition_hash` 覆盖来源、Provider、变更说明和全部工件。
- 每个版本保存规划会话 ID、批准时修订号、计划/评审上下文哈希、计划批准人和批准时间，形成从企业知识到最终立项包的追踪链。
- 发布需要 `project_approver` 或 `knowledge_admin`，读取继续执行项目/仓库 ACL；内存和 PostgreSQL 存储具有相同版本语义。
- 默认本地 Provider 生成保守、可重复的标准工件，不虚构端点和指标；`openai-compatible` Provider 使用严格 JSON 协议，并复用既有企业 LLM 配置。

详细标准参阅 [`planning/PROJECT_PACKAGE_STANDARD.md`](./planning/PROJECT_PACKAGE_STANDARD.md)。立项包是详细设计与后续受控开发的标准输入，不等于预算、架构委员会或生产发布批准。

### 阶段七 D：跨工件追踪、单工件评审与文档导出——已完成

- 为 PRD 中每个唯一条目生成稳定的 `REQ-001...` 编号，并关联架构、API、测试、部署章节；章节名必须真实存在，Provider 不能伪造链接。
- API 不适用时必须给出明确原因；覆盖状态和缺口由服务端重新计算，Provider 自报结果不被信任。
- 追踪矩阵进入不可变立项包并参与 `definition_hash`，版本比较可同时识别需求正文和实现链路变化。
- 单工件评审采用独立追加记录，不修改已发布版本；请求必须携带当前包哈希，旧哈希返回 `409 Conflict`。
- 评审决策限制为 `approve` 或 `request_changes`，仅 `project_approver` / `knowledge_admin` 可提交，项目成员可读取。
- Memory 与 PostgreSQL 均保存按“立项包 + 工件”递增的评审序号；PostgreSQL 使用事务级咨询锁避免并发重复序号。
- 固定版本可确定性导出 Markdown 或 DOCX。导出包含元数据、追踪矩阵、七类工件、引用和评审记录，不创建新版本。
- DOCX 使用标准 OOXML、层级标题、真实项目符号、固定列宽和重复表头，并移除个人作者元数据；示例见 [`planning/order-export-project-package.example.docx`](./planning/order-export-project-package.example.docx)。
- 新增 `review-project-package.ps1` 与 `export-project-package.ps1`，保持 Windows PowerShell 5.1 和 UTF-8 请求兼容。

详细约束与验收口径已合并到 [`planning/PROJECT_PACKAGE_STANDARD.md`](./planning/PROJECT_PACKAGE_STANDARD.md)。本阶段仍不执行代码、合并或部署。

### 阶段七 E：可恢复 Agent 任务、成本轨迹与专项质量门禁——已完成

- 新增通用 `agenttask` 运行时，把 `role_review` 和 `project_package` 作为两个独立受控步骤异步执行；原同步 API 暂时保留兼容。
- 任务状态覆盖 `pending`、`running`、`completed`、`failed`、`canceled`，Memory 与 PostgreSQL 使用一致语义。
- Worker 通过原子领取、30 秒租约、10 秒心跳和过期租约恢复支持服务重启及多实例竞争；晚到结果不能覆盖跨实例取消标志。
- 每个任务使用独立超时上下文，默认 600 秒，可由 `EKBDA_AGENT_TASK_TIMEOUT_SECONDS` 配置；超时与租约丢失只取消当前任务。
- 失败或取消任务可显式重试，重试创建不可变新任务并记录 `retry_of_task_id`、`attempt`，一条链最多三次且禁止重复分叉。
- OpenAI-compatible 角色与立项包 Provider 读取响应 `usage`；五角色并行调用和协调调用通过线程安全 Collector 汇总 Token，并按任务执行时费率计算成本。
- 角色评审质量门禁检查五角色完整性、评审上下文和结束状态；立项包门禁检查七工件、来源、定义哈希和完整追踪覆盖。
- `quality_gate_failed` 保留资源 ID 与检查明细，但不允许原样重试，避免对确定性设计缺口浪费模型调用。
- 新增任务创建、查询、列表、取消、重试 API，以及 PowerShell 5.1 兼容的创建、等待和管理脚本。

完整状态、授权、错误码和验收标准参阅 [`planning/AGENT_TASK_STANDARD.md`](./planning/AGENT_TASK_STANDARD.md)。本阶段的单步骤重试是重试完整 `role_review` 或 `project_package` 步骤，尚不支持只恢复五角色中的单一角色。

### 阶段八 A：受控 Vibe Coding 安全基线——已完成

- 新增从“已批准且七工件最新评审全部通过”的固定立项包版本创建开发变更会话的能力；追踪矩阵存在未覆盖项时失败关闭。
- 创建时要求受控 Git 工作区干净且存在 `HEAD`，并固定立项包哈希、规划来源、基线提交、当前分支、允许路径、允许命令和 `codex/` 计划分支名。
- 仅接收最多 512 KiB、50 文件、10000 行增删的 Git unified diff；服务端完整解析文件头、操作类型、hunk 行数和实际增删，不调用 Git 应用补丁。
- 拒绝绝对/越界/重复/范围外路径、rename、copy、mode change、symlink、submodule、二进制补丁、敏感文件及新增行中的高置信秘密。
- 命令只能引用服务端固定目录 ID；当前仅有 `git-diff-check`、`go-test-all`、`go-vet-all`、`go-build-all`，不接受 Shell 字符串或调用方参数。
- 状态机为 `draft → awaiting_approval → approved/rejected`；提案只能由创建者提交，决策要求独立的 `project_approver` / `knowledge_admin`，并禁止自审批。
- 提交和决策前都重新检查工作区仍然干净且 `HEAD` 等于固定基线；任何漂移均返回冲突，不继续审批。
- 普通会话响应只显示 Patch 哈希、大小、文件统计和命令计划；完整 Diff 仅通过项目 ACL 保护的 preview 接口读取。
- Memory 与 PostgreSQL 均持久化会话、完整提案和不可变事件流；所有写操作使用 `revision` 乐观锁。
- 新增命令目录、会话、提案、预览、决策和事件 API，以及 PowerShell 5.1 UTF-8 脚本。

完整边界与验收口径参阅 [`development/CONTROLLED_CHANGE_STANDARD.md`](./development/CONTROLLED_CHANGE_STANDARD.md)。本阶段的 `approved` 仅表示只读变更方案获批；系统仍不会写工作区、创建分支、执行命令、提交、推送、合并或部署。

### 阶段八 B：受控隔离验证执行——已完成

- 新增默认关闭的本地隔离执行器；启用时要求专用执行根目录与源码工作区完全分离，重叠或符号链接重叠时启动失败。
- 只有会话创建者可执行已审批提案，并必须提交当前 `revision`、准确 `patch_hash` 和固定确认词；执行前再次校验立项包最新工件审批、追踪覆盖和 Git 基线。
- 使用 `--no-hardlinks --no-tags --no-checkout --local` 创建临时隔离克隆并 detached checkout，不创建业务分支，也不写源仓库的 worktree 元数据。
- Patch 先 `git apply --check --whitespace=error-all`，再只在隔离副本应用；源文件与 Git 状态在真实自动化测试中保持不变。
- 应用后对全部变更文件执行纵深秘密扫描，再对隔离副本执行 common、technology、project 三层规范门禁；任一步失败停止后续命令。
- 命令根据服务端 ID 重新解析固定参数，不信任持久化可执行文件或参数；Go 命令使用隔离缓存、离线模块策略、最小环境变量、代理拒绝和 CGO 关闭。
- 整体执行和每条命令均有超时，stdout/stderr 各限制 1 MiB；持久化仅保存字节数、SHA-256、退出码、超时和耗时，不保存原始输出。
- 状态扩展为 `approved → executing → verified/execution_failed`；失败不能原地重试，修改方案必须新建会话并重新审批。
- 服务定期恢复超过整体超时与宽限期的中断执行，使用乐观锁并只清理对应执行 ID 的隔离目录，避免多实例误删活动目录。
- 执行证据、稳定错误码、规范报告、清理结果和开始/结束事件在 Memory 与 PostgreSQL 中保持同一语义。

完整安全边界参阅 [`development/CONTROLLED_EXECUTION_STANDARD.md`](./development/CONTROLLED_EXECUTION_STANDARD.md)。本地执行器不能提供不可绕过的 Socket、CPU、内存、系统调用或主机文件系统隔离，因此默认关闭，不得作为不可信代码的生产沙箱；Commit、Push、合并和部署仍保持关闭。

### 阶段八 C：强隔离、企业秘密扫描与受控 PR 交付——已完成

- 新增 `container` 强隔离执行驱动并作为启用执行时的默认驱动；阶段 8B 的 `local` 驱动只保留为显式开发模式。
- 容器镜像必须使用 SHA-256 摘要固定，运行时禁止自动拉取；命令容器禁网、禁 IPC、只读根文件系统和源码挂载、去除全部 Capability、禁止提权并使用固定非 root 用户。
- CPU、内存、PID 和 `/tmp` 容量均受配置限制；不挂载容器运行时 Socket、宿主 Home 或服务密钥，可选 Go Module 缓存只能只读挂载。
- 接入可配置企业 Secret Scanner；执行前和交付前均扫描完整变更后仓库，失败、超时和输出超限全部失败关闭，只持久化扫描器名称、耗时、字节数与输出哈希。
- 状态机扩展为 `verified → delivering → delivered/delivery_failed`；交付要求独立 `project_approver` / `knowledge_admin`，禁止会话创建者自交付。
- 交付请求固定当前修订、Patch 哈希和确认词；服务再次检查 Git 基线、立项包哈希、七工件审批与追踪覆盖。
- 在独立无硬链接克隆中创建预先固定的 `codex/` 分支，只暂存批准文件，使用服务端固定作者创建 Commit，并以非强推方式 Push 到批准远端。
- 通过固定 GitHub/GitHub Enterprise CLI 创建 HTTPS PR；API 调用方不能提供分支、Commit 文本、远端、镜像、扫描参数或凭据。
- 交付证据包含 Branch、Commit、Push 状态、PR URL、扫描证据、稳定错误码和清理结果；Push 后 PR 失败会明确记录需人工对账，不自动删除远端分支。
- 服务可恢复超时的 `delivering` 会话并只清理匹配交付 ID 的本地目录；不自动重试可能已发生远端副作用的交付。

完整启用条件、安全边界、API 和验收标准参阅 [`development/CONTROLLED_DELIVERY_STANDARD.md`](./development/CONTROLLED_DELIVERY_STANDARD.md)。生产合并、保护分支审批、发布和部署仍保持独立权限与独立审批。

### 阶段八 D：代码平台对账与企业受控发布——已完成

- 新增独立发布聚合；只有阶段 8C 状态为 `delivered` 且交付结果为 `passed` 的会话可以创建发布请求，源 Commit 与 PR 由交付证据固定。
- 发布创建后必须等待代码平台 HMAC Webhook；只有 PR 已合并、保护分支成立、必需检查通过且审批数达标，才固定 Merge Commit 并进入发布审批。
- 代码平台与 CI/CD 使用不同的至少 32 字节 HMAC 密钥、时间窗口和原始请求体签名；Provider Event ID 在 Memory/PostgreSQL 中全局幂等，重复回调返回 `applied=false`，乱序回调只记审计而不回退状态。
- Pipeline、Environment 和 CI/CD Provider URL 均为启动配置白名单；应用只调用 HTTPS Broker 的固定 Run/Rollback API，不接受任意命令、URL、生产登录信息或调用方凭据。
- `release_engineer` 创建/触发，`release_approver` 独立审批，禁止创建者自审批；回滚另有申请、审批和触发链路，回滚申请人同样不能自审批。
- CI/CD 触发固定 Merge Commit、Manifest/Configuration SHA-256、变更单、九项必需门禁和供应链策略，并使用稳定 `Idempotency-Key`。
- Provider 宣告成功后仍必须提供制品 SHA-256、SBOM 地址与哈希、签名验证、来源证明，以及 configuration/secret/image/migration/monitoring/rollback/health/smoke/logs 九项可信证据；缺项时失败关闭。
- `production` 发布必须引用同项目、同源 Commit、非生产且成功的发布，固定复用其已验证制品摘要；CI/CD 回调摘要不一致时不得晋级。
- 发布、审批、运行、制品、SBOM、门禁、验证和回滚证据均持久化到不可变事件流；PostgreSQL 在单事务中完成快照、事件和 Webhook 收据更新。
- 新增发布目录、状态查询、审批、触发、回滚和双 Webhook API，以及 UTF-8 PowerShell 脚本和发布/回调样例。
- 新增 Go 摘要固定多阶段镜像、非特权 Kubernetes Deployment、无秘密环境变量和回滚预案模板；模板产物必须由 CI 计算并回填 Manifest/Configuration 哈希。

完整启用条件、API 契约、Provider 协议、状态机和验收标准参阅 [`release/CONTROLLED_RELEASE_STANDARD.md`](./release/CONTROLLED_RELEASE_STANDARD.md)。EKBDA 仍不会直接登录生产环境；代码合并、制品生成和实际部署由代码平台及最小权限 CI/CD Broker 执行。

### 验证结果

| 检查项 | 结果 |
|---|---|
| `go test ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go build ./cmd/server` | 通过 |
| `docker compose config --quiet` | 通过 |
| 导入新文件、重复跳过、修改升级版本 | 自动化测试通过 |
| 删除失效、重新恢复、历史版本 | 自动化测试通过 |
| 异步任务创建、进度查询与结果持久化 | 自动化测试通过 |
| 补丁语法、hunk 行数、路径范围和敏感内容校验 | 自动化测试通过 |
| 立项包工件审批、Git 基线漂移和自审批门禁 | 自动化测试通过 |
| 受控变更 HTTP 创建、提案隐藏、预览与独立审批 | 自动化测试通过 |
| PostgreSQL 受控变更会话、Patch 与事件往返 | 待 Docker Desktop 启动后执行 |
| 隔离克隆、Patch 应用、离线固定命令与源仓库不变 | 真实 Git 自动化测试通过 |
| 变更后秘密扫描、规范门禁、执行证据与中断恢复 | 自动化测试通过 |
| 容器禁网、只读、非特权和 CPU/内存/PID/磁盘参数 | 自动化测试通过 |
| 企业 Secret Scanner 通过、拒绝和无原文证据 | 自动化测试通过 |
| 独立交付克隆、Branch、Commit、Push 与源仓库不变 | 真实 Git 自动化测试通过 |
| 受控 PR API、独立交付角色和交付中断恢复 | 自动化测试通过 |
| PostgreSQL 交付状态、扫描证据与事件往返 | 待 Docker Desktop 启动后执行 |
| 本地向量生成、向量单路召回、RRF 融合 | 自动化测试通过 |
| OpenAI-compatible Embedding 协议适配 | 自动化测试通过 |
| 有证据回答、无证据拒答、非法引用拒答 | 自动化测试通过 |
| 受限知识不进入未授权回答上下文 | 自动化测试通过 |
| OpenAI-compatible Chat Completions 适配 | 自动化测试通过 |
| 原始问题脱敏、问答轨迹、项目指标聚合 | 自动化测试通过 |
| 标准评测的回答、拒答、引用与必含文本校验 | 自动化测试通过 |
| PostgreSQL 问答轨迹往返与指标聚合 | 待 Docker Desktop 启动后执行 |
| 评测套件自动版本、不可变快照与定义哈希 | 自动化测试通过 |
| 异步评测状态、历史报告与通过/失败门禁 | 自动化测试通过 |
| PostgreSQL 评测套件与运行报告往返 | 待 Docker Desktop 启动后执行 |
| 过期租约恢复、原子领取与租约续期 | 自动化测试通过 |
| 运行取消、重试血缘与三次尝试上限 | 自动化测试通过 |
| Token 费率快照、成本聚合与轨迹保留清理 | 自动化测试通过 |
| 越界路径拒绝、忽略目录、非法 UTF-8 | 自动化测试通过 |
| 本地 Rerank、HTTP 协议校验与失败回退 | 自动化测试通过 |
| PostgreSQL 权限条件下推、pgvector 写入与维度校验 | 自动化测试通过；实际连接待 Docker Desktop 启动后执行 |
| JWT 签名、Issuer、Audience、过期时间与算法限制 | 自动化测试通过 |
| JWT 角色映射、开发头防提权与 JWKS 密钥轮换 | 自动化测试通过 |
| 规范包不可变版本、定义哈希与三层最新版本解析 | 自动化测试通过 |
| 五类规则执行、阻断门禁、冲突和路径安全 | 自动化测试通过 |
| PostgreSQL 规范包与校验报告往返 | 待 Docker Desktop 启动后执行 |
| Git 跟踪/未跟踪/忽略/修改/删除/二进制状态采集 | 自动化测试通过 |
| Git 根目录限制、越界拒绝和规范门禁联动 | 自动化测试通过 |
| PostgreSQL Git 工作区快照往返 | 待 Docker Desktop 启动后执行 |
| ACL 用户/角色成员匹配、缺失策略失败关闭和管理员旁路 | 自动化测试通过 |
| ACL 仓库精确映射、API 无信息泄露拒绝和不可变版本 | 自动化测试通过 |
| PostgreSQL 项目访问策略往返 | 待 Docker Desktop 启动后执行 |
| 干净仓库同步、内容哈希跳过、更新与删除版本 | 自动化测试通过 |
| 提交级 A/M/D 差异、敏感文件跳过和秘密脱敏 | 自动化测试通过 |
| 仓库同步 ACL、脏工作区拒绝和审计报告查询 | 自动化测试通过 |
| PostgreSQL 仓库同步报告与成功基线往返 | 待 Docker Desktop 启动后执行 |
| 规划澄清、引用白名单、乐观锁、状态迁移与四眼审批 | 自动化测试通过 |
| 规划会话项目 ACL、持久化上下文脱敏和不可变事件流 | 自动化测试通过 |
| OpenAI-compatible 规划协议与异常输出拒绝 | 自动化测试通过 |
| 五角色并行评审、角色职责隔离和统一上下文 | 自动化测试通过 |
| 协调汇总、阻断冲突、完整人工决议和审批前置门禁 | 自动化测试通过 |
| OpenAI-compatible 角色评审与协调协议 | 自动化测试通过 |
| 七类立项工件完整性、分域引用白名单和虚构引用拒绝 | 自动化测试通过 |
| 立项包不可变版本、定义哈希、来源快照与项目 ACL | 自动化测试通过 |
| OpenAI-compatible 立项包生成协议 | 自动化测试通过 |
| 需求—架构—API—测试—部署追踪完整性与虚构章节拒绝 | 自动化测试通过 |
| 单工件评审权限、哈希冲突、追加序号和历史查询 | 自动化测试通过 |
| Markdown/DOCX 导出、OOXML 必需部件和 XML 解析 | 自动化测试通过 |
| DOCX 可访问性审计 | 高/中/低问题均为 0 |
| Agent 任务创建、领取、取消、超时与过期租约恢复 | 自动化测试通过 |
| Agent 任务重试血缘、唯一分支和三次尝试上限 | 自动化测试通过 |
| 角色评审/立项包异步 HTTP 生命周期与项目 ACL | 自动化测试通过 |
| OpenAI-compatible Token 汇总、成本计算与零成本本地模式 | 自动化测试通过 |
| 角色评审和立项包专项质量门禁 | 自动化测试通过 |
| PostgreSQL 规划会话与事件往返 | 待 Docker Desktop 启动后执行 |
| PostgreSQL 立项包版本与评审往返 | 待 Docker Desktop 启动后执行 |
| PostgreSQL Agent 任务租约、完成、历史与重试唯一性 | 待 Docker Desktop 启动后执行 |
| PostgreSQL 实际连接与重启持久化验证 | 待 Docker Desktop 启动后执行 |

### 当前阶段结论

项目已经具备可信 JWT 身份、版本化项目/仓库 ACL、版本化研发规范、受控 Git 只读扫描、仓库知识增量同步与确定性门禁、秘密脱敏、知识录入、受控目录导入、删除失效、历史版本、文档切片、数据库权限过滤、pgvector 混合检索、可插拔 Rerank、带引用问答、拒答、脱敏轨迹、版本化质量评测、可恢复 CI 门禁、只读实施规划、五角色独立评审、冲突人工决议、版本化七工件立项包、跨工件追踪矩阵、单工件评审、Markdown/DOCX 导出、受控代码验证与 PR 交付，以及基于独立审批、双签名 Webhook、供应链证据和环境晋级的 CI/CD 发布编排。实际合并和部署只由外部代码平台及最小权限 CI/CD Broker 执行；开发身份模式、同步仓库导入、内存模式应用层扫描、应用内轮询和自动迁移方式仍属于初版方案，不能直接作为生产环境最终实现。

## 启动

需要 Go 1.25 或更高版本。

```powershell
$env:GOTELEMETRY='off'
$env:GOMODCACHE="$PWD/.cache/go-mod"
$env:EKBDA_HTTP_ADDR=':8080'
$env:EKBDA_IMPORT_ROOT=(Resolve-Path '.').Path
$env:EKBDA_AUTH_MODE='dev_headers'
$env:EKBDA_EMBEDDING_PROVIDER='local'
$env:EKBDA_AGENT_TASK_TIMEOUT_SECONDS='600'
go run ./cmd/server
```

### 使用完整功能测试台

服务启动后访问 `http://localhost:8080/`。测试台直接随 Go 服务发布，不需要额外安装 Node.js，也不使用模拟接口；页面中的状态、对象和错误都来自当前 EKBDA 实例。

测试台按研发链路划分为九个工作区：指挥总览、知识资产、工程治理、规划协作、七工件、Agent 任务运行时、Vibe Coding、交付编排和质量闭环。当前后端注册的全部业务 API 都有可编辑的操作表单，支持查看结构化结果、原始响应、实际请求、HTTP 状态码和耗时；Markdown/DOCX 导出会直接下载文件，代码平台与 CI/CD Webhook 支持在浏览器内使用测试密钥生成 HMAC 签名。

首次打开时填写连接身份：

- 本地 `dev_headers` 模式填写项目、仓库、用户 ID 和角色即可；默认测试角色覆盖知识管理员、开发者、项目审批人、发布工程师和发布审批人。
- 企业 `jwt` 模式填写 Bearer Token，并按企业实际权限选择项目。Token 和 Webhook 密钥只保存在当前页面内存中，不写入 Local Storage；本地只保存不含密钥的连接资料。
- “调用记录”只保留当前页面会话，便于复现测试顺序；页面刷新后清空，不作为服务端审计日志。

推荐先在“指挥总览”确认 API 在线，再按 `知识 → 规范 → 规划 → 立项 → Agent → 编码 → 交付 → 发布 → 质量` 顺序测试。需要测试发布功能时，还应按阶段 8D 的配置启用 Release Service、代码平台 Webhook 和 CI/CD Broker 白名单。

### 接入企业 SSO/JWT

EKBDA 作为资源服务器校验企业身份平台签发的访问令牌：

```powershell
$env:EKBDA_AUTH_MODE='jwt'
$env:EKBDA_AUTH_JWT_ISSUER='https://sso.example.com/tenant'
$env:EKBDA_AUTH_JWT_AUDIENCE='ekbda-api'
$env:EKBDA_AUTH_JWT_JWKS_URL='https://sso.example.com/tenant/keys'
$env:EKBDA_AUTH_JWT_USER_ID_CLAIM='sub'
$env:EKBDA_AUTH_JWT_ROLES_CLAIM='roles'
$env:EKBDA_AUTH_JWT_CLOCK_SKEW_SECONDS='60'
go run ./cmd/server
```

企业身份平台必须为管理员令牌签发 `knowledge_admin` 角色。Keycloak 等使用嵌套声明的平台可以把角色路径配置为 `realm_access.roles`。`EKBDA_AUTH_JWT_JWKS_URL` 默认必须使用 HTTPS；`EKBDA_AUTH_JWT_ALLOW_INSECURE_HTTP=true` 仅用于隔离的本地身份服务测试，禁止用于生产。

调用 API 或运行项目脚本前，把身份平台取得的访问令牌注入当前进程：

```powershell
$env:EKBDA_ACCESS_TOKEN='<access-token>'
.\scripts\ask.ps1 -Query '如何启动服务' -Project 'order-service'
```

直接调用 API 时使用：

```powershell
$headers = @{ 'Authorization' = "Bearer $env:EKBDA_ACCESS_TOKEN" }
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/knowledge/search?q=start&project=order-service' -Headers $headers
```

### 启用项目与仓库 ACL

本地开发默认使用 `disabled`，不会因为尚未发布策略而阻断现有请求。企业试点和生产环境必须与 JWT 身份同时启用强制模式：

```powershell
$env:EKBDA_AUTH_MODE='jwt'
$env:EKBDA_PROJECT_AUTHORIZATION_MODE='enforced'
go run ./cmd/server
```

推荐先在 PostgreSQL 持久化环境发布项目策略，再切换到 `enforced`。`knowledge_admin` 在两种模式下都保留治理旁路，因此即使项目尚无策略，管理员仍可发布修复版本。示例策略位于 `access/order-service.policy.example.json`：

```powershell
.\scripts\publish-project-access.ps1
.\scripts\publish-project-access.ps1 -Path '.\access\my-project.policy.json'
```

策略中的 `users` 使用 JWT 用户标识声明的精确值，`roles` 会转换为小写匹配，`repositories` 必须与 `EKBDA_WORKSPACE_ROOT` 下的仓库相对路径精确一致。用户和角色采用“或”关系；空成员列表可用于立即锁定全部普通用户。每次发布生成新版本且立即生效，不覆盖历史：

```powershell
$headers = @{ 'Authorization' = "Bearer $env:EKBDA_ACCESS_TOKEN" }
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/access/projects/order-service' -Headers $headers
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/access/projects/order-service/versions?limit=20' -Headers $headers
```

强制模式下，搜索和问答必须显式携带 `project`。项目 ACL 先决定用户是否可进入项目边界，知识文档的 `restricted/allowed_roles` 再决定具体文档是否可见，两层授权都必须通过。

### 发布并校验项目规范

项目提供首个 Go 服务规范包和合规文件清单：

- `standards/go-service.package.example.json`
- `standards/go-service.validation.example.json`

以管理员身份发布规范包。同一包重复发布会产生新版本，不会覆盖历史定义：

```powershell
.\scripts\publish-standard-package.ps1
```

执行规范校验；存在 `block` 违规时脚本输出完整报告并返回退出码 `1`，可直接作为 CI 质量门禁：

```powershell
.\scripts\validate-standards.ps1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

校验请求格式：

```json
{
  "project": "order-service",
  "technology": "go",
  "files": [
    {"path": "go.mod", "content": "module company/order-service"},
    {"path": "internal/order/service_test.go", "content": "package order"}
  ]
}
```

系统按如下顺序加载最新规范：

1. 所有 `scope=common` 包。
2. `scope=technology` 且 `selector` 等于请求 `technology` 的包。
3. `scope=project` 且 `selector` 等于请求 `project` 的包。

校验 API 只保存文件清单的整体哈希，不保存 `content`。不过违规报告可能包含仓库相对路径，仍应按企业研发元数据进行访问控制和保留期治理。

### 校验受控 Git 仓库

启动服务前单独设置允许读取的 Git 仓库父目录：

```powershell
$env:EKBDA_WORKSPACE_ROOT='D:\enterprise-workspaces'
go run ./cmd/server
```

如果实际仓库位于 `D:\enterprise-workspaces\order-service`，请求中的 `repository` 必须是仓库根目录相对路径 `order-service`，不能传绝对路径或仓库内部子目录：

```powershell
.\scripts\validate-workspace.ps1 `
  -Repository 'order-service' `
  -Project 'order-service' `
  -Technology 'go'
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

响应由两部分组成：`repository` 是 Git 审计快照，`standards` 是本次实际执行的规范报告。未被 Git 跟踪但没有被标准忽略规则排除的文件也会进入校验；被 `.gitignore` 等规则忽略的文件不会读取。

### 同步受控仓库到知识库

同步使用与工作区校验相同的 `EKBDA_WORKSPACE_ROOT`，并要求 ACL 策略的 `repositories` 包含相同相对路径。EKBDA 不会替调用方执行 Pull 或 Checkout；运维流水线应先把已审批提交部署到只读工作区，并确认仓库干净：

```powershell
git -C 'D:\enterprise-workspaces\order-service' status --porcelain
.\scripts\sync-repository.ps1
```

默认请求位于 `repositories/order-service.sync.example.json`。同步成功后，`created/updated/skipped/deleted` 表示知识版本动作，`commit_changes` 表示上一个成功基线到当前 `HEAD` 的路径级差异，`redaction_count` 和 `sensitive_files_skipped` 用于安全审计。使用受限知识分级时必须提供允许角色：

```json
{
  "repository": "order-service",
  "project": "order-service",
  "business_domain": "交易",
  "classification": "restricted",
  "allowed_roles": ["team_order"],
  "full_resync": false
}
```

当仓库被重新克隆、历史提交已不可达或需要重置同步基线时，可以审批后设置 `full_resync=true`。这只重建差异基线并重新核对内容哈希，不会修改仓库或无条件创建知识版本。

管理员查询同步历史：

```powershell
$headers = @{ 'Authorization' = "Bearer $env:EKBDA_ACCESS_TOKEN" }
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/repositories/syncs?project=order-service' -Headers $headers
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/repositories/syncs/<sync_id>' -Headers $headers
```

内置脱敏只覆盖高置信度常见模式，是防御纵深而不是完整的秘密检测器。生产流水线仍应在提交进入受控工作区前执行企业批准的 Secret Scanner，并阻断真实密钥进入 Git 历史。

### 创建并审批只读实施计划

规划前应先完成规范发布、项目 ACL、受控仓库准备和仓库知识同步。`EKBDA_WORKSPACE_ROOT` 必须指向仓库父目录；本地默认使用确定性 Provider：

```powershell
$env:EKBDA_PLANNER_PROVIDER='local'
$created = .\scripts\create-planning-session.ps1 | ConvertFrom-Json
$created.status
$created.questions
```

默认创建请求位于 `planning/order-export.session.example.json`。如果验收标准、约束或范围边界缺失，会话停留在 `awaiting_clarification`。根据返回的真实问题 ID 修改澄清文件后提交：

```powershell
$planned = .\scripts\submit-planning-clarifications.ps1 -SessionID $created.id | ConvertFrom-Json
```

返回的计划处于 `awaiting_role_review`。使用响应中的当前修订号并行运行五角色评审：

```powershell
$reviewed = .\scripts\run-planning-role-reviews.ps1 `
  -SessionID $created.id `
  -Revision $planned.revision | ConvertFrom-Json
$reviewed.status
$reviewed.role_review.coordination
```

如果状态为 `awaiting_resolution`，根据响应中的真实 `decision_items` 修改 `planning/order-export.resolutions.example.json`，并确保文件中的 `revision` 等于 `$reviewed.revision`：

```powershell
$resolved = .\scripts\resolve-planning-reviews.ps1 -SessionID $created.id | ConvertFrom-Json
```

没有冲突时角色评审直接返回 `awaiting_approval`；存在冲突时必须完成上述决议才能进入该状态。最终审批请求中的 `revision` 必须与会话当前值一致。相应修改 `planning/order-export.approval.example.json` 后，由 `project_approver` 或 `knowledge_admin` 执行，且审批人不能是会话创建者：

```powershell
$approved = .\scripts\decide-planning-session.ps1 -SessionID $created.id | ConvertFrom-Json
```

批准完成后，由审批角色生成初始立项包。相同 `project + name` 再次生成会自动创建下一版本，`ChangeSummary` 必须说明原因：

```powershell
$projectPackage = .\scripts\create-project-package.ps1 `
  -SessionID $approved.id `
  -Name 'order-export' `
  -ChangeSummary '根据批准计划生成初始立项包' | ConvertFrom-Json
$projectPackage.version
$projectPackage.artifacts.type
```

项目成员可查询固定版本和版本历史：

```powershell
. .\scripts\auth.ps1
$headers = New-EKBDAHeaders -UserID 'developer-1' -Roles 'developer'
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/project-packages/$($projectPackage.id)" -Headers $headers
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/project-packages?project=order-service&name=order-export' -Headers $headers
```

审批角色可针对单个工件追加评审。必须使用响应中的 `definition_hash`，避免对已过期内容误评：

```powershell
.\scripts\review-project-package.ps1 `
  -PackageID $projectPackage.id `
  -PackageHash $projectPackage.definition_hash `
  -ArtifactType 'prd' `
  -Decision 'approve' `
  -Comment '范围与验收标准已经确认'
```

项目成员可导出固定版本；导出不会修改版本或哈希：

```powershell
.\scripts\export-project-package.ps1 -PackageID $projectPackage.id -Format markdown -OutputPath '.\order-export-v1.md'
.\scripts\export-project-package.ps1 -PackageID $projectPackage.id -Format docx -OutputPath '.\order-export-v1.docx'
```

耗时角色评审和立项包生成建议使用可恢复任务接口。创建任务后，脚本会返回任务 ID：

```powershell
$reviewTask = .\scripts\start-role-review-task.ps1 `
  -SessionID $created.id `
  -Revision $created.revision | ConvertFrom-Json
.\scripts\wait-agent-task.ps1 -TaskID $reviewTask.id

$packageTask = .\scripts\start-project-package-task.ps1 `
  -SessionID $approved.id `
  -Name 'order-export' `
  -ChangeSummary '异步生成初始立项包' | ConvertFrom-Json
.\scripts\wait-agent-task.ps1 -TaskID $packageTask.id -UserID 'approver-1' -UserRoles 'project_approver'
```

失败或取消任务可在限制内重试；运行中任务可由创建者取消：

```powershell
.\scripts\manage-agent-task.ps1 -TaskID $reviewTask.id -Action cancel
.\scripts\manage-agent-task.ps1 -TaskID '<failed_task_id>' -Action retry
```

任务响应中的 `resource_id` 指向完成后的规划会话或立项包；`usage` 保存 Token 与成本，`quality` 保存确定性检查结果。

查询会话、项目历史和审计事件：

```powershell
. .\scripts\auth.ps1
$headers = New-EKBDAHeaders -UserID 'developer-1' -Roles 'developer'
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/planning/sessions/$($created.id)" -Headers $headers
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/planning/sessions?project=order-service' -Headers $headers
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/planning/sessions/$($created.id)/events" -Headers $headers
```

生产环境可复用问答模型配置启用远程规划模型：

```powershell
$env:EKBDA_PLANNER_PROVIDER='openai-compatible'
$env:EKBDA_LLM_BASE_URL='https://llm.example.com/v1'
$env:EKBDA_LLM_API_KEY='your-api-key'
$env:EKBDA_LLM_MODEL='your-chat-model'
```

远程模型只接收当前需求、澄清答案、授权知识片段、适用规范和仓库元数据。知识片段不会写入规划会话；远程服务仍必须通过企业数据处理、保留期和跨境合规审批。

### 使用 PostgreSQL

启动本地数据库（需要 Docker Desktop 已运行）：

```powershell
docker compose up -d postgres
```

使用 PostgreSQL 启动服务：

```powershell
$env:GOTELEMETRY='off'
$env:GOMODCACHE="$PWD/.cache/go-mod"
$env:EKBDA_STORAGE_DRIVER='postgres'
$env:EKBDA_POSTGRES_DSN='postgres://ekbda:ekbda@localhost:5432/ekbda?sslmode=disable'
$env:EKBDA_IMPORT_ROOT=(Resolve-Path '.').Path
$env:EKBDA_EMBEDDING_PROVIDER='local'
$env:EKBDA_EMBEDDING_DIMENSION='384'
$env:EKBDA_RERANK_PROVIDER='local'
go run ./cmd/server
```

服务启动时会自动执行幂等数据库迁移并创建对应维度的 HNSW 索引。Compose 使用 `pgvector/pgvector:0.8.2-pg16`；从旧配置升级时先执行 `docker compose pull postgres`。重新启动服务后，已录入的知识仍然存在。

### 使用企业 Embedding 服务

如果服务兼容 OpenAI Embeddings 请求格式，可使用以下配置：

```powershell
$env:EKBDA_EMBEDDING_PROVIDER='openai-compatible'
$env:EKBDA_EMBEDDING_BASE_URL='https://embedding.example.com/v1'
$env:EKBDA_EMBEDDING_API_KEY='your-api-key'
$env:EKBDA_EMBEDDING_MODEL='your-embedding-model'
$env:EKBDA_EMBEDDING_DIMENSION='1024'
go run ./cmd/server
```

`EKBDA_EMBEDDING_DIMENSION` 必须等于服务实际返回的向量维度，PostgreSQL/pgvector 当前允许 1～2000 维。切换 Provider、模型或维度后必须重新执行全部知识导入；系统会为文件生成新向量。API Key 只能通过环境变量或企业密钥系统注入，不能写入仓库和知识文档。

### 使用企业 Rerank 服务

默认本地重排无需配置：

```powershell
$env:EKBDA_RERANK_PROVIDER='local'
```

接入企业 Rerank HTTP 服务：

```powershell
$env:EKBDA_RERANK_PROVIDER='http'
$env:EKBDA_RERANK_BASE_URL='https://rerank.example.com/v1'
$env:EKBDA_RERANK_API_KEY='your-api-key'
$env:EKBDA_RERANK_MODEL='your-rerank-model'
go run ./cmd/server
```

服务会向 `${EKBDA_RERANK_BASE_URL}/rerank` 发送以下结构，并要求每个输入下标恰好返回一次：

```json
{
  "model": "your-rerank-model",
  "query": "如何启动订单服务",
  "documents": [{"title": "启动说明", "text": "运行服务……"}],
  "top_n": 1
}
```

```json
{
  "results": [{"index": 0, "relevance_score": 0.97}]
}
```

只有已经通过项目和角色授权的有限候选会发送给远端服务。服务超时、非 2xx、结果缺失、下标重复或分数非法时自动回退到 `local-weighted-v1`。

### 使用企业 LLM 服务

默认 `local` 模式返回抽取式答案，适合验证引用链路：

```powershell
$env:EKBDA_ANSWER_PROVIDER='local'
```

接入兼容 Chat Completions 请求格式的企业模型服务：

```powershell
$env:EKBDA_ANSWER_PROVIDER='openai-compatible'
$env:EKBDA_LLM_BASE_URL='https://llm.example.com/v1'
$env:EKBDA_LLM_API_KEY='your-api-key'
$env:EKBDA_LLM_MODEL='your-chat-model'
$env:EKBDA_LLM_INPUT_USD_PER_MILLION_TOKENS='2.5'
$env:EKBDA_LLM_OUTPUT_USD_PER_MILLION_TOKENS='10'
go run ./cmd/server
```

Embedding 和问答模型可以使用不同服务及不同 API Key。费率单位为每百万 Token 的美元价格；系统会把当时费率写入每条轨迹，因此后续修改环境变量不会重算历史成本。无效或负数费率按 0 处理。

## API 示例

如果使用 Windows PowerShell 5.1，必须显式把 JSON 转成 UTF-8 字节；否则请求体中的中文可能被转换成 `?`。下面示例使用 `dev_headers` 模式；JWT 模式请把两个 `X-User-*` 头替换为上一节的 `Authorization` 头。代码块边界不是需要执行的命令。

录入一条知识：

```powershell
$headers = @{
  'Content-Type' = 'application/json; charset=utf-8'
  'X-User-ID' = 'admin-1'
  'X-User-Roles' = 'knowledge_admin'
}
$bodyJson = @{
  title = '订单服务启动说明'
  content = '运行 go run ./cmd/server 启动订单服务'
  source_uri = 'git://order-service/README.md'
  business_domain = '交易'
  project = 'order-service'
  classification = 'internal'
} | ConvertTo-Json
$bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyJson)
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/knowledge/documents' -Headers $headers -Body $bodyBytes
```

检索知识：

```powershell
$headers = @{
  'X-User-ID' = 'developer-1'
  'X-User-Roles' = 'developer'
}
$query = [System.Uri]::EscapeDataString('启动')
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/knowledge/search?q=$query&project=order-service" -Headers $headers
```

也可以保持服务运行，并在另一个 PowerShell 窗口直接执行：

```powershell
.\scripts\demo.ps1
```

### 导入文件或目录

服务启动前必须设置 `EKBDA_IMPORT_ROOT`。请求中的 `path` 必须是相对于该根目录的路径：

```powershell
$headers = @{
  'Content-Type' = 'application/json; charset=utf-8'
  'X-User-ID' = 'admin-1'
  'X-User-Roles' = 'knowledge_admin'
}
$importJson = @{
  path = '企业知识库开发助手-产品方案.md'
  project = 'ekbda'
  business_domain = '研发平台'
  classification = 'internal'
} | ConvertTo-Json
$importBytes = [System.Text.Encoding]::UTF8.GetBytes($importJson)
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/knowledge/imports' -Headers $headers -Body $importBytes
```

接口返回 `202 Accepted` 和任务 ID。使用 `GET /api/v1/knowledge/imports/{id}` 查询进度。对同一路径重复执行时，未变化文件返回 `skipped`；文件内容变化后返回 `updated`；目录中消失的已导入文件返回 `deleted`。

也可以使用已经处理好 UTF-8 编码的脚本：

```powershell
.\scripts\import.ps1
.\scripts\import.ps1 -Path 'internal' -Project 'ekbda-backend'
```

### 知识问答

```powershell
.\scripts\ask.ps1 -Query '这个项目如何启动？' -Project 'ekbda-backend'
```

返回结果包含 `trace_id`、`answer`、`refused`、`refusal_reason`、`provider` 和 `citations`。当 `refused=true` 时，客户端不得把回答包装成确定性业务结论。

### 问答评测与轨迹

先运行 `scripts/demo.ps1` 创建示例知识，再执行标准评测：

```powershell
.\scripts\evaluate.ps1
```

评测文件默认读取 `evaluations/answer_cases.example.json`，也可以指定企业自己的版本化用例：

```powershell
.\scripts\evaluate.ps1 -Path '.\evaluations\release-2026-08.json'
```

使用回答返回的 `trace_id` 查询脱敏轨迹和项目聚合指标：

```powershell
$headers = @{
  'X-User-ID' = 'admin-1'
  'X-User-Roles' = 'knowledge_admin'
}
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/observability/answer-traces/<trace_id>' -Headers $headers
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/observability/answer-metrics?project=order-service' -Headers $headers
```

评测与可观测性接口只允许 `knowledge_admin` 访问。轨迹不包含原始问题正文，`query_hash` 只能用于同问题关联和审计比对，不能用于恢复问题内容。

显式清理超过指定保留期的轨迹：

```powershell
$body = [System.Text.Encoding]::UTF8.GetBytes('{"retention_days":90}')
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/observability/answer-traces/prune' -Headers $headers -Body $body
```

该操作会永久删除截止时间之前的轨迹。建议先根据企业审计、合规和事件调查要求确定保留期，并在 PostgreSQL 备份策略中完成验证。

### 发布版本化评测套件并执行 CI 门禁

先创建示例知识，再发布评测套件。每次发布都会生成新的不可变版本，因此只有在用例或阈值发生变更时才应重新发布：

```powershell
.\scripts\demo.ps1
$suite = .\scripts\publish-evaluation-suite.ps1 | ConvertFrom-Json
$suite.id
```

CI 日常回归应固定使用已审批的 `suite_id`：

```powershell
.\scripts\evaluation-gate.ps1 -SuiteId '<suite_id>'
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

退出码 `0` 表示质量门禁通过，`1` 表示通过率未达到阈值，`2` 表示评测执行错误，`3` 表示等待超时。可通过以下接口查询版本和运行历史：

```powershell
$headers = @{
  'X-User-ID' = 'admin-1'
  'X-User-Roles' = 'knowledge_admin'
}
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/evaluations/suites?name=order-service-release-gate' -Headers $headers
Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/evaluations/runs?suite_id=<suite_id>' -Headers $headers
```

取消和重试运行：

```powershell
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/evaluations/runs/<run_id>/cancel' -Headers $headers
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/api/v1/evaluations/runs/<run_id>/retry' -Headers $headers
```

只有执行失败或已取消的运行允许重试，且总尝试次数不能超过三次。`completed` 且门禁未通过属于真实质量结论，接口会拒绝重试。

## 创建并审批只读开发变更提案

先确保立项包七类工件的最新评审均为 `approve`，受控仓库存在干净的已提交 `HEAD`。创建会话时显式收窄路径和命令范围：

```powershell
$development = .\scripts\create-development-session.ps1 `
  -PackageID $projectPackage.id `
  -AllowedPaths 'internal','cmd' `
  -AllowedCommands 'git-diff-check','go-test-all','go-vet-all','go-build-all' |
  ConvertFrom-Json
$development.status
$development.baseline_commit
$development.planned_branch
```

准备标准 Git unified diff 文件后提交。脚本以 UTF-8 读取 Patch；API 只接受已批准路径和命令 ID：

```powershell
$proposal = .\scripts\submit-development-proposal.ps1 `
  -SessionID $development.id `
  -Revision $development.revision `
  -PatchPath '.\change.patch' `
  -Summary '按已批准设计实现订单校验' `
  -CommandIDs 'git-diff-check','go-test-all' |
  ConvertFrom-Json

. .\scripts\auth.ps1
$headers = New-EKBDAHeaders -UserID 'developer-1' -Roles 'developer'
Invoke-RestMethod -Uri "http://localhost:8080/api/v1/development/sessions/$($development.id)/preview" -Headers $headers
```

由另一位审批人使用当前修订号决策：

```powershell
$approvedDevelopment = .\scripts\decide-development-session.ps1 `
  -SessionID $development.id `
  -Revision $proposal.revision `
  -Decision approve `
  -Comment '范围、补丁和验证计划符合批准设计' |
  ConvertFrom-Json
```

执行器默认关闭。生产启用时必须准备独立目录、节点上预置的摘要固定镜像和企业 Secret Scanner。以下以 Gitleaks 兼容参数为例；实际二进制、规则和许可证由企业安全平台提供：

```powershell
New-Item -ItemType Directory -Force 'D:\ekbda-executions'
$env:EKBDA_DEVELOPMENT_EXECUTION_ENABLED='true'
$env:EKBDA_DEVELOPMENT_EXECUTION_DRIVER='container'
$env:EKBDA_DEVELOPMENT_EXECUTION_ROOT='D:\ekbda-executions'
$env:EKBDA_DEVELOPMENT_EXECUTION_TIMEOUT_SECONDS='120'
$env:EKBDA_DEVELOPMENT_CONTAINER_IMAGE='registry.example/ekbda/go-runner@sha256:<64位镜像摘要>'
$env:EKBDA_DEVELOPMENT_SECRET_SCANNER_NAME='enterprise-gitleaks'
$env:EKBDA_DEVELOPMENT_SECRET_SCANNER_BINARY='gitleaks'
$env:EKBDA_DEVELOPMENT_SECRET_SCANNER_ARGS_JSON='["detect","--no-git","--redact","--source","{repository}"]'
```

重启服务后可执行一次已经审批的固定提案：

```powershell
$execution = .\scripts\execute-development-session.ps1 `
  -SessionID $development.id `
  -Revision $approvedDevelopment.revision `
  -PatchHash $approvedDevelopment.proposal.patch_hash |
  ConvertFrom-Json
$execution.status
$execution.execution.commands
```

受控交付也默认关闭。准备另一个独立目录、机器人 Git 身份、批准远端以及已登录的 `gh`/GitHub Enterprise CLI；令牌应由 Secret Manager 注入，不应写入 `.env` 或命令历史：

```powershell
New-Item -ItemType Directory -Force 'D:\ekbda-deliveries'
$env:EKBDA_DEVELOPMENT_DELIVERY_ENABLED='true'
$env:EKBDA_DEVELOPMENT_DELIVERY_ROOT='D:\ekbda-deliveries'
$env:EKBDA_DEVELOPMENT_DELIVERY_REMOTE='origin'
$env:EKBDA_DEVELOPMENT_DELIVERY_AUTHOR_NAME='EKBDA Delivery Bot'
$env:EKBDA_DEVELOPMENT_DELIVERY_AUTHOR_EMAIL='ekbda@example.com'
```

重启服务后，由独立审批人交付已经通过强隔离验证的会话：

```powershell
$delivery = .\scripts\deliver-development-session.ps1 `
  -SessionID $development.id `
  -Revision $execution.revision `
  -PatchHash $execution.proposal.patch_hash |
  ConvertFrom-Json
$delivery.status
$delivery.delivery.pull_request_url
```

可通过 `GET /api/v1/development/commands` 查看固定命令以及实际 `execution_enabled`、`delivery_enabled`。变更审批、8B 本地执行边界和 8C 强隔离交付分别见 [`development/CONTROLLED_CHANGE_STANDARD.md`](./development/CONTROLLED_CHANGE_STANDARD.md)、[`development/CONTROLLED_EXECUTION_STANDARD.md`](./development/CONTROLLED_EXECUTION_STANDARD.md) 与 [`development/CONTROLLED_DELIVERY_STANDARD.md`](./development/CONTROLLED_DELIVERY_STANDARD.md)。

## 测试

```powershell
$env:GOTELEMETRY='off'
$env:GOCACHE="$PWD/.cache/go-build"
$env:GOMODCACHE="$PWD/.cache/go-mod"
go test ./...
```

PostgreSQL 集成测试默认跳过。数据库运行后可显式执行：

```powershell
$env:EKBDA_TEST_POSTGRES_DSN='postgres://ekbda:ekbda@localhost:5432/ekbda?sslmode=disable'
go test ./internal/knowledge ./internal/ingestion ./internal/answer ./internal/evaluation ./internal/standards ./internal/workspace ./internal/access ./internal/repositorysync ./internal/planning ./internal/initiative ./internal/agenttask ./internal/development -run Postgres -v
```

## 初版限制

- 默认内存模式下服务重启后数据会清空；PostgreSQL 模式支持持久化。
- 内存模式仍在应用层扫描授权文档切片；PostgreSQL 模式已使用 pgvector/HNSW 下推候选检索。
- 已提供通用 HTTP Rerank 适配器，但具体 Cross-Encoder/专用模型、批量能力、限流和费用需要按企业供应商完成生产适配。
- pgvector 索引按单一配置维度建立；Embedding 模型变更必须重新导入知识并执行评测门禁。
- 问答证据阈值目前为固定初始值；已具备评测工具，但仍需用企业真实评测集完成校准。
- OpenAI-compatible 模式依赖模型遵循 JSON 输出约定，异常输出会作为生成失败处理。
- 受控 Agent 任务和异步评测支持数据库租约恢复；知识目录导入任务仍由单进程执行，服务异常退出时不会恢复未完成导入。
- 异步评测已支持数据库租约、过期恢复、取消和受控重试，但尚未实现优先级、退避、死信队列和并发配额。
- `dev_headers` 仍允许调用方声明任意身份，只能用于本地开发；生产必须显式配置 `EKBDA_AUTH_MODE=jwt` 并在部署门禁中禁止回退。
- 当前只支持 RS256/JWKS 单 Issuer 验证；多租户 Issuer、令牌撤销/introspection 和身份事件审计仍需后续补齐。
- 规范包当前发布后立即成为同名适用包的最新版本，尚未实现草稿审批、生效时间、废弃、回滚和例外申请工作流。
- 规则引擎支持调用方清单、受控 Git 清单和隔离副本门禁；当前规则仍是确定性路径/内容检查，不等价于完整语言 Lint、覆盖率或企业 SAST。
- 阶段 8C 已提供容器参数级强隔离，但生产仍必须由平台确保 rootless 运行时、默认 Seccomp/AppArmor/SELinux、节点级出站拒绝、签名镜像准入和只读供应链缓存。
- 仓库知识同步当前是同步 HTTP 任务，仅提供进程内同仓库互斥；多实例原子领取、租约恢复、Webhook 去重、重试和死信队列尚未实现。
- 内置秘密脱敏仍只提供纵深防御；生产必须配置企业 Secret Scanner，并由安全平台治理规则版本、例外审批、许可证和原始报告归档。
- ACL 当前只有项目成员和仓库白名单，没有操作级权限、策略草稿审批、生效/失效时间、临时授权、外部身份目录同步和授权决策事件审计。
- 需求澄清和初始计划生成仍是同步 HTTP 请求；五角色评审已支持异步任务、超时、重试和成本轨迹。
- 计划审批后即进入终态，尚未提供带修订意见的重新规划、计划版本差异和审批撤回工作流。
- 五个角色当前并行调用同一 Provider 配置；任务可重试完整角色评审步骤，但尚未支持按角色选择模型、单角色重试和部分结果恢复。
- 协调器可汇总模型识别出的语义冲突，本地确定性 Provider 只把阻断发现、开放问题和拒绝建议提升为待决策事项，不执行自然语言矛盾推理。
- 追踪矩阵目前以 PRD 条目为粒度引用章节，不解析章节内更细的设计元素、API 操作 ID、测试用例 ID 或部署任务 ID；企业可在后续专项模型中继续细化。
- 单工件评审是追加意见，不支持原地修订；变更必须回到规划/设计流程并发布新的立项包版本。
- DOCX 运行时采用纯 Go 生成标准 OOXML；已完成结构解析和可访问性审计，但当前开发机缺少 LibreOffice，自动化像素级渲染回归仍需在 CI 文档工具镜像中补齐。
- 本地立项包 Provider 只整理已批准事实并提供标准化基线，不会推断真实端点、架构拓扑、容量阈值或竞品数据；这些内容需要企业模型或人工补充后发布新版本。
- 立项包已支持同步兼容接口和可恢复异步任务；尚未支持只重试单个工件生成或工件级 Token 成本拆分。
- 受控交付当前实现 GitHub/GitHub Enterprise CLI 提供器；GitLab、Bitbucket、企业代码平台 Broker、PR 幂等查询和 Webhook 对账尚未接入。
- 交付中断可能发生“远端 Push/PR 已成功但本地证据未持久化”，系统会失败关闭并要求人工对账，不会自动删除远端分支或重试外部副作用。
- 阶段 8D 已提供保护分支合并证据对账、供应链证据门禁、环境晋级、发布审批和可回滚 CI/CD 编排；具体 GitHub/GitLab Webhook 转换器、制品仓库/KMS 签名实现和 Kubernetes/云平台执行器仍应由企业 CI/CD Broker 适配，EKBDA 本身不直接持有生产权限。

## 下一步

1. 使用企业真实标注扩充评测套件，校准证据阈值，并把门禁脚本接入正式 CI 审批流水线。
2. 将仓库同步改造成带数据库租约的可恢复任务，并接入 Git Webhook、幂等事件键和企业 Secret Scanner 门禁。
3. 为 ACL 增加策略审批、生效时间、临时授权到期和授权决策审计，并对接企业用户目录。
4. 将追踪矩阵细化到 API 操作、测试用例和部署任务标识，并在新版本发布时生成结构化差异报告。
5. 为 Agent 任务增加队列优先级、指数退避、死信队列、项目并发配额、单角色恢复和可观测告警。
6. 阶段八完成后执行一次大范围测试：覆盖 Memory/PostgreSQL、JWT/ACL、知识导入检索问答、规划与多角色 Agent、立项包、8A—8D 全链路、容器安全参数、Webhook 重放/乱序/冲突、故障恢复、并发、性能和发布回滚演练，并形成缺陷与准生产验收报告。
