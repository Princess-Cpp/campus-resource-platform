# M3 第一轮：文本报修与本人查询

日期：2026-10-22。代码基线 `aff197af384931162d8d46ec3f2b8d86911d7c91`（main 的 PR #7 目录调整），实现时本地分支 `codex/m3-alignment`；后续按用户要求统一交付到团队仓库的 `feat/m3-reporting`。依据任务书、总约定 v0.2、需求基线 v0.4、范围分级及现有 M1/M2 公共约定实施。

用户本轮授权按现有公共依赖推进本地开发。此授权允许本地实现，不代表三组已评审 schema、权限映射或业务参数；DEC-02 至 DEC-12 和需求 approval_status 保持原状态。本文记录实际实现和自动验证，人工验收待进行，不作为已提交、已部署或全模块完成证明。

## 本轮实现及边界

正式 React 工作台 → Gin HTTP → 共享 MySQL 连接池，已打通文本上报、本人列表、筛选、分页、详情及上报时间线。复用已合入的 `maintenance/domain`，不另造业务状态机；上报成功为 `REPORTED`（待派单），`syncState=IDLE`，尚无资源操作。

| 任务 | 需求 ID | 本轮结果 | 验收案例与真实证据 |
| --- | --- | --- | --- |
| 文本报修 | M3-01.1 | 已实现本地 SQL/API/页面 | 授权用户选择空间、可选座位、类别和描述；生成编号、空间/参数快照；`TestReportingPersistsAndMigrationReplays`、浏览器实际提交 |
| 字段与空间范围 | M3-01.1 | 使用领域规则与显式目录适配器 | 描述过短、非法类别、座位不属于空间拒绝；异院系及注销空间不暴露；停用空间允许报修；`TestRoomAccessAndReportValidation` 的 6 个子案例 |
| 认证与服务端权限 | M3-01.1、M3-01.7 | 真实 Cookie 会话/CSRF，学生报修权限为开发映射 | 未登录 401、缺少 CSRF 403、body 伪造 reporterId 拒绝；撤权后同键重放 403、禁用后读取 401；`TestAuthenticationCSRFAndBodyCannotSpoofIdentity`、`TestRevocationAndDisableApplyToReplay` |
| 幂等、并发及结果未知 | M3-01.1 | 命令与结果持久化 | 同用户同键 12 路并发仅一单/事件；同键异内容 409；不同用户和不同键独立；提交成功但 HTTP 回包断开后，重建服务并同键核对不重复；`TestIdempotentConcurrentReportsAndChangedContent`、`TestLostHTTPReplyReconcilesAfterServiceRecreation` |
| 历史请求恢复 | M3-01.1 | 先核对已提交命令，再查当前目录 | 空间后来注销，相同键相同请求仍返回原单；新键报修被拒绝；每次仍检查当前报修权限；`TestReplaySurvivesLaterRoomRetirement` |
| 本人列表和详情 | M3-01.7 | 本轮子范围已实现 | 服务端按 reporter_id 限制；他人详情 404、列表无他人单；状态/类别/带时区日期过滤，稳定分页；未知/重复参数拒绝；`TestOwnerQueriesFiltersAndPagination`；浏览器类别筛选及清除验证 |
| 事务事件 | M3-02.2 | M3 本地开发适配器已实现，公共接入/投递未实现 | 工单、命令、审计事实、outbox 同事务；真实 MySQL trigger 使 outbox 写入失败时全部回滚，同键重试成功；`TestEventFailureRollsBackEntireReport` |
| 前端防重复与重试 | M3-01.1 | 已实现 | 单次发送锁、未知结果沿用请求和幂等键、按登录用户隔离 sessionStorage；页面重建保留未决请求；存储不可用/损坏时阻止新请求；`reporting.test.ts`、`transport.test.ts` |

图片、相似工单管理继续 postponed，无上传入口。派单/接单/重派、区域列表/待办/统计、维修下架、完工/验收/恢复、取消、评价、超时任务仍未接 SQL/API/页面；现有对应领域测试不等于完整实现。

## 状态与权限

| 请求/状态 | 服务端权限及范围 | 数据效果 |
| --- | --- | --- |
| GET session | 有效、当前未禁用的真实会话 | 只返回自己的 ID、有效权限及开发标识 |
| GET rooms | workorder.report；公共空间、同院系或授权区域；仅 ACTIVE/DISABLED | 只读目录；不修改空间 |
| POST workorders | 每次检查 workorder.report，用户身份来自会话；当前可访问空间 | 初次成功生成 REPORTED/version=1；同键重试返回原始创建结果 |
| GET workorders / :id | 有效会话且记录属于本人 | 只读；他人详情返回 404；无区域调度入口 |
| REPORTED → 后续状态 | 本轮无动作端点 | 不提供假派单、假维修或假验收成功 |

拟用描述 10–1000 字和严重程度 NORMAL/URGENT 均沿用现有提案与领域规则，未新增组内业务决定。紧急标记不自动触发派单或权限提升。创建之外的 expectedVersion/动作事务由下一轮实现，不能以当前创建测试代替。

## 数据库与事务

新增 `backend/migrations/006_m3_reporting.sql`，此前最新迁移为 005；迁移编号和 schema 仍待团队 review，后续同步前应再检查编号。只新增 M3 表，不改 001–005 或其他模块业务结构。

| 表 | 保存内容 | 关键约束/索引 |
| --- | --- | --- |
| m3_workorders | 报修人、空间/区域、类别、状态、版本、完整领域 JSON 快照、座位名称快照、UTC 时间 | id 主键；报修人+创建时间+ID；区域+状态索引 |
| m3_commands | 操作者、幂等键、规范化请求 SHA256、首次响应 | (actor_id, idem_key) 唯一；键区分大小写，1–128 位安全 ASCII |
| m3_events | 操作者、动作、前后状态、业务版本、时间与 correlationId | event_id 主键；工单+版本+动作唯一 |
| m3_outbox | 收件人、最小通知载荷、PENDING/尝试次数 | event_id 主键；状态+时间索引；本轮没有投递 worker |

READ COMMITTED 创建事务先锁定命令，再写工单、事件、待投递意图及响应后提交。重复请求等待同一命令锁，不用“先查再插”避免并发冲突。目录验证使用公开端口；当前 fixture 不支持运行中变更，正式主数据变更并发语义仍待 M2 契约接入验证。本人列表的 count 和分页使用同一只读一致快照，按 created_at DESC、id DESC 排序。数据库时间以 UTC 保存；API 带时区，页面按上海时区显示。

## M1/M2 依赖与适配器

| 依赖 | 当前接法 | 尚未证明的部分 |
| --- | --- | --- |
| M2 身份/会话 | 调用现有 identity.Store.AuthenticateRequest，使用当前用户和角色；沿用全局 CSRF | 正式 workorder 权限目录及学生角色映射待 M2 提供。开发开关仅给当前 student 角色补 workorder.report，不写 D 角色种子，不接收客户端角色 |
| M2 空间目录 | M3 Catalog 端口 + backend/dev-maintenance.json，显式 ACTIVE/DISABLED/RETIRED | 正式主数据接口、版本/变更并发以及院系/区域联合验收；不把 M1 Active=false 猜成停用或注销 |
| 审计/通知公共库 | Events.Append 接收同一个 *sql.Tx；开发实现仅写 M3 的 events/outbox | 公共 D 库、审计查询、站内去重、真实渠道、重试及投递均未完成，不计 D-03 验收 |
| M1 资源 HTTP | 本轮不发生维修操作；不修改 M1/M2 表或现有资源契约 | 后续按 BLOCK_ROOM_MAINTENANCE、RELEASE_SOURCE 及回执查询实现；来源隔离、多工单、响应丢失、重启补扫、INT-02/03/04 均 not_run |

服务端默认不注册 M3。当前本机明确开启 M3_ENABLED、M3_DEV_MODE、M3_DEV_STUDENT_REPORTING，并指定 M3_DEV_FIXTURE；同时沿用应用已有 M1_DEV_MODE。若缺少显式开发模式则拒绝启用，避免将适配器误作正式依赖。凭证只在 `.git/local-env` 私有配置，未写入源码或本文件。

新增 M3 OpenAPI 位于 `contracts/m3.openapi.json`，标记 pending-team-review。端点为 `/api/v1/m3/session`、`/rooms`、POST/GET `/workorders`、GET `/workorders/{id}`。公共壳只改注册入口：后端 main.go 加注册调用、前端 modules.ts 加 M3 模块。M1/M2/identity 内部实现、旧 SQL 迁移和已有契约未修改。

## 本机验证结果

- 后端：为 M1/M2/identity/M3 分别传入隔离测试 DSN，执行 `go test -mod=readonly -race -count=1 -json -cover ./...`；39 个顶层测试、18 个子测试通过，失败 0、跳过 0。其中 M3 应用层 9 个顶层 + 6 子测试，领域层 12 个顶层 + 12 子测试。`go vet -mod=readonly ./...` 及服务端构建通过。
- 前端：`bash scripts/check-m3-preview.sh` 执行 17 个测试（M3 逻辑 11、HTTP/会话存储 4、原会话 2）并通过 TypeScript/Vite 构建；Vite 的产物体积提示仍存在，未作性能优化。独立 preview 保留，正式构建不依赖其示例工单。
- 破坏验证：只在 `.git/round1-api/2026-10-22/mutation-backend` 的副本中移除详情归属过滤，测试因他人详情返回 200 失败；另将每次请求的幂等键重生成，测试因重复并发建单失败。恢复副本后 M3 race 测试通过；实际工作区源码未被破坏。
- 浏览器：实际登录虚构本地学生，从 `/m3/report` 提交照明故障，跳转真实详情，刷新仍存在；返回本人列表，筛选电源显示 0 条，清除后恢复 1 条。不是人工验收，也不是旧示例 preview 的记录。
- 真实记录 `WO-184e51bd6b13d884368cac59ce21317c`：本机开发库查询确认工单、命令、事件、PENDING outbox 各 1 条，006 迁移记录 1 条。仅保存待投递意图，不表示邮件已发送。

原始日志与截图在本地 `.git/round1-api/2026-10-22/`：`go-all.jsonl`、`go-result.json`、`go-vet.log`、`go-build.log`、`frontend-check.log`、`frontend-result.json`、`mutation-*.log`、`mutation-result.json`、`live-api-smoke.json`、`live-sql-evidence.json`、`m3-live-detail.png`。这些本地证据不会随普通 Git 提交上传；团队交付前应人工选择脱敏证据。测试库与开发库分离；撤权/禁用测试中的 D 表操作只用于隔离的 campus_m3_check 构造故障，不是生产或开发业务代码。

## 运行与下一轮

本机已配好 MySQL :13306、API :8082、Vite :5176。工作台入口 `http://localhost:5176/m3/workorders`，新增报修入口 `/m3/report`。本机进程重启可在项目根目录分别执行 `bash .git/local-env/dev.sh api` 和 `bash .git/local-env/dev.sh web`；配置在该目录 env.sh，勿粘贴其中密钥到聊天或提交。API 通过既有 migrations.Apply 执行版本化迁移，不手动建业务表。

前端检查使用上述脚本。后端 M3 集成测试需先为一次性隔离库 campus_m3_check 设置 M3_TEST_DSN，再在 backend 执行 `go test -mod=readonly -race -count=1 ./internal/maintenance/...`；未设置时会跳过 SQL 测试，不能把这种结果描述为集成通过。不要将测试 DSN 指向开发业务库。

下一轮先补调度员区域列表、派单/重派和维修人接单：服务端动作/范围检查、expectedVersion、同事务历史与事件、HTTP/页面以及并发验收。之后再接 M1 真实资源 HTTP 做维修下架和验收恢复。仍需确认正式目录/权限/事务事件接法、迁移编号、已有预约影响及参数；不把这些待确认项当成阻止本地单模块开发的额外流程，也不将开发适配器计为跨组集成完成。

## 文档和 Git 保护

原有 m3-plan.md、baseline.json 在基线移动前已逐字备份并校验，保存在本地 `.git/round1-api/2026-10-22/preserved/`；旧路径改名前的干净源文件另存于 old-clean-source。保留前端预览、对齐和旧计划作为历史记录，仅新增当前说明、M3 入口提示及索引证据。提交前在保留原文备份后，将非 M3 索引中的旧目录引用按 PR #7 对齐；核对这些条目与 main@aff197a 完全一致。本次远程变更不回退 M1/M2/D 的需求或路径。

上述验证阶段未 commit、push、merge 或修改远程。用户随后明确授权本次提交并选择团队仓库 Princess-Cpp/campus-resource-platform 的 feat/m3-reporting，后续 M3 统一使用此分支；本次仅交付到该开发分支，不创建 PR 或合并 main，不将授权描述为全组业务评审。原 origin 默认 push 保护保留，上传使用已授权的 GitHub 连接。
