const GROUPS = [
  { id: 'overview', index: '00', title: '总览', short: '指挥总览', description: '系统健康、项目对象和端到端交付链路。' },
  { id: 'knowledge', index: '01', title: '知识库', short: '知识资产', description: '录入、导入、版本、权限检索、可信问答与引用证据。', list: 'knowledge.search', primary: 'knowledge.documents.create' },
  { id: 'governance', index: '02', title: '规范与仓库', short: '工程治理', description: '项目 ACL、仓库知识同步、规范包、文件校验和真实工作区扫描。', list: 'governance.standards.list', primary: 'governance.standards.create' },
  { id: 'planning', index: '03', title: '产品规划', short: '规划协作', description: '需求澄清、五角色评审、冲突决议和独立规划审批。', list: 'planning.sessions.list', primary: 'planning.sessions.create' },
  { id: 'packages', index: '04', title: '项目立项包', short: '七工件', description: 'PRD、架构、API、测试、部署、监控、风险与跨工件追踪。', list: 'packages.list', primary: 'packages.create' },
  { id: 'agents', index: '05', title: 'Agent 任务', short: '任务运行时', description: '异步角色评审和立项包生成，支持取消、重试、成本与质量轨迹。', list: 'agents.list', primary: 'agents.role-review.create' },
  { id: 'development', index: '06', title: '受控开发', short: 'Vibe Coding', description: '变更提案、审批、隔离验证、秘密扫描以及受控 Branch、Commit 与 PR。', list: 'development.sessions.list', primary: 'development.sessions.create' },
  { id: 'release', index: '07', title: '发布上线', short: '交付编排', description: '保护分支对账、供应链证据、环境晋级、部署审批和可验证回滚。', list: 'release.list', primary: 'release.create' },
  { id: 'quality', index: '08', title: '评测与观测', short: '质量闭环', description: '问答指标、脱敏轨迹、评测套件、异步门禁、取消与重试。', list: 'quality.runs.list', primary: 'quality.suites.create' }
];

const field = (name, label, location, fallback = '', options = {}) => ({ name, label, location, fallback, ...options });
const operation = (group, id, label, method, path, description, role, options = {}) => ({ group, id, label, method, path, description, role, params: [], body: null, ...options });

const OPERATIONS = [
  operation('overview', 'system.health', '健康检查', 'GET', '/healthz', '验证 API 进程是否可用。', '公开'),

  operation('knowledge', 'knowledge.documents.create', '创建或更新知识文档', 'POST', '/api/v1/knowledge/documents', '录入知识并生成版本、切片和检索索引。', 'knowledge_admin', { body: { title: '订单服务启动说明', content: '运行 go run ./cmd/server 启动订单服务。', source_uri: 'git://order-service/README.md', business_domain: '交易', project: '$project', classification: 'internal', allowed_roles: [] } }),
  operation('knowledge', 'knowledge.documents.versions', '查询文档版本', 'GET', '/api/v1/knowledge/documents/{id}/versions', '查看文档不可变版本历史。', 'knowledge_admin', { params: [field('id', '文档 ID', 'path')] }),
  operation('knowledge', 'knowledge.imports.create', '目录批量导入', 'POST', '/api/v1/knowledge/imports', '从受控导入根目录扫描并同步知识文件。', 'knowledge_admin', { body: { path: '.', project: '$project', business_domain: '研发', classification: 'internal', allowed_roles: [] } }),
  operation('knowledge', 'knowledge.imports.get', '查询导入报告', 'GET', '/api/v1/knowledge/imports/{id}', '查看目录导入结果和单文件动作。', 'knowledge_admin', { params: [field('id', '导入任务 ID', 'path')] }),
  operation('knowledge', 'knowledge.search', '权限过滤检索', 'GET', '/api/v1/knowledge/search', '执行关键词、向量与 Rerank 混合检索。', '项目成员', { params: [field('q', '检索问题', 'query', '如何启动服务'), field('project', '项目', 'query', '$project'), field('limit', '返回数量', 'query', '10', { type: 'number' })] }),
  operation('knowledge', 'knowledge.answers.create', '可信问答', 'POST', '/api/v1/knowledge/answers', '生成带引用的答案；证据不足时明确拒答。', '项目成员', { body: { query: '如何在本地启动订单服务？', project: '$project', limit: 6 } }),

  operation('governance', 'governance.access.create', '发布项目 ACL', 'POST', '/api/v1/access/projects', '创建不可变项目授权策略新版本。', 'knowledge_admin', { body: { project: '$project', description: '项目测试访问策略', owner: 'platform', users: ['developer-1'], roles: ['team_order'], repositories: ['$repository'] } }),
  operation('governance', 'governance.access.get', '读取生效 ACL', 'GET', '/api/v1/access/projects/{project}', '查看项目当前生效授权策略。', 'knowledge_admin', { params: [field('project', '项目', 'path', '$project')] }),
  operation('governance', 'governance.access.versions', '读取 ACL 历史', 'GET', '/api/v1/access/projects/{project}/versions', '查看授权策略全部版本。', 'knowledge_admin', { params: [field('project', '项目', 'path', '$project'), field('limit', '数量', 'query', '20', { type: 'number' })] }),
  operation('governance', 'governance.sync.create', '同步仓库知识', 'POST', '/api/v1/repositories/syncs', '扫描干净 Git 工作区并增量同步文档。', '项目成员', { body: { repository: '$repository', project: '$project', business_domain: '研发', classification: 'internal', allowed_roles: [], full_resync: false } }),
  operation('governance', 'governance.sync.list', '列出同步报告', 'GET', '/api/v1/repositories/syncs', '查看仓库知识同步记录。', 'knowledge_admin', { params: [field('project', '项目', 'query', '$project'), field('limit', '数量', 'query', '20', { type: 'number' })] }),
  operation('governance', 'governance.sync.get', '读取同步报告', 'GET', '/api/v1/repositories/syncs/{id}', '查看提交变化、文件动作与脱敏统计。', 'knowledge_admin', { params: [field('id', '同步报告 ID', 'path')] }),
  operation('governance', 'governance.standards.create', '发布研发规范包', 'POST', '/api/v1/standards/packages', '创建目录、命名、注释、测试或流程规范版本。', 'knowledge_admin', { body: { name: 'go-service-baseline', description: 'Go 服务基础规范', scope: 'technology', selector: 'go', owner: 'platform', rules: [{ id: 'GO-TEST-001', title: '必须包含测试文件', description: 'Go 服务至少包含一个测试文件', owner: 'platform', category: 'testing', level: 'block', check: { type: 'minimum_matches', pattern: '_test\\.go$', minimum: 1 } }] } }),
  operation('governance', 'governance.standards.list', '列出规范包', 'GET', '/api/v1/standards/packages', '按名称、范围和选择器查询规范版本。', '项目成员', { params: [field('name', '名称', 'query', ''), field('scope', '范围', 'query', ''), field('selector', '选择器', 'query', ''), field('limit', '数量', 'query', '50', { type: 'number' })] }),
  operation('governance', 'governance.standards.get', '读取规范包', 'GET', '/api/v1/standards/packages/{id}', '查看规范规则和确定性检查定义。', '项目成员', { params: [field('id', '规范包 ID', 'path')] }),
  operation('governance', 'governance.standards.validate', '校验文件清单', 'POST', '/api/v1/standards/validations', '对调用方提交的文件内容执行三层规范门禁。', '项目成员', { body: { project: '$project', technology: 'go', files: [{ path: 'README.md', content: '# Order Service\n' }, { path: 'main_test.go', content: 'package main\n' }] } }),
  operation('governance', 'governance.validations.list', '列出规范报告', 'GET', '/api/v1/standards/validations', '查看项目规范校验历史。', 'knowledge_admin', { params: [field('project', '项目', 'query', '$project'), field('limit', '数量', 'query', '20', { type: 'number' })] }),
  operation('governance', 'governance.validations.get', '读取规范报告', 'GET', '/api/v1/standards/validations/{id}', '查看阻断项和规则来源。', 'knowledge_admin', { params: [field('id', '报告 ID', 'path')] }),
  operation('governance', 'governance.workspace.validate', '扫描真实工作区', 'POST', '/api/v1/workspaces/validations', '只读扫描受控 Git 仓库并执行规范门禁。', '项目成员', { body: { repository: '$repository', project: '$project', technology: 'go' } }),
  operation('governance', 'governance.workspace.list', '列出工作区校验', 'GET', '/api/v1/workspaces/validations', '查看仓库快照和规范报告引用。', 'knowledge_admin', { params: [field('project', '项目', 'query', '$project'), field('limit', '数量', 'query', '20', { type: 'number' })] }),
  operation('governance', 'governance.workspace.get', '读取工作区校验', 'GET', '/api/v1/workspaces/validations/{id}', '查看文件统计、Git 状态和规范结果。', 'knowledge_admin', { params: [field('id', '校验 ID', 'path')] }),

  operation('planning', 'planning.sessions.create', '创建规划会话', 'POST', '/api/v1/planning/sessions', '固定需求、约束、知识、规范与仓库上下文。', '项目成员', { body: { project: '$project', repository: '$repository', technology: 'go', title: '订单导出能力', requirement: '支持运营人员按日期范围导出订单。', acceptance_criteria: ['仅导出当前项目授权数据', '一万条订单在一分钟内完成'], constraints: ['复用现有鉴权', '不直接访问生产数据库'], out_of_scope: ['实时数据仓库'] } }),
  operation('planning', 'planning.sessions.list', '列出规划会话', 'GET', '/api/v1/planning/sessions', '查看项目规划及当前状态。', '项目成员', { params: [field('project', '项目', 'query', '$project'), field('limit', '数量', 'query', '30', { type: 'number' })] }),
  operation('planning', 'planning.sessions.get', '读取规划会话', 'GET', '/api/v1/planning/sessions/{id}', '完整展示问题、计划、角色结论和决策项。', '项目成员', { params: [field('id', '规划会话 ID', 'path')] }),
  operation('planning', 'planning.events.list', '查看规划事件', 'GET', '/api/v1/planning/sessions/{id}/events', '读取不可变状态迁移事件。', '项目成员', { params: [field('id', '规划会话 ID', 'path')] }),
  operation('planning', 'planning.clarifications.submit', '提交需求澄清', 'POST', '/api/v1/planning/sessions/{id}/clarifications', '按真实问题 ID 提交答案并生成实施计划。', '会话参与者', { params: [field('id', '规划会话 ID', 'path')], body: { revision: 1, answers: [{ question_id: 'replace-with-question-id', answer: '由运营角色使用，导出保留七天。' }] } }),
  operation('planning', 'planning.role-reviews.run', '执行五角色评审', 'POST', '/api/v1/planning/sessions/{id}/role-reviews', '同步运行产品调研、产品、后端、前端和运维评审。', '会话参与者', { params: [field('id', '规划会话 ID', 'path')], body: { revision: 2 } }),
  operation('planning', 'planning.resolutions.submit', '提交冲突决议', 'POST', '/api/v1/planning/sessions/{id}/resolutions', '解决角色评审产生的真实决策项。', '会话参与者', { params: [field('id', '规划会话 ID', 'path')], body: { revision: 3, resolutions: [{ decision_id: 'replace-with-decision-id', resolution: '采用异步导出并限制并发。' }] } }),
  operation('planning', 'planning.decision.submit', '审批规划', 'POST', '/api/v1/planning/sessions/{id}/decision', '独立批准或拒绝规划。', 'project_approver', { params: [field('id', '规划会话 ID', 'path')], body: { revision: 4, decision: 'approve', reason: '范围、风险和验收标准已确认。' } }),

  operation('packages', 'packages.create', '生成项目立项包', 'POST', '/api/v1/project-packages', '从已批准规划生成版本化七工件。', 'project_approver', { body: { session_id: 'replace-with-approved-planning-session-id', name: 'order-export', change_summary: '根据批准规划生成初始立项包' } }),
  operation('packages', 'packages.list', '列出项目包', 'GET', '/api/v1/project-packages', '查看项目立项包版本。', '项目成员', { params: [field('project', '项目', 'query', '$project'), field('name', '名称', 'query', ''), field('limit', '数量', 'query', '30', { type: 'number' })] }),
  operation('packages', 'packages.get', '读取完整项目包', 'GET', '/api/v1/project-packages/{id}', '展示七工件和需求追踪矩阵。', '项目成员', { params: [field('id', '项目包 ID', 'path')] }),
  operation('packages', 'packages.reviews.create', '评审单个工件', 'POST', '/api/v1/project-packages/{id}/reviews', '基于当前包哈希追加批准或拒绝意见。', 'project_approver', { params: [field('id', '项目包 ID', 'path')], body: { artifact_type: 'prd', package_hash: 'replace-with-definition-hash', decision: 'approve', comment: '范围与验收标准已确认。' } }),
  operation('packages', 'packages.reviews.list', '列出工件评审', 'GET', '/api/v1/project-packages/{id}/reviews', '查看指定项目包的追加式评审。', '项目成员', { params: [field('id', '项目包 ID', 'path'), field('artifact_type', '工件类型', 'query', ''), field('limit', '数量', 'query', '50', { type: 'number' })] }),
  operation('packages', 'packages.export', '导出项目包', 'GET', '/api/v1/project-packages/{id}/export', '下载 Markdown 或 DOCX 立项包。', '项目成员', { params: [field('id', '项目包 ID', 'path'), field('format', '格式', 'query', 'markdown', { choices: ['markdown', 'docx'] })], download: true }),

  operation('agents', 'agents.role-review.create', '创建角色评审任务', 'POST', '/api/v1/agent-tasks/role-reviews', '异步执行完整五角色评审步骤。', '会话参与者', { body: { session_id: 'replace-with-planning-session-id', revision: 2 } }),
  operation('agents', 'agents.package.create', '创建立项包任务', 'POST', '/api/v1/agent-tasks/project-packages', '异步生成版本化项目立项包。', 'project_approver', { body: { session_id: 'replace-with-approved-planning-session-id', name: 'order-export', change_summary: '异步生成批准版本' } }),
  operation('agents', 'agents.list', '列出 Agent 任务', 'GET', '/api/v1/agent-tasks', '按类型和状态查看任务、费用和质量。', '项目成员', { params: [field('project', '项目', 'query', '$project'), field('kind', '任务类型', 'query', ''), field('status', '状态', 'query', ''), field('limit', '数量', 'query', '50', { type: 'number' })] }),
  operation('agents', 'agents.get', '读取 Agent 任务', 'GET', '/api/v1/agent-tasks/{id}', '查看尝试次数、成本、质量和资源 ID。', '项目成员', { params: [field('id', '任务 ID', 'path')] }),
  operation('agents', 'agents.cancel', '取消 Agent 任务', 'POST', '/api/v1/agent-tasks/{id}/cancel', '对 pending/running 任务发出取消请求。', '任务发起者/管理员', { params: [field('id', '任务 ID', 'path')] }),
  operation('agents', 'agents.retry', '重试 Agent 任务', 'POST', '/api/v1/agent-tasks/{id}/retry', '对允许重试的终态任务创建新尝试。', '任务发起者/管理员', { params: [field('id', '任务 ID', 'path')] }),

  operation('development', 'development.commands.list', '读取命令目录', 'GET', '/api/v1/development/commands', '查看服务端固定命令及执行/交付开关。', '项目成员'),
  operation('development', 'development.sessions.create', '创建开发会话', 'POST', '/api/v1/development/sessions', '从七工件全部批准的固定项目包创建变更会话。', 'developer', { body: { project_package_id: 'replace-with-approved-package-id', technology: 'go', allowed_paths: ['internal/', 'cmd/', 'README.md'], allowed_commands: ['git-diff-check', 'go-test-all', 'go-vet-all', 'go-build-all'] } }),
  operation('development', 'development.sessions.list', '列出开发会话', 'GET', '/api/v1/development/sessions', '查看受控变更状态和证据。', '项目成员', { params: [field('project', '项目', 'query', '$project'), field('limit', '数量', 'query', '30', { type: 'number' })] }),
  operation('development', 'development.sessions.get', '读取开发会话', 'GET', '/api/v1/development/sessions/{id}', '查看固定基线、提案、执行和交付摘要。', '项目成员', { params: [field('id', '开发会话 ID', 'path')] }),
  operation('development', 'development.events.list', '查看开发事件', 'GET', '/api/v1/development/sessions/{id}/events', '读取完整状态迁移审计。', '项目成员', { params: [field('id', '开发会话 ID', 'path')] }),
  operation('development', 'development.preview.get', '预览完整 Diff', 'GET', '/api/v1/development/sessions/{id}/preview', '查看批准前的完整 Patch 与文件统计。', '项目成员', { params: [field('id', '开发会话 ID', 'path')] }),
  operation('development', 'development.proposals.submit', '提交变更提案', 'POST', '/api/v1/development/sessions/{id}/proposals', '提交严格解析的 unified diff 和固定命令 ID。', '会话创建者', { params: [field('id', '开发会话 ID', 'path')], body: { revision: 1, summary: '补充订单服务健康检查', patch: 'diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1,2 @@\n # Order Service\n+Health endpoint: /healthz\n', command_ids: ['git-diff-check', 'go-test-all'] } }),
  operation('development', 'development.decision.submit', '审批变更提案', 'POST', '/api/v1/development/sessions/{id}/decision', '独立批准或拒绝代码变更提案。', 'project_approver', { params: [field('id', '开发会话 ID', 'path')], body: { revision: 2, decision: 'approve', comment: '变更范围与命令计划符合规范。' } }),
  operation('development', 'development.execute', '隔离验证执行', 'POST', '/api/v1/development/sessions/{id}/execute', '在隔离克隆或强隔离容器中验证批准 Patch。', 'developer', { params: [field('id', '开发会话 ID', 'path')], body: { revision: 3, patch_hash: 'replace-with-patch-hash', confirmation: 'execute_approved_change' } }),
  operation('development', 'development.deliver', '受控 PR 交付', 'POST', '/api/v1/development/sessions/{id}/deliver', '执行企业秘密扫描、Branch、Commit、Push 和 PR。', 'project_approver', { params: [field('id', '开发会话 ID', 'path')], body: { revision: 4, patch_hash: 'replace-with-patch-hash', confirmation: 'deliver_verified_change' } }),

  operation('release', 'release.catalog', '读取发布目录', 'GET', '/api/v1/releases/catalog', '查看发布开关、白名单环境、流水线和固定门禁。', '项目成员'),
  operation('release', 'release.create', '创建发布申请', 'POST', '/api/v1/releases', '从已交付开发会话创建受控发布。', 'release_engineer', { body: { development_session_id: 'replace-with-delivered-session-id', environment: '$environment', pipeline: 'standard-build-deploy', change_ticket: 'CHG-2026-001', manifest_sha256: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', configuration_sha256: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', rollback_plan: 'Redeploy the previously verified artifact and run rollback health and smoke checks.' } }),
  operation('release', 'release.list', '列出发布申请', 'GET', '/api/v1/releases', '查看项目发布、晋级与回滚状态。', '项目成员', { params: [field('project', '项目', 'query', '$project'), field('limit', '数量', 'query', '30', { type: 'number' })] }),
  operation('release', 'release.get', '读取发布申请', 'GET', '/api/v1/releases/{id}', '查看源代码、审批、运行与供应链证据。', '项目成员', { params: [field('id', '发布 ID', 'path')] }),
  operation('release', 'release.events.list', '查看发布事件', 'GET', '/api/v1/releases/{id}/events', '读取源码晋级、部署与回滚审计。', '项目成员', { params: [field('id', '发布 ID', 'path')] }),
  operation('release', 'release.decision.submit', '审批发布', 'POST', '/api/v1/releases/{id}/decision', '在保护分支验证后独立批准或拒绝。', 'release_approver', { params: [field('id', '发布 ID', 'path')], body: { revision: 2, decision: 'approve', comment: '风险、变更单和回滚方案已确认。' } }),
  operation('release', 'release.trigger', '触发 CI/CD', 'POST', '/api/v1/releases/{id}/trigger', '回显固定确认值，调用白名单 CI/CD Broker。', 'release_engineer', { params: [field('id', '发布 ID', 'path')], body: { revision: 3, confirmation: 'replace-with-trigger-confirmation' } }),
  operation('release', 'release.rollback.request', '申请回滚', 'POST', '/api/v1/releases/{id}/rollback', '对成功发布创建回滚申请。', 'release_engineer', { params: [field('id', '发布 ID', 'path')], body: { revision: 6, reason: '错误率超过发布 SLO' } }),
  operation('release', 'release.rollback.decision', '审批回滚', 'POST', '/api/v1/releases/{id}/rollback-decision', '由独立审批人批准回滚。', 'release_approver', { params: [field('id', '发布 ID', 'path')], body: { revision: 7, decision: 'approve', comment: '回滚条件已满足。' } }),
  operation('release', 'release.rollback.trigger', '触发回滚', 'POST', '/api/v1/releases/{id}/rollback-trigger', '通过受控 Broker 触发回滚流水线。', 'release_engineer', { params: [field('id', '发布 ID', 'path')], body: { revision: 8, confirmation: 'replace-with-trigger-confirmation' } }),
  operation('release', 'release.webhook.code-platform', '模拟代码平台 Webhook', 'POST', '/api/v1/releases/webhooks/code-platform', '使用浏览器 Web Crypto 生成真实 HMAC 签名，验证保护分支晋级。', '代码平台 HMAC', { webhook: true, body: { event_id: 'code-platform-event-001', release_id: 'replace-with-release-id', pull_request_url: 'https://github.example.com/acme/order-service/pull/42', head_commit: '0123456789abcdef0123456789abcdef01234567', merge_commit: '89abcdef0123456789abcdef0123456789abcdef', protected_branch: true, approvals: 2, required_approvals: 2, checks_passed: true, merged: true } }),
  operation('release', 'release.webhook.provider', '模拟 CI/CD Webhook', 'POST', '/api/v1/releases/webhooks/provider', '签名上报运行状态、制品、SBOM 和九项发布证据。', 'CI/CD HMAC', { webhook: true, body: { event_id: 'cicd-event-001', release_id: 'replace-with-release-id', run_id: 'replace-with-provider-run-id', phase: 'deploy', status: 'running', message: 'pipeline running' } }),

  operation('quality', 'quality.metrics.get', '读取问答指标', 'GET', '/api/v1/observability/answer-metrics', '聚合请求、成功、拒答、耗时、Token 和成本。', 'knowledge_admin', { params: [field('project', '项目', 'query', '$project')] }),
  operation('quality', 'quality.traces.get', '读取问答轨迹', 'GET', '/api/v1/observability/answer-traces/{id}', '查看已脱敏的检索、生成和费用轨迹。', 'knowledge_admin', { params: [field('id', 'Trace ID', 'path')] }),
  operation('quality', 'quality.traces.prune', '清理过期轨迹', 'POST', '/api/v1/observability/answer-traces/prune', '按保留天数删除旧问答轨迹。', 'knowledge_admin', { body: { retention_days: 90 } }),
  operation('quality', 'quality.answers.evaluate', '即时答案评测', 'POST', '/api/v1/evaluations/answers', '直接运行一组答案评测用例。', 'knowledge_admin', { body: { cases: [{ name: '启动说明', query: '如何启动服务？', project: '$project', roles: [], expect_refused: false, required_sources: ['git://order-service/README.md'], answer_contains: ['go run'] }] } }),
  operation('quality', 'quality.suites.create', '创建评测套件', 'POST', '/api/v1/evaluations/suites', '发布版本化质量基线和最低通过率。', 'knowledge_admin', { body: { name: 'order-service-smoke', description: '订单服务知识问答冒烟套件', minimum_pass_rate: 0.9, cases: [{ name: '启动说明', query: '如何启动服务？', project: '$project', roles: [], expect_refused: false, required_sources: ['git://order-service/README.md'], answer_contains: ['go run'] }] } }),
  operation('quality', 'quality.suites.list', '列出评测套件', 'GET', '/api/v1/evaluations/suites', '按名称查看评测基线版本。', 'knowledge_admin', { params: [field('name', '名称', 'query', ''), field('limit', '数量', 'query', '30', { type: 'number' })] }),
  operation('quality', 'quality.suites.get', '读取评测套件', 'GET', '/api/v1/evaluations/suites/{id}', '查看完整用例、哈希和通过率。', 'knowledge_admin', { params: [field('id', '套件 ID', 'path')] }),
  operation('quality', 'quality.runs.create', '启动异步评测', 'POST', '/api/v1/evaluations/runs', '创建带租约恢复的评测门禁运行。', 'knowledge_admin', { body: { suite_id: 'replace-with-suite-id' } }),
  operation('quality', 'quality.runs.list', '列出评测运行', 'GET', '/api/v1/evaluations/runs', '查看运行状态、门禁、通过率和尝试次数。', 'knowledge_admin', { params: [field('suite_id', '套件 ID', 'query', ''), field('limit', '数量', 'query', '30', { type: 'number' })] }),
  operation('quality', 'quality.runs.get', '读取评测运行', 'GET', '/api/v1/evaluations/runs/{id}', '查看逐用例报告和失败原因。', 'knowledge_admin', { params: [field('id', '运行 ID', 'path')] }),
  operation('quality', 'quality.runs.cancel', '取消评测运行', 'POST', '/api/v1/evaluations/runs/{id}/cancel', '取消 pending/running 评测。', 'knowledge_admin', { params: [field('id', '运行 ID', 'path')] }),
  operation('quality', 'quality.runs.retry', '重试评测运行', 'POST', '/api/v1/evaluations/runs/{id}/retry', '从可重试终态创建受控新尝试。', 'knowledge_admin', { params: [field('id', '运行 ID', 'path')] })
];

const state = {
  profile: { apiBase: '', project: '', repository: '', userId: '', roles: '', token: '', environment: 'staging' },
  page: 'overview', currentOperation: null, history: [], lastResponse: null, moduleData: {}, operationValues: {}
};

const $ = selector => document.querySelector(selector);
const $$ = selector => [...document.querySelectorAll(selector)];
const esc = value => String(value ?? '').replace(/[&<>'"]/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[character]));
const pretty = value => JSON.stringify(value, null, 2);
const groupFor = id => GROUPS.find(group => group.id === id);
const operationFor = id => OPERATIONS.find(item => item.id === id);
const operationsFor = group => OPERATIONS.filter(item => item.group === group);

function restoreProfile() {
  try {
    const saved = JSON.parse(localStorage.getItem('ekbda-test-profile') || '{}');
    state.profile = { ...state.profile, ...saved, token: '' };
  } catch (_) { /* ignore malformed local profile */ }
}

function persistProfile() {
  const { token, ...safe } = state.profile;
  localStorage.setItem('ekbda-test-profile', JSON.stringify(safe));
}

function resolveToken(value) {
  const values = {
    '$project': state.profile.project,
    '$repository': state.profile.repository,
    '$environment': state.profile.environment
  };
  return values[value] ?? value;
}

function materialize(value) {
  if (Array.isArray(value)) return value.map(materialize);
  if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, materialize(item)]));
  return typeof value === 'string' ? resolveToken(value) : value;
}

function initNavigation() {
  const nav = $('#module-nav');
  nav.replaceChildren();
  GROUPS.forEach(group => {
    const button = document.createElement('button');
    button.className = 'nav-button';
    button.dataset.page = group.id;
    button.innerHTML = `<span class="nav-index">${group.index}</span><span>${esc(group.short)}</span><span class="nav-count">${operationsFor(group.id).length}</span>`;
    button.addEventListener('click', () => activatePage(group.id));
    nav.append(button);
  });
}

function initCoverage() {
  const band = $('#coverage-band');
  band.innerHTML = GROUPS.filter(group => group.id !== 'overview').map(group => `<div class="coverage-item"><span>${group.index}</span><strong>${esc(group.short)}</strong><i></i></div>`).join('');
  const stages = [
    ['01','知识','知识与新人上手'], ['02','规范','企业研发准则'], ['03','规划','需求与角色评审'], ['04','立项','七工件与追踪'],
    ['05','任务','Agent 运行时'], ['06','编码','8A / 8B'], ['07','交付','8C PR'], ['08','发布','8D 回滚']
  ];
  $('#delivery-chain').innerHTML = stages.map(([number,title,copy]) => `<div class="chain-step"><b>${number}</b><span>${title}</span><small>${copy}</small></div>`).join('');
}

function initModulePages() {
  const host = $('#module-pages');
  const template = $('#module-page-template');
  GROUPS.filter(group => group.id !== 'overview').forEach(group => {
    const fragment = template.content.cloneNode(true);
    const page = fragment.querySelector('.module-page');
    page.id = `page-${group.id}`;
    page.dataset.page = group.id;
    page.querySelector('.serial').textContent = `MODULE ${group.index} / ${operationsFor(group.id).length} OPERATIONS`;
    page.querySelector('h2').textContent = group.title;
    page.querySelector('.module-summary').textContent = group.description;
    const primary = operationFor(group.primary);
    const primaryButton = page.querySelector('.module-primary');
    primaryButton.textContent = primary ? `＋ ${primary.label}` : '执行操作';
    if (primary) primaryButton.addEventListener('click', () => openOperation(primary.id));
    page.querySelector('.module-refresh').addEventListener('click', () => loadModule(group.id, true));
    const list = page.querySelector('.operation-list');
    operationsFor(group.id).forEach(item => {
      const button = document.createElement('button');
      button.className = 'operation-button';
      button.innerHTML = `<b>${item.method}</b><span>${esc(item.label)}<small>${esc(item.path)}</small></span>`;
      button.addEventListener('click', () => openOperation(item.id));
      list.append(button);
    });
    page.querySelector('.operation-count').textContent = `${operationsFor(group.id).length} 项`;
    renderModuleKPIs(page, group, null);
    renderEmpty(page.querySelector('.module-data'), '尚未加载真实数据', `连接项目后刷新，或从右侧 ${operationsFor(group.id).length} 项操作中选择功能。`);
    host.append(fragment);
  });
}

function activatePage(id) {
  const group = groupFor(id) || GROUPS[0];
  state.page = group.id;
  $$('.nav-button').forEach(button => button.classList.toggle('active', button.dataset.page === group.id));
  $$('.workspace-page').forEach(page => page.classList.toggle('active', page.dataset.page === group.id));
  $('#page-title').textContent = group.title === '总览' ? '企业功能测试台' : group.title;
  $('#breadcrumb-module').textContent = group.title;
  history.replaceState(null, '', `#${group.id}`);
  if (group.id === 'overview') loadOverview(); else loadModule(group.id);
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function renderEmpty(target, title, copy) {
  target.innerHTML = `<div class="empty-ledger"><div><b>${esc(title)}</b><p>${esc(copy)}</p></div></div>`;
}

function renderModuleKPIs(page, group, payload) {
  const values = moduleKPIs(group.id, payload);
  page.querySelector('.module-kpis').innerHTML = values.map(item => `<div class="module-kpi"><span>${esc(item.label)}</span><strong>${esc(item.value)}</strong><small>${esc(item.copy)}</small></div>`).join('');
}

function moduleKPIs(group, payload) {
  const items = extractArray(payload);
  const statusCounts = countStatuses(items);
  const defaults = {
    knowledge: [['当前结果', items.length, '权限过滤后的对象'], ['已引用', sum(items, 'version'), '版本字段合计'], ['拒答', statusCounts.refused || 0, '可信边界'], ['接口覆盖', operationsFor(group).length, '全部可操作']],
    governance: [['当前对象', items.length, '策略/规范/报告'], ['通过', statusCounts.passed || statusCounts.completed || 0, '门禁成功'], ['阻断', statusCounts.failed || 0, '需处理结果'], ['接口覆盖', operationsFor(group).length, '全部可操作']],
    planning: [['规划会话', items.length, '当前项目'], ['待处理', pendingCount(items), '需要人工推进'], ['已批准', statusCounts.approved || 0, '规划终态'], ['角色', 5, '独立评审']],
    packages: [['立项包', items.length, '当前版本列表'], ['七工件', 7, '每个固定版本'], ['追踪覆盖', sum(items, 'version'), '版本累计'], ['接口覆盖', operationsFor(group).length, '全部可操作']],
    agents: [['任务总数', items.length, '当前筛选'], ['运行中', statusCounts.running || statusCounts.pending || 0, '含待领取'], ['已完成', statusCounts.completed || 0, '质量已记录'], ['失败/取消', (statusCounts.failed || 0) + (statusCounts.canceled || 0), '可受控处理']],
    development: [['开发会话', items.length, '8A—8C'], ['待审批', statusCounts.awaiting_approval || 0, '独立审批'], ['已验证', statusCounts.verified || 0, '隔离执行通过'], ['已交付', statusCounts.delivered || 0, 'PR 已创建']],
    release: [['发布申请', items.length, '8D 全状态'], ['待审批', statusCounts.awaiting_approval || 0, '发布/回滚'], ['成功', statusCounts.succeeded || 0, '证据完整'], ['已回滚', statusCounts.rolled_back || 0, '验证完成']],
    quality: [['评测运行', items.length, '当前筛选'], ['门禁通过', statusCounts.passed || 0, '质量基线'], ['运行中', statusCounts.running || statusCounts.pending || 0, '异步任务'], ['失败/取消', (statusCounts.failed || 0) + (statusCounts.canceled || 0), '需复核']]
  };
  return (defaults[group] || []).map(([label,value,copy]) => ({ label, value: value ?? 0, copy }));
}

function extractArray(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== 'object') return [];
  for (const key of ['results','documents','sessions','packages','tasks','releases','runs','suites','reports','validations','syncs','policies','events','reviews','commands']) {
    if (Array.isArray(payload[key])) return payload[key];
  }
  return [];
}

function countStatuses(items) {
  return items.reduce((counts, item) => {
    const status = String(item?.gate_status || item?.status || (item?.passed === true ? 'passed' : item?.passed === false ? 'failed' : '')).toLowerCase();
    if (status) counts[status] = (counts[status] || 0) + 1;
    return counts;
  }, {});
}

function pendingCount(items) { return items.filter(item => /awaiting|pending|running/.test(String(item?.status || ''))).length; }
function sum(items, key) { return items.reduce((total, item) => total + Number(item?.[key] || 0), 0); }

async function loadModule(groupID, notify = false) {
  const group = groupFor(groupID);
  if (!group?.list) return;
  const page = $(`#page-${groupID}`);
  const target = page.querySelector('.module-data');
  if (!state.profile.project && ['knowledge','planning','packages','agents','development','release'].includes(groupID)) {
    renderEmpty(target, '需要连接项目', '打开连接设置，填写项目和调用身份。');
    return;
  }
  target.innerHTML = '<div class="empty-ledger"><div><b>正在读取 API</b><p>只展示真实返回，不注入演示对象。</p></div></div>';
  try {
    const item = operationFor(group.list);
    const values = Object.fromEntries(item.params.map(param => [param.name, resolveToken(param.fallback)]));
    const result = await execute(item, values, null, { silent: true });
    state.moduleData[groupID] = result.body;
    renderModuleData(target, groupID, result.body);
    renderModuleKPIs(page, group, result.body);
    if (notify) toast(`${group.title}数据已刷新`);
  } catch (error) {
    renderEmpty(target, '读取失败', error.message);
    if (notify) toast(error.message, true);
  }
}

async function loadOverview() {
  const pulse = $('#overview-pulse');
  pulse.innerHTML = '<div class="pulse-cell"><small>CHECKING</small><strong>…</strong><span>正在检测真实 API</span></div>';
  const checks = [
    ['API 健康', operationFor('system.health'), {}],
    ['规划会话', operationFor('planning.sessions.list'), { project: state.profile.project, limit: '50' }],
    ['Agent 任务', operationFor('agents.list'), { project: state.profile.project, kind: '', status: '', limit: '50' }],
    ['开发会话', operationFor('development.sessions.list'), { project: state.profile.project, limit: '50' }],
    ['发布申请', operationFor('release.list'), { project: state.profile.project, limit: '50' }],
    ['评测运行', operationFor('quality.runs.list'), { suite_id: '', limit: '50' }]
  ];
  const results = await Promise.allSettled(checks.map(([,item,values]) => {
    if (!state.profile.project && item.id !== 'system.health' && item.id !== 'quality.runs.list') return Promise.reject(new Error('未设置项目'));
    return execute(item, values, null, { silent: true });
  }));
  const health = results[0].status === 'fulfilled';
  setConnectionState(health ? 'online' : 'error', health ? 'API 在线' : 'API 不可用');
  pulse.innerHTML = results.slice(0,4).map((result,index) => {
    const success = result.status === 'fulfilled';
    const body = success ? result.value.body : null;
    const count = index === 0 ? (success ? 'OK' : 'ERR') : extractArray(body).length;
    return `<div class="pulse-cell ${success ? 'good' : 'bad'}"><small>${esc(checks[index][0])}</small><strong>${esc(count)}</strong><span>${success ? `${result.value.status} · ${result.value.duration} ms` : esc(result.reason.message)}</span></div>`;
  }).join('');
  const tallies = checks.slice(1).map((check,index) => ({ label: check[0], value: results[index+1].status === 'fulfilled' ? extractArray(results[index+1].value.body).length : '—' }));
  $('#overview-tallies').innerHTML = tallies.map(item => `<div class="tally-row"><span>${esc(item.label)}</span><strong>${esc(item.value)}</strong></div>`).join('');
  renderMiniHistory();
}

function openOperation(id) {
  const item = operationFor(id);
  if (!item) return;
  state.currentOperation = item;
  $('#operation-serial').textContent = `${item.group.toUpperCase()} / ${item.id}`;
  $('#operation-title').textContent = item.label;
  $('#operation-method').textContent = item.method;
  $('#operation-path').textContent = item.path;
  $('#operation-role').textContent = item.role;
  $('#operation-description').textContent = item.description;
  $('#operation-error').textContent = '';
  const fields = $('#operation-fields');
  fields.replaceChildren();
  item.params.forEach(param => {
    const label = document.createElement('label');
    label.textContent = param.label;
    const input = param.choices ? document.createElement('select') : document.createElement('input');
    input.name = param.name;
    input.dataset.location = param.location;
    if (param.type) input.type = param.type;
    const remembered = state.operationValues[item.id]?.[param.name];
    const value = remembered ?? resolveToken(param.fallback);
    if (param.choices) param.choices.forEach(choice => { const option = document.createElement('option'); option.value = choice; option.textContent = choice; input.append(option); });
    input.value = value || '';
    label.append(input);
    fields.append(label);
  });
  const hasBody = item.body !== null;
  $('#operation-body-wrap').classList.toggle('hidden', !hasBody);
  $('#operation-body').value = hasBody ? pretty(materialize(item.body)) : '';
  $('#operation-secret-wrap').classList.toggle('hidden', !item.webhook);
  $('#operation-secret').value = '';
  $('#operation-dialog').showModal();
}

async function executeCurrent(event) {
  event.preventDefault();
  const item = state.currentOperation;
  if (!item) return;
  const values = {};
  $('#operation-fields').querySelectorAll('input,select').forEach(input => values[input.name] = input.value.trim());
  state.operationValues[item.id] = values;
  let body = null;
  if (item.body !== null) {
    try { body = JSON.parse($('#operation-body').value); }
    catch (error) { $('#operation-error').textContent = `JSON 无效：${error.message}`; return; }
  }
  const secret = item.webhook ? $('#operation-secret').value : '';
  if (item.webhook && secret.length < 32) { $('#operation-error').textContent = 'Webhook 密钥至少需要 32 个字符。'; return; }
  const button = $('#execute-operation');
  button.disabled = true;
  button.textContent = '请求中…';
  try {
    const result = await execute(item, values, body, { secret });
    $('#operation-dialog').close();
    showResult(result);
    if (item.group !== 'overview') {
      const page = $(`#page-${item.group}`);
      renderModuleData(page.querySelector('.module-data'), item.group, result.body);
      renderModuleKPIs(page, groupFor(item.group), result.body);
    }
    toast(`${item.label}完成 · HTTP ${result.status}`);
  } catch (error) {
    $('#operation-error').textContent = error.message;
    if (error.result) showResult(error.result);
    toast(error.message, true);
  } finally {
    button.disabled = false;
    button.textContent = '发送请求';
  }
}

async function execute(item, values = {}, body = null, options = {}) {
  let path = item.path;
  item.params.filter(param => param.location === 'path').forEach(param => {
    const value = values[param.name] ?? resolveToken(param.fallback);
    if (!value) throw new Error(`${param.label}不能为空`);
    path = path.replace(`{${param.name}}`, encodeURIComponent(value));
  });
  const query = new URLSearchParams();
  item.params.filter(param => param.location === 'query').forEach(param => {
    const value = values[param.name] ?? resolveToken(param.fallback);
    if (value !== '' && value !== undefined && value !== null) query.set(param.name, value);
  });
  const urlPath = `${path}${query.size ? `?${query}` : ''}`;
  const url = `${state.profile.apiBase.replace(/\/$/, '')}${urlPath}`;
  const headers = item.webhook ? {} : authHeaders();
  let encodedBody;
  if (body !== null) {
    encodedBody = JSON.stringify(body);
    headers['Content-Type'] = 'application/json; charset=utf-8';
  }
  if (item.webhook) {
    const timestamp = String(Math.floor(Date.now() / 1000));
    headers['X-EKBDA-Timestamp'] = timestamp;
    headers['X-EKBDA-Signature'] = await hmacSignature(options.secret, timestamp, encodedBody || '');
  }
  const started = performance.now();
  let response;
  try { response = await fetch(url, { method: item.method, headers, body: encodedBody }); }
  catch (error) { throw new Error(`无法连接 API：${error.message}`); }
  const duration = Math.round(performance.now() - started);
  const contentType = response.headers.get('Content-Type') || '';
  let responseBody;
  if (item.download && response.ok && !contentType.includes('json')) {
    const blob = await response.blob();
    const disposition = response.headers.get('Content-Disposition') || '';
    const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] || `ekbda-export-${Date.now()}`;
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob); link.download = filename; link.click();
    setTimeout(() => URL.revokeObjectURL(link.href), 1000);
    responseBody = { downloaded: true, filename, content_type: contentType, bytes: blob.size };
  } else if (contentType.includes('json')) {
    responseBody = await response.json().catch(() => ({}));
  } else {
    responseBody = { text: await response.text() };
  }
  const result = { operation: item, status: response.status, ok: response.ok, duration, url: urlPath, request: { method: item.method, path: urlPath, body }, body: responseBody, at: new Date().toISOString() };
  if (!options.silent) recordHistory(result);
  if (!response.ok) {
    const error = new Error(responseBody?.error || `请求失败（HTTP ${response.status}）`);
    error.result = result;
    throw error;
  }
  return result;
}

function authHeaders() {
  const headers = {};
  if (state.profile.token) headers.Authorization = `Bearer ${state.profile.token}`;
  else {
    if (state.profile.userId) headers['X-User-ID'] = state.profile.userId;
    if (state.profile.roles) headers['X-User-Roles'] = state.profile.roles;
  }
  return headers;
}

async function hmacSignature(secret, timestamp, body) {
  if (!window.crypto?.subtle) throw new Error('当前浏览器不支持 Web Crypto；请使用 localhost 或 HTTPS。');
  const encoder = new TextEncoder();
  const key = await crypto.subtle.importKey('raw', encoder.encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const signature = await crypto.subtle.sign('HMAC', key, encoder.encode(`${timestamp}.${body}`));
  return `sha256=${[...new Uint8Array(signature)].map(byte => byte.toString(16).padStart(2, '0')).join('')}`;
}

function recordHistory(result) {
  state.history.unshift(result);
  state.history = state.history.slice(0, 50);
  $('#history-count').textContent = state.history.length;
  renderMiniHistory();
  renderHistory();
}

function renderMiniHistory() {
  const target = $('#mini-history');
  if (!state.history.length) { target.innerHTML = '<div class="history-mini-row"><span>本次页面会话暂无调用。</span></div>'; return; }
  target.innerHTML = state.history.slice(0,5).map(item => `<div class="history-mini-row"><b>${item.status}</b><span>${esc(item.operation.label)}</span><small>${esc(item.operation.method)} ${esc(item.url)} · ${item.duration} ms</small></div>`).join('');
}

function renderHistory() {
  const target = $('#history-list');
  target.replaceChildren();
  state.history.forEach((item,index) => {
    const row = document.createElement('article');
    row.className = 'history-entry';
    row.innerHTML = `<h4>${esc(item.operation.label)}</h4><p><b class="${item.ok ? '' : 'error'}">${item.status}</b>${esc(item.operation.method)} ${esc(item.url)} · ${item.duration} ms</p>`;
    row.addEventListener('click', () => { closeDrawers(); showResult(state.history[index]); });
    target.append(row);
  });
}

function showResult(result) {
  state.lastResponse = result;
  $('#result-title').textContent = result.operation.label;
  const status = $('#result-status');
  status.textContent = `HTTP ${result.status}`;
  status.classList.toggle('error', !result.ok);
  $('#result-time').textContent = `${result.duration} ms`;
  $('#result-endpoint').textContent = `${result.operation.method} ${result.url}`;
  $('#result-raw').textContent = pretty(result.body);
  $('#result-request').textContent = pretty(result.request);
  renderVisual($('#result-visual'), result.operation.group, result.body);
  $$('.response-tabs button').forEach(button => button.classList.toggle('active', button.dataset.resultTab === 'visual'));
  $$('.result-panel').forEach(panel => panel.classList.toggle('active', panel.id === 'result-visual'));
  openDrawer($('#result-drawer'));
}

function renderModuleData(target, group, payload) {
  if (!payload || (typeof payload === 'object' && !Object.keys(payload).length)) { renderEmpty(target, '响应为空', '接口调用成功，但没有可展示对象。'); return; }
  renderVisual(target, group, payload);
}

function renderVisual(target, group, payload) {
  target.replaceChildren();
  const array = extractArray(payload);
  if (array.length || Array.isArray(payload)) {
    target.innerHTML = renderObjectList(array.length ? array : payload, group);
    return;
  }
  if (payload === null || typeof payload !== 'object') { target.innerHTML = `<div class="kv-item"><span>VALUE</span><strong>${esc(payload)}</strong></div>`; return; }
  const special = specialVisualization(group, payload);
  const summary = document.createElement('div');
  summary.className = 'kv-grid';
  Object.entries(payload).filter(([,value]) => value === null || ['string','number','boolean'].includes(typeof value)).slice(0,16).forEach(([key,value]) => {
    const item = document.createElement('div'); item.className = 'kv-item'; item.innerHTML = `<span>${esc(labelize(key))}</span><strong>${esc(formatValue(value))}</strong>`; summary.append(item);
  });
  if (summary.childNodes.length) target.append(summary);
  if (special) { const holder = document.createElement('div'); holder.innerHTML = special; target.append(...holder.childNodes); }
  const nested = Object.entries(payload).filter(([key,value]) => value && typeof value === 'object' && !['citations','plan','role_review','artifacts','traceability','checks','events','violations','results'].includes(key));
  if (!special && nested.length) {
    const section = document.createElement('section'); section.className = 'visual-section'; section.innerHTML = '<h4>NESTED DATA</h4>';
    const pre = document.createElement('pre'); pre.className = 'result-panel active'; pre.textContent = pretty(Object.fromEntries(nested)); section.append(pre); target.append(section);
  }
}

function renderObjectList(items, group) {
  if (!items.length) return '<div class="empty-ledger"><div><b>暂无对象</b><p>接口返回了空列表。</p></div></div>';
  return `<div class="object-list">${items.map(item => {
    if (item === null || typeof item !== 'object') return `<article class="object-row"><div><h4>${esc(item)}</h4></div></article>`;
    const citation = item.citation || {};
    const title = item.title || citation.title || item.name || item.suite_name || item.kind || item.project || item.path || item.type || item.id || '未命名对象';
    const description = item.snippet || item.summary || item.description || item.requirement || item.error_message || item.source_uri || citation.source_uri || item.repository || item.comment || '';
    const status = item.gate_status || item.status || item.reranker || (typeof item.passed === 'boolean' ? (item.passed ? 'passed' : 'failed') : '');
    const score = typeof item.score === 'number' ? item.score.toFixed(4) : undefined;
    const lines = citation.start_line ? `L${citation.start_line}—${citation.end_line || citation.start_line}` : undefined;
    const meta = [['score',score],['revision',item.revision],['version',item.version ?? citation.version],['lines',lines],['created',formatDate(item.created_at || item.updated_at)],['project',item.project]].filter(([,value]) => value !== undefined && value !== '' && value !== null);
    const objectID = item.id || item.document_id || item.package_id || citation.document_id || '';
    return `<article class="object-row"><div><h4>${esc(title)}</h4><p>${esc(description)}</p><div class="object-meta">${status ? `<span class="status-pill ${statusClass(status)}">${esc(status)}</span>` : ''}${meta.map(([key,value]) => `<span>${esc(key)} ${esc(value)}</span>`).join('')}</div></div><code class="object-id" title="点击复制" data-copy="${esc(objectID)}">${esc(objectID || '—')}</code></article>`;
  }).join('')}</div>`;
}

function specialVisualization(group, payload) {
  const sections = [];
  if (payload.status) sections.push(renderWorkflow(group, payload.status));
  if (group === 'knowledge' && payload.answer !== undefined) {
    sections.push(`<section class="visual-section"><h4>ANSWER</h4><div class="artifact-card"><b>${payload.refused ? '已拒答' : '可信回答'}</b><p>${esc(payload.answer || payload.refusal_reason)}</p></div></section>`);
    if (payload.citations?.length) sections.push(`<section class="visual-section"><h4>CITATIONS · ${payload.citations.length}</h4>${payload.citations.map(item => `<div class="citation-card"><b>${esc(item.citation?.title || item.id)}</b><p>${esc(item.snippet || '')}</p><p>${esc(item.citation?.source_uri || '')} · L${esc(item.citation?.start_line)}—${esc(item.citation?.end_line)}</p></div>`).join('')}</section>`);
  }
  if (group === 'planning') {
    if (payload.questions?.length) sections.push(`<section class="visual-section"><h4>CLARIFICATION QUESTIONS</h4>${payload.questions.map(item => `<div class="finding-card"><b>${esc(item.question)}</b><p>${esc(item.reason)} · ID ${esc(item.id)}</p></div>`).join('')}</section>`);
    if (payload.plan?.steps?.length) sections.push(`<section class="visual-section"><h4>IMPLEMENTATION PLAN</h4>${payload.plan.steps.map((step,index) => `<div class="artifact-card"><b>${String(index+1).padStart(2,'0')} · ${esc(step.title)}</b><p>${esc(step.description)}</p><p>交付：${esc((step.deliverables || []).join('、'))}</p></div>`).join('')}</section>`);
    if (payload.role_review?.reviews?.length) sections.push(`<section class="visual-section"><h4>ROLE REVIEWS</h4>${payload.role_review.reviews.map(review => `<div class="finding-card"><b>${esc(review.role)} · ${esc(review.recommendation)}</b><p>${esc(review.summary)}</p></div>`).join('')}</section>`);
  }
  if (group === 'packages') {
    if (payload.artifacts?.length) sections.push(`<section class="visual-section"><h4>SEVEN ARTIFACTS</h4>${payload.artifacts.map(item => `<div class="artifact-card"><b>${esc(item.type)} · ${esc(item.title)}</b><p>${esc(item.summary)}</p><p>${esc((item.sections || []).map(section => section.name).join(' / '))}</p></div>`).join('')}</section>`);
    if (payload.traceability?.length) sections.push(renderTraceability(payload.traceability));
  }
  if (payload.checks?.length) sections.push(`<section class="visual-section"><h4>CHECK EVIDENCE</h4>${payload.checks.map(item => `<div class="finding-card"><b>${esc(item.name)} · ${esc(item.status || (item.passed ? 'passed' : 'failed'))}</b><p>${esc(item.evidence_uri || item.details || item.sha256 || '')}</p></div>`).join('')}</section>`);
  if (payload.violations?.length) sections.push(`<section class="visual-section"><h4>VIOLATIONS</h4>${payload.violations.map(item => `<div class="finding-card"><b>${esc(item.level)} · ${esc(item.rule_title)}</b><p>${esc(item.path)} ${esc(item.message)}</p></div>`).join('')}</section>`);
  if (payload.events?.length) sections.push(renderEvents(payload.events));
  if (payload.results?.length && group === 'quality') sections.push(`<section class="visual-section"><h4>CASE RESULTS</h4>${payload.results.map(item => `<div class="finding-card"><b>${item.passed ? 'PASS' : 'FAIL'} · ${esc(item.name)}</b><p>${esc((item.failures || []).join('；') || item.trace_id || '')}</p></div>`).join('')}</section>`);
  return sections.join('');
}

function renderWorkflow(group, current) {
  const workflows = {
    planning: ['awaiting_clarification','awaiting_role_review','awaiting_resolution','awaiting_approval','approved'],
    development: ['draft','awaiting_approval','approved','executing','verified','delivering','delivered'],
    release: ['awaiting_source_verification','awaiting_approval','approved','queued','running','succeeded','rollback_awaiting_approval','rollback_approved','rollback_queued','rollback_running','rolled_back'],
    agents: ['pending','running','completed'], quality: ['pending','running','completed']
  };
  const flow = workflows[group];
  if (!flow) return '';
  const currentIndex = flow.indexOf(current);
  return `<section class="visual-section"><h4>STATE WORKFLOW</h4><div class="workflow-rail">${flow.map((status,index) => `<span class="workflow-state ${status === current ? 'current' : currentIndex > index ? 'done' : ''}">${esc(status)}</span>`).join('')}</div></section>`;
}

function renderTraceability(items) {
  return `<section class="visual-section"><h4>TRACEABILITY MATRIX</h4><table class="trace-table"><thead><tr><th>REQ</th><th>Architecture</th><th>API</th><th>Test</th><th>Deploy</th><th>Status</th></tr></thead><tbody>${items.map(item => `<tr><td>${esc(item.requirement_id)}</td><td>${esc((item.architecture_sections || []).join(', '))}</td><td>${esc((item.api_sections || []).join(', ') || item.api_not_applicable_reason)}</td><td>${esc((item.test_sections || []).join(', '))}</td><td>${esc((item.deployment_sections || []).join(', '))}</td><td>${esc(item.coverage_status)}</td></tr>`).join('')}</tbody></table></section>`;
}

function renderEvents(items) {
  return `<section class="visual-section"><h4>EVENT TIMELINE</h4>${items.map(item => `<div class="artifact-card"><b>${esc(item.sequence)} · ${esc(item.type)}</b><p>${esc(item.from_status)} → ${esc(item.to_status)} · ${esc(item.actor)}</p><p>${esc(item.reason || '')}</p></div>`).join('')}</section>`;
}

function labelize(key) { return key.replaceAll('_',' ').toUpperCase(); }
function formatValue(value) { if (typeof value === 'boolean') return value ? 'YES' : 'NO'; if (typeof value === 'number') return Number.isInteger(value) ? value : value.toFixed(4); return value || '—'; }
function formatDate(value) { if (!value) return ''; const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit' }); }
function statusClass(status) { status = String(status).toLowerCase(); if (/passed|completed|approved|delivered|succeeded|rolled_back|active/.test(status)) return 'good'; if (/failed|rejected|canceled|deleted|error/.test(status)) return 'bad'; if (/draft|unknown/.test(status)) return 'neutral'; return ''; }

function openSettings() {
  const form = $('#settings-form');
  Object.entries(state.profile).forEach(([key,value]) => { if (form.elements[key]) form.elements[key].value = value || ''; });
  $('#settings-error').textContent = '';
  $('#settings-dialog').showModal();
}

async function saveSettings(event) {
  event.preventDefault();
  const data = new FormData(event.currentTarget);
  const profile = Object.fromEntries([...data.entries()].map(([key,value]) => [key,String(value).trim()]));
  if (!profile.project) { $('#settings-error').textContent = '项目标识不能为空。'; return; }
  if (!profile.token && !profile.userId) { $('#settings-error').textContent = '请填写用户 ID 或 Bearer Token。'; return; }
  state.profile = { ...state.profile, ...profile };
  persistProfile();
  updateIdentity();
  $('#settings-dialog').close();
  await loadOverview();
  if (state.page !== 'overview') await loadModule(state.page, true);
}

function updateIdentity() {
  $('#rail-project').textContent = state.profile.project || '未连接';
  $('#identity-name').textContent = state.profile.userId || (state.profile.token ? 'JWT 用户' : '未配置');
  $('#identity-avatar').textContent = (state.profile.userId || 'JWT').slice(0,2).toUpperCase();
}

function setConnectionState(kind, text) {
  $('#connection-light').className = `context-light ${kind}`;
  $('#api-state').className = `api-state ${kind}`;
  $('#api-state span').textContent = text;
}

function openDrawer(drawer) { closeDrawers(); drawer.classList.add('open'); drawer.setAttribute('aria-hidden','false'); $('#scrim').classList.add('visible'); }
function closeDrawers() { $$('.result-drawer,.history-drawer').forEach(drawer => { drawer.classList.remove('open'); drawer.setAttribute('aria-hidden','true'); }); $('#scrim').classList.remove('visible'); }

function toast(message, error = false) {
  const item = document.createElement('div'); item.className = `toast ${error ? 'error' : ''}`; item.textContent = message; $('#toast-stack').append(item); setTimeout(() => item.remove(), 4200);
}

function bindEvents() {
  $('#open-settings').addEventListener('click', openSettings);
  $('#open-settings-top').addEventListener('click', openSettings);
  $('#settings-form').addEventListener('submit', saveSettings);
  $('#operation-form').addEventListener('submit', executeCurrent);
  $('#show-history').addEventListener('click', () => openDrawer($('#history-drawer')));
  $('#close-result').addEventListener('click', closeDrawers);
  $('#close-history').addEventListener('click', closeDrawers);
  $('#scrim').addEventListener('click', closeDrawers);
  $$('[data-refresh="overview"]').forEach(button => button.addEventListener('click', loadOverview));
  $$('.response-tabs button').forEach(button => button.addEventListener('click', () => {
    $$('.response-tabs button').forEach(item => item.classList.toggle('active', item === button));
    $$('.result-panel').forEach(panel => panel.classList.toggle('active', panel.id === `result-${button.dataset.resultTab}`));
  }));
  document.addEventListener('click', event => {
    const copy = event.target.closest('[data-copy]');
    if (copy?.dataset.copy) navigator.clipboard?.writeText(copy.dataset.copy).then(() => toast('ID 已复制'));
  });
  addEventListener('hashchange', () => activatePage(location.hash.slice(1) || 'overview'));
}

function boot() {
  restoreProfile();
  initNavigation();
  initCoverage();
  initModulePages();
  bindEvents();
  updateIdentity();
  renderMiniHistory();
  activatePage(location.hash.slice(1) || 'overview');
  if (!state.profile.project || (!state.profile.userId && !state.profile.token)) setTimeout(openSettings, 220);
}

boot();
