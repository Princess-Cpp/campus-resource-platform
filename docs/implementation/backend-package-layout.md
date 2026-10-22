# 后端按业务职责组织目录

实施基线：`main@3b7d897`，模块设计与集成总约定 v0.2。此变更依据用户对后端功能目录的调整要求，覆盖 M1/M2/M3 和 D-01 的代码组织；不新增业务需求，不改变各项需求的实现或验收状态。业务维护组仍按分工表执行。

## 目录与范围

| 原位置 | 新位置 | 职责与维护组 |
| --- | --- | --- |
| `backend/internal/m1` | `backend/internal/booking` | 座位预约、签到、任务及现有资源仲裁，M1 |
| `backend/internal/m2` | `backend/internal/classroom` | 整室申请、审批及资源 HTTP 客户端，M2 |
| `backend/internal/m3/domain` | `backend/internal/maintenance/domain` | 报修与维修工单领域规则，M3 |
| `backend/internal/common/identity` | `backend/internal/identity` | 登录、会话与权限，M2 |
| `backend/internal/dev/m2.go` | `backend/internal/dev/classroom.go` | 教室审批开发适配器 |

同时更新 Go 包名、import、入口中的身份适配器、开发 fixture 类型/加载函数、现有测试和需求索引路径。identity 测试移到较浅目录后，fixture 路径改为 `../../dev-fixture.json`。

目录按业务职责命名，组号保留在需求 ID、负责人、审计来源和协议中。前端目录、本次未改动的 HTTP v1 路径、JSON 字段、权限字符串、环境变量、SQL 表名和历史迁移文件保持原值；无需迁移数据库或调整调用方。M2 对资源的调用仍经过 HTTP。

资源仲裁目前与预约事务、维修取消、审计和通知共用 Service；此次保留在 `booking/resources.go`。以后独立为 resource 包时需明确事务和预约影响回调，并单独评审。没有建立尚无实现的 catalog、notification、audit 或 platform 空包。

## 验收方式

对照基线检查所有 Go 文件，允许的差异仅为包名、导入与标识符、fixture 相对路径和格式。运行已有预约、审批、身份及维修规则测试；MySQL 测试使用隔离容器中的三个专用测试库，不能以缺少 DSN 导致的跳过替代通过。审批测试继续使用真实 HTTP 资源调用。

临时破坏审批详情权限过滤，确认越权测试失败，恢复后重跑。另执行 `go vet ./...`、后端构建和 `git diff --check`。后端构建使用已有前端嵌入产物，仅验证 Go 入口装配；不代表本次重新构建前端或完成人工 UI 验收。

## 实际结果

2026-10-22，在 bac 临时验证目录中使用 Go 1.27.0 与隔离 MySQL 8.4.11 容器执行：

- 设置三个专用数据库的 `M1_TEST_DSN`、`M2_TEST_DSN`、`IDENTITY_TEST_DSN` 后，`go test -race -count=1 -json ./...` 通过：30 个顶层测试、12 个子测试，测试失败及跳过均为 0。server、dev、migrations 三个包没有测试文件，不将其计作通过的测试。
- `go vet ./...` 与 `go build ./cmd/server` 通过；构建使用已有前端嵌入产物，未重新构建前端。
- 仅在临时验证副本中将 `classroom.canRead` 改为恒真，`TestM2HTTPWorkflowAndScopes` 按预期失败（预期 404、实际 200）；恢复原文件后，全量 race 测试、vet 与构建再次通过。
- 对照基线的 27 个 Go 文件，除目录/包名、引用标识符、fixture 相对路径及格式外一致；SQL、协议、前端和依赖文件未修改。44 条需求的状态和历史证据保留，仅更新实现/测试路径。
- `git diff --check` 通过。

首次默认工具链下载因网络超时失败，改用备用 Go 下载源后取得仓库要求的 1.27.0；未降低 go.mod 版本。以上是临时环境自动验证，不是项目远程部署。公共层目录和跨模块引用变更仍需 PR 评审，不声称组内或人工验收已完成。
