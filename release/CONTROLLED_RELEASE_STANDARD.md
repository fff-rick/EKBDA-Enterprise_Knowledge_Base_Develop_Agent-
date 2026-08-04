# 企业受控发布与回滚标准（阶段 8D）

## 1. 目标与边界

阶段 8D 将阶段 8C 的受控 PR 交付接入代码平台和 CI/CD。EKBDA 只编排已配置白名单内的流水线，不接受任意 URL、Shell、部署命令或生产凭据，也不直接登录服务器、容器平台或生产数据库。

发布链路为：`8C delivered → 代码平台可信晋级 → 发布审批 → CI/CD 触发 → 签名回调对账 → 证据门禁 → succeeded/failed`。回滚使用独立申请、独立审批和独立流水线运行。

## 2. 强制控制

1. 发布源必须是 `delivered` 且交付结果为 `passed` 的开发会话，Commit 和 HTTPS PR 地址来自 8C 证据，调用方不能覆盖。
2. 创建后状态为 `awaiting_source_verification`。只有代码平台 HMAC 回调同时证明 PR 已合并、目标为保护分支、必需检查通过且实际审批数不少于平台要求，才固定 Merge Commit 并进入 `awaiting_approval`。
3. `release_engineer` 创建和触发；`release_approver` 独立审批；创建者不能审批自己的发布。生产应使用 JWT 与项目 ACL，`dev_headers` 不得启用。
4. Pipeline、Environment 和 Provider Base URL 由启动配置固定。Provider URL 必须为 HTTPS；访问令牌仅存在服务端，API 响应和事件不保存令牌。
5. CI/CD 请求携带稳定 `Idempotency-Key`、固定 Merge Commit、Manifest/Configuration 哈希、变更单号、必需门禁及供应链要求。
6. CI/CD 回调使用与代码平台不同的至少 32 字节密钥，签名内容为 `<unix_timestamp>.<raw_body>`，默认只接受前后 5 分钟窗口。
7. Provider `succeeded` 不是发布成功的充分条件。必须同时提供 SHA-256 制品摘要、HTTPS 制品地址、SBOM 地址及哈希、签名验证、来源证明验证，以及九项带 HTTPS 证据地址和 SHA-256 的门禁结果。
8. 九项固定门禁为：`configuration`、`secret_scan`、`image_scan`、`migration`、`monitoring`、`rollback`、`health`、`smoke`、`logs`。缺项、失败或伪造成功回调都会得到 `release_evidence_failed`。
9. `production` 是受控生产环境名。生产发布必须通过 `promote_from_release_id` 引用同项目、同源 Commit、非生产且成功的发布，并固定复用其已验证制品摘要；回调摘要不一致则失败关闭。
10. 发布成功后才能申请回滚。回滚申请人不能审批自己的回滚；回滚成功必须至少具有 `health` 和 `smoke` 两项可信证据。

## 3. 状态机

```text
awaiting_source_verification
  -> awaiting_approval -> approved/rejected
  -> queued -> running -> succeeded/failed
succeeded
  -> rollback_awaiting_approval -> rollback_approved
  -> rollback_queued -> rollback_running -> rolled_back/rollback_failed
```

乱序或终态后的 Provider 回调会记录为 `provider_event_ignored`，但不会造成状态倒退。相同 `event_id + raw body` 返回 `applied=false`；相同事件 ID 携带不同内容或绑定其他发布时拒绝并报警。

## 4. 配置

```dotenv
EKBDA_RELEASE_ENABLED=true
EKBDA_RELEASE_PROVIDER_BASE_URL=https://cicd-broker.example.com
EKBDA_RELEASE_PROVIDER_TOKEN=use-secret-manager-reference
EKBDA_RELEASE_CODE_WEBHOOK_SECRET=at-least-32-random-bytes
EKBDA_RELEASE_WEBHOOK_SECRET=another-at-least-32-random-bytes
EKBDA_RELEASE_PIPELINES=standard-build-deploy
EKBDA_RELEASE_ENVIRONMENTS=staging,production
EKBDA_RELEASE_TIMEOUT_SECONDS=120
EKBDA_RELEASE_WEBHOOK_MAX_AGE_SECONDS=300
```

生产中密钥必须由 Secret Manager 注入，不得写入 `.env`、镜像或配置仓库。CI/CD Broker 实现 `POST /api/v1/runs` 与 `POST /api/v1/rollbacks`，并对 `Idempotency-Key` 做强幂等。

## 5. API 与角色

| API | 角色/身份 | 说明 |
|---|---|---|
| `GET /api/v1/releases/catalog` | 已认证项目成员 | 白名单与固定门禁 |
| `POST /api/v1/releases` | `release_engineer` | 创建发布请求 |
| `GET /api/v1/releases?project=...` | 已认证项目成员 | 查询发布 |
| `GET /api/v1/releases/{id}`、`/events` | 已认证项目成员 | 查询证据和不可变事件 |
| `POST /api/v1/releases/{id}/decision` | `release_approver` | 独立发布审批 |
| `POST /api/v1/releases/{id}/trigger` | `release_engineer` | 回显 `trigger_confirmation` 后触发 |
| `POST /api/v1/releases/{id}/rollback` | `release_engineer` | 申请回滚 |
| `POST /api/v1/releases/{id}/rollback-decision` | `release_approver` | 独立回滚审批 |
| `POST /api/v1/releases/{id}/rollback-trigger` | `release_engineer` | 触发回滚流水线 |
| `POST /api/v1/releases/webhooks/code-platform` | HMAC 代码平台 | 源码晋级与保护分支证据 |
| `POST /api/v1/releases/webhooks/provider` | HMAC CI/CD Provider | 运行状态和供应链证据 |

示例请求和回调位于本目录。PowerShell 示例：

```powershell
$created = .\scripts\create-release.ps1 -InputPath '.\release\order-service.release.example.json' | ConvertFrom-Json
# 代码平台回调通过后重新 GET，取得 awaiting_approval、revision 和 trigger_confirmation
. .\scripts\auth.ps1
$headers = New-EKBDAHeaders -UserID 'release-engineer-1' -Roles 'release_engineer'
$release = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/releases/$($created.id)" -Headers $headers
$approved = .\scripts\decide-release.ps1 -ReleaseID $release.id -Revision $release.revision | ConvertFrom-Json
.\scripts\trigger-release.ps1 -ReleaseID $approved.id -Revision $approved.revision -Confirmation $approved.trigger_confirmation
```

PowerShell 命令中不要输入 Markdown 围栏行（例如 `````powershell``）。JSON 文件和脚本统一以 UTF-8 字节发送，避免中文乱码。

`release/templates/` 提供摘要固定的 Go 多阶段 Dockerfile、非特权 Kubernetes Deployment、无秘密环境变量样例和回滚预案模板。模板中的占位符必须由企业模板流水线解析并重新计算 Manifest/Configuration SHA-256；API 不接收或渲染任意模板内容。

## 6. 持久化与审计

Memory 与 PostgreSQL 使用相同状态语义。PostgreSQL 以事务同时更新发布快照、追加事件并登记全局 Provider Event ID；所有人工写入使用 `revision` 乐观锁。记录包含源 Commit、Merge Commit、PR、变更单、环境、流水线、Manifest/Configuration 哈希、审批人、运行 ID、制品/SBOM/签名/来源证据、门禁、回滚及时间戳，不记录原始凭据或扫描报告内容。

## 7. 阶段验收

- 未完成保护分支合并证明不能审批或触发。
- 自审批、过期修订、错误确认词、非白名单环境/流水线全部拒绝。
- 重复或乱序 Webhook 不重复执行且不回退状态；事件 ID 内容冲突拒绝。
- 缺少签名、来源、SBOM 或任一固定门禁时，Provider 成功回调仍使发布失败。
- 生产只能晋级已成功验证的非生产制品，摘要必须完全一致。
- 回滚有独立申请、审批、运行和验证证据。
- 应用不持有生产登录入口，实际发布/回滚只能由最小权限 CI/CD Broker 执行。
