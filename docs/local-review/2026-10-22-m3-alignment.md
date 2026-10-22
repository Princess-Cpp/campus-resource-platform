# M3 本地基线与依赖对齐

日期：2026-10-22，Asia/Shanghai。总约定 v0.2、需求基线 v0.4。结论：代码、依赖、启动环境已对齐；跨组 schema/接口评审仍待真实确认，没有宣称已完成团队契约对齐。

## 1. Git 与本轮边界

- 重新 fetch 后 origin/main = 3b7d8975ace942212b835724733de1651b1db89e。
- 当前本地分支 codex/m3-alignment 直接从该提交创建，不跟踪远程；不是 merge，不是 worktree。
- 原 docs/review-1-adjustments@137537e、原本地 main 和用户 PDF/本地规划均保留。
- main 已含 M3 PR #6（领域规则）和登录 PR #5。本地只改 M3 说明/证据、规划及 .git 内私人启动环境；未修改应用代码、正式 schema 或 M1/M2 内部数据。
- 本轮未提交、推送、合并、创建远程分支/PR 或部署；push URL 仍禁用。

## 2. 技术对齐

| 项目 | 当前事实 | 后续采用方式 |
| --- | --- | --- |
| M3 规则 | domain/workorder.go 已实现 New/Apply/ConfirmResource/ResourceUnknown/Escalate | 应用层复用，不重写另一套规则 |
| 前端 | React/TypeScript、shadcn、模块路由 | 接入 src/modules/m3，装配 app/modules.ts，不新建旧 main.jsx |
| 身份 | identity.Store、Principal、Cookie/CSRF；每次认证检查禁用和当前授权 | 只从认证后的 Principal 映射 domain.Actor；请求 body 不能设置权限/Enabled |
| 权限缺口 | 005_identity 中无 workorder.*，学生也没有 workorder.report | M2 评审角色/权限迁移和种子；不能把 student 自动当报修权限 |
| 空间缺口 | M2 Room 只有 Active，无座位和 DISABLED/RETIRED 区分；正式目录未交付 | 由 M2 维护可信目录契约；开发适配器不能通过可预约列表代替完整报修目录 |
| 迁移 | 已有 001–005；runner 会发现所有 SQL，使用校验和与迁移锁 | 不改旧迁移，申请下一可用编号；006 尚未预留 |
| 事务事件 | M1/M2 有各自业务事件，公共审计/outbox/任务端口未交付 | 先评审同事务端口，不直接访问 m1/m2 事件表，不另造公共表 |
| 资源接口 | M1 已有 operations/receipt/sources，尚无正式资源 OpenAPI | 第一期报修/查询不调用资源；维修阶段需 M1 确认正式契约 |
| 维修差异 | M1 在阻塞事务内取消本人模块预约；总约定描述影响事件，M2 影响链路缺失 | INT-04 由三组确认；M3 不重复代取消 |
| 停用空间 | M3 领域允许 DISABLED 上报，M1 阻塞拒绝非 Active | 报修与开始维修分别判断，组合行为待共同评审 |

## 3. 本轮真实验证

后端在当前 checkout、独立 MySQL 13316 测试实例运行，使用各测试要求的 campus_m1_check、campus_m2_check、campus_identity_check；凭证未写入文档。测试实例运行后停止，原 13306 开发实例保持。

| 检查 | 实际结果 | 边界 |
| --- | --- | --- |
| go test -mod=readonly -json -race -count=1 -cover ./... | 30 顶层测试及 12 子测试通过，失败0，跳过0 | M1 3、M2 10、identity 5、M3 domain 12 顶层；不重复把子测试算顶层 |
| M3 domain 覆盖率 | 90.9% | 仅领域代码；不是 M3 全模块覆盖率 |
| go vet -mod=readonly ./... | 通过 | 静态检查 |
| go build -mod=readonly ./cmd/server | 通过 | 开发后端构建，嵌入仍为源码占位文件，不是生产前端完整打包 |
| npm ci --ignore-scripts --no-audit --no-fund | 从现有锁文件安装395依赖，成功 | 未升级版本，未运行安全审计 |
| npm test | 2 个会话测试通过，无失败/跳过 | HTTP 模拟测试，不是浏览器操作 |
| npm run build | TypeScript/Vite 通过 | 主 JS 约1.03MB，大 chunk 提示保留 |
| 本地 HTTP | 8082健康200；5176登录路由200；代理健康200；直连及代理未登录会话401 | 只证明启动/代理及未登录边界，不算 UI 人工验收或 M3 业务 |

证据在 .git/alignment/2026-10-22/{go-test.jsonl,verification.json,go-vet.log,go-build.log,http-smoke.json}。没有重新执行历史破坏验证，也未执行 M3 SQL、Gin/API、资源联调、通知或部署测试。

## 4. 本地环境修正

旧 .git/local-env/dev.sh 只在二进制不存在时构建，会复用旧程序；web 使用旧私人 Vite 配置，缺少当前 React/Tailwind/alias。现已修正：api 每次先构建当前代码；web 使用仓库 frontend/vite.config.ts。

为避免覆盖旧开发数据，新建独立数据库 campus_m3_alignment_dev，仅执行已有版本化迁移与显式开发种子；数据库创建/账户授权是环境初始化，未手工修改业务表。配置仍位于 .git/local-env/env.sh（0600），不输出或提交凭证。

启动：

```sh
bash .git/local-env/dev.sh api
bash .git/local-env/dev.sh web
```

前端 http://127.0.0.1:5176；后端 http://127.0.0.1:8082；Vite 代理 /api 和 /health。当前运行 M1/M2/登录基线，M3 页面/API 尚不存在。密码只存私人环境配置。旧 .git/local-env/vite.config.mjs 不再用于启动，历史配置保留。

`test-domain` 执行领域测试。`test` 的全包测试仍须显式设置各自独立测试 DSN；若缺失导致跳过，不得记为 SQL 集成通过。本轮测试通过由上述独立运行提供证据。

原私人脚本备份在 .git/alignment/2026-10-22/*.before-alignment，不包含在 Git 交付中。

## 5. 进入持久化/API 前的评审清单

下列是待确认清单，不是已确认分工/决策。具体方案见 docs/superpowers/specs/2026-10-22-m3-reporting-query-design.md。

| 编号 | 需要确认的具体内容 | 参与者 | 当前状态 |
| --- | --- | --- | --- |
| ALIGN-01 | 第一轮 M3 表与唯一键、迁移编号，M3 schema 不改其他模块表 | M3、迁移维护人及使用组 | 待评审 |
| ALIGN-02 | workorder.report/dispatch/repair/accept/supervise；角色/区域/撤权；谁可报修停用空间 | M2 + M3 | 待评审 |
| ALIGN-03 | 可信房间与座位目录，ACTIVE/DISABLED/RETIRED，当前访问校验；开发适配器替换 | M2 + M3 | 待评审 |
| ALIGN-04 | 在同一个 sql.Tx 中写统一审计/outbox 的接口；第一轮未投递事件如何保留 | M2 + M3 | 待评审 |
| ALIGN-05 | 停用房间维修、M1 直接取消与影响事件差异、M2 既有使用处理 | M1 + M2 + M3 | 待评审，第二轮门槛 |

推进判断：当前可以继续审查 schema/API 草案；未发现足以宣称上述事项已通过的评审记录。既有 m3-plan.md 明确“未评审前只完成独立规则及可审查方案”。因此本轮不擅自执行 M3 DDL、添加公共权限或注册看似可用但依赖未确认的 API。
