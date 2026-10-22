# M3 第二轮：区域派单、接单与早期取消

日期：2026-10-22；分支 `feat/m3-reporting`；实施基线 `a994d3f61af2ac6d1b362c666e89cb63ef8dc44b`。依据任务书、总约定 v0.2、需求基线 v0.4、范围分级及 [本轮计划](../superpowers/plans/2026-10-22-m3-dispatch.md)。用户授权沿用既有约定直接推进；本文不代表组内评审、用户人工验收、提交、部署或完整 M3 交付。

## 已实现的纵向链路

第一轮的文本报修与本人查询继续保留；本轮已打通区域调度台、当前维修待办、真实可派人员查询、派单/重派、维修人确认接单、本人工单早期取消。所有页面通过 Gin HTTP 操作 MySQL，使用已存在的领域规则与统一 Cookie/CSRF。接单后仍是 ASSIGNED，只增加 acceptedAt；不显示已经维修或资源已下架。

| 任务/需求 | 已实现与验收 | 未完成部分 |
| --- | --- | --- |
| M3-01.2 派单/接单/重派 | 调度员只能处理授权区域；目标须当前启用且有该区域维修权限；处理建议必填；expectedVersion 竞争只成功一方；接受分派记录时间；重派清除接单状态、旧维修人失去处理权、首派时间与阶段期限保留 | 正式公共角色/人员目录的联合验收；维修中重派虽沿用现有领域约束，本轮没有开始维修入口，不计维修阻塞联合验收 |
| M3-01.5 早期取消 | 本人 REPORTED/ASSIGNED 可取消；终态不能重新派单；取消与派单并发只有一个版本胜出；同键重放无重复事件 | 开始维修/未知资源操作期间的完整取消竞争尚未进行，异常关闭仍 postponed |
| M3-01.7 查询 | 本人列表保持原范围；新增区域调度列表和当前维修待办；详情由领域 CanRead 控制；按状态、类别、日期筛选和稳定分页；详情与时间线在同一只读快照中查询 | 验收/主管待办、耗时与评分统计尚未实现 |
| M3-02.2 事务事实/通知意图 | 动作、版本、历史、幂等响应、审计事实及按参与人的待投递意图同事务；旧维修人仅写交接通知意图 | 仍为开发 Events 适配器，没有公共审计查询、站内投递、外部邮件或重试 worker，不计 D-03 完成 |

## 状态、权限与成功界点

| 动作或视图 | 当前服务端授权 | 成功结果 |
| --- | --- | --- |
| view=mine | 当前有效会话，reporter_id=本人 | 本人历史；与第一轮保持兼容 |
| view=dispatch | workorder.dispatch，按当前 Areas 过滤 | 授权区域工单；区域使用二进制精确比较，与领域规则一致，不因 MySQL 默认大小写折叠泄露相似区域 |
| view=assigned | workorder.repair，assignee_id=本人 | 当前 ASSIGNED/IN_PROGRESS 待办；取消工单不再显示 |
| ASSIGN | 当前调度权限+工单区域，可信目录实时校验目标 | REPORTED→ASSIGNED；ASSIGNED 重派仍为 ASSIGNED，保存独立分派历史 |
| ACCEPT_ASSIGNMENT | 当前维修权限+当前被派人 | 保持 ASSIGNED、记录 acceptedAt；未调用任何 M1 资源操作 |
| CANCEL | 当前有效会话+本人，REPORTED/ASSIGNED 且无待确认资源操作 | CANCELLED；不发送资源释放 |
| 详情 | 本人、当前有维修权限的被派人或获授权区域的工作人员 | 越权返回 404；历史分派本身不授予访问权 |

写入带 Idempotency-Key，命令键在同一用户所有 M3 创建/动作中共用命名空间。动作哈希含工单 ID、动作、原始 expectedVersion、目标和规范化说明；同键换工单或改内容返回 409。创建之外的动作先锁命令再锁工单；首次执行检查版本并复用 Order.Apply；历史重放也检查当前权限和归属，不能因有旧响应绕过撤权。同键重放返回原动作结果，页面随后重新查询当前详情。

前端将未知结果的原动作/目标/版本/键按当前用户和工单保存在 sessionStorage；双击只发一次，刷新保留未决请求，核对期间不允许换目标或换动作。首次明确的 4xx 拒绝允许刷新重选；曾经丢包后再遇到拒绝仍保留核对记录。存储损坏/不可用及成功响应不完整时不盲目创建新命令。浏览器按钮可见性仅辅助操作，所有权限和状态由服务端检查。

## 数据与接口变化

新增 `007_m3_dispatch.sql`（本地候选版本，团队同步前仍需检查编号和 review），不改 001–006：

- m3_workorders 新增 assignee_id、姓名快照和维修待办索引；acceptedAt、首派时间、阶段与参数仍随领域 JSON 快照持久化。
- m3_assignment_history 记录每次分派/重派的业务版本、操作者、原/新维修人、姓名快照、处理建议、时间；工单+业务版本唯一。
- m3_events 新增操作说明；接单与取消也追加事实和可追溯操作者。
- m3_outbox 主键改为 event_id+recipient_id+kind，同一次业务事件可以保存多个不同参与人的通知意图。上报人、新维修人和原维修人按动作分别写意图，原维修人仅交接通知；载荷不含联系方式或故障全文。状态仍为 PENDING，不能描述为已送达。

M3 OpenAPI 在 `contracts/m3.openapi.json` 增量更新，仍 pending-team-review：

- POST /api/v1/m3/workorders/{id}/actions，仅允许 ASSIGN/ACCEPT_ASSIGNMENT/CANCEL；拒绝客户端 userId、权限及未来 START/FINISH 等动作。
- GET /api/v1/m3/workorders/{id}/assignees，返回授权区域内启用维修人员的 ID/姓名；派单时再次核对，不能相信下拉框缓存。
- GET /api/v1/m3/workorders 支持 view=mine/dispatch/assigned；原筛选/分页保留。
- 详情新增服务端 allowedActions、当前维修人、接单/首派/阶段截止时间及相应时间线字段。

只修改 M3 包、M3 应用装配、M3 页面和检查脚本；未修改 M1/M2/identity 内部实现、公共需求/业务决策或其他模块业务数据。测试里的 D 表故障/角色设置仅用于独立 campus_m3_check，不是运行时代码。

## 公共依赖与本机适配

身份始终经 identity.Store.AuthenticateRequest 读取真实会话；人员查询只调用公开 Principal/Principals 能力，不读写 D 内部表。正式 workorder 权限已存在时直接使用；本机缺少正式映射，因此延续显式开发配置：StudentReporting 标记报修权限，新增 M3_DEV_STAFF_FIXTURE 指向私有配置，绑定已通过公开注册创建的虚构用户 ID 及区域/派单或维修权限。浏览器和注册表单不能自行提供这些绑定；当前基础角色被撤销、账号禁用后绑定不再生效；没有新增 D 权限或修改 D 角色种子。

空间仍使用第一轮的开发 Catalog；事务事件仍是 M3 本地开发适配器；没有为公共分工新增远程服务。身份目录的正式并发变更语义、共同 schema/权限 review、公共事务库及真实投递仍待共同接入，开发模式不是组内决定。现有 M1 资源 HTTP 本轮未调用，INT-02/03/04 保持 not_run。

本机 MySQL :13306、API :8082、Vite :5176。入口：`http://localhost:5176/m3/dispatch`、`/m3/assigned`、`/m3/workorders`；无对应权限时 API 拒绝读取。虚构调度/两名维修人的登录信息与绑定只保存在 .git/local-env 的 0600 私有文件，不提交。重启仍使用 .git/local-env/dev.sh api/web，保留原有学生与业务数据。

## 真实验证

后端最终全包执行 `go test -mod=readonly -race -count=1 -json -cover ./...`，为 M1/M2/identity/M3 分别设置隔离 DSN：51 个顶层测试、18 个子测试通过，0 失败、0 跳过。M3 应用层 21 个顶层，领域层 12 个顶层+12 子测试。go vet 和后端构建通过。未配置 DSN 的跳过不能计为集成通过。

新增 12 个真实 SQL/HTTP 测试覆盖：区域列表/候选/派单/接单；禁用/撤权/异区域目标；相同版本竞争；10 路相同动作并发与跨工单键冲突；重派后旧人失权及期限保留；本人取消与伪造字段；outbox 故障整笔回滚；实际 HTTP 回包丢失后服务重建核对；候选加载后失效及历史重放；取消与派单竞争；动作登录/CSRF/字段校验；开发映射不写公共角色且撤权生效。

前端 `bash scripts/check-m3-preview.sh`：22 个测试通过（原 17 个+新增动作 5 个），TypeScript/Vite 构建通过。新增测试覆盖原键/目标/版本传输、未知结果恢复、双击、版本拒绝、存储损坏和不完整成功响应。Vite 仍提示主产物超过 500 kB，未作性能优化。

破坏验证在独立源码副本执行：移除区域过滤后，B楼调度员读到 A楼工单，测试失败；绕过客户端版本核对并将领域 expectedVersion 替换为当前版本后，并发派单出现两个 200，测试失败。恢复副本后全 M3 race 测试通过；实际工作区源码哈希保持不变。

浏览器以虚构学生提交记录，再登录虚构调度员派单、登录当前维修人查看待办并确认接单，刷新仍显示 ASSIGNED/已接单。实际记录 `WO-96f659eff80726515047e1f941e793f0`：MySQL 中 version=3、命令 3 条、分派历史 1 条、事件 3 条、PENDING 通知意图 5 条，有真实 acceptedAt。没有资源下架，也没有通知投递。

原始本地证据在 `.git/round2/2026-10-22/`：go-all.jsonl、go-result.json、go-vet.log、go-build.log、m3-expanded.log、frontend-check.log、dispatch-red.log、mutation-*.log、mutation-result.json、live-sql-evidence.json、m3-accepted.png。日志和截图默认不上传；虚构用户数据也不作为正式人工验收。

## 交付状态与后续

本轮实施结束时没有 commit/push/merge/PR，起点为上轮 a994d3f。用户随后授权“仔细检查，如果没问题可以上传”；本次按 [提交前复查](../local-review/2026-10-22-m3-round2-prepublish.md) 修正取消状态文案并复跑检查后，仅交付到开发分支，不合并 main，也不将上传授权写成全组业务评审或正式人工验收。继续统一使用 feat/m3-reporting，不新建其他 M3 远程分支。旧计划、第一轮实施记录、个人本地草案与任务书均保留；需求索引只增量更新 M3 证据，不改变其他需求或评审状态。

下一轮按原顺序接 M1 资源真实 HTTP：开始维修的准备/回执核对、全室阻塞、响应丢失/重启恢复，再做完工、独立验收及仅释放本工单来源。当前不能用接单成功替代维修开始或跨模块验收。

## 其他开发者的本地复现

除原有 M3_ENABLED/M3_DEV_MODE/M3_DEV_STUDENT_REPORTING/M3_DEV_FIXTURE 外，M3_DEV_STAFF_FIXTURE 可指定本机私有 JSON 文件。通过已有注册/登录接口取得自己的虚构用户 ID，配置以下结构；不需要改 D 表，空配置不授予物业开发权限。真实角色交付后应替换适配器并联合验收，不沿用此配置作为正式授权。

```json
[{"userId":"your-local-fictional-user-id","permissions":["workorder.dispatch"],"areas":["A楼"]}]
```

维修人条目使用 workorder.repair；文件不能接受注册表单传入，也不放在公开静态目录。配置只对明确绑定且当前仍有基础 student 角色的启用用户生效，修改配置后重启本地 API。
