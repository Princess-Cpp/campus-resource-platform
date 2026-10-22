# M3 文本报修与本人查询设计提案

日期：2026-10-22，Asia/Shanghai。状态：本地待评审；不是组内决定、正式契约或已实现功能。基线：main@3b7d897，总约定v0.2、需求v0.4。修订前必须重查远程迁移编号与提供方契约。

## 1. 第一项开发的目标与边界

用户已认可先对齐，再考虑推进文本报修/本人查询。首个实现任务仅覆盖 M3-01.1、M3-01.7 的本人查询部分及 M3-02.2 的事务事件接入；不把整条需求标为完成。

验收用户：具备 workorder.report 的授权用户。完成效果：登录 → 选择有权访问的房间/可选座位 → 填类别与文本 → 提交 → 本人列表/详情可查询，刷新或服务重启后不丢。非授权用户拒绝；同键重试不重复建单。

不新增独立登录；不调用 M1 阻塞；不做图片、派单、统计或取消入口；不读取其他模块内部表。后续派单在此基础另立任务。

## 2. 方案选择

推荐：M3 自有 Go 包与 m3_* SQL 表，复用 domain.New/CanRead；通过入口适配公共身份/目录，使用共同确认的事务事件端口。优点是沿用单应用与现有规则，事务/授权清晰。

备选：显式本地目录适配器可支持虚构房间开发，但须经使用组确认接口、默认关闭且正式环境拒绝启用，并单列替换验收。不能把 fixture 算正式目录；不借 M1 Search 代替目录，因为已占用/停用房间也可能需要报修。

不采用：M3 自建身份数据库、跨读写 M1/M2 内部表，或内存工单代替 SQL。公共接口未定时先提供本设计供评审，不把候选端口写进对方实现。

## 3. 可信依赖端口提案

下面 Go 签名是 M3 调用侧提案，不表示 M2 已承诺实现。最终命名和兼容方式由双方评审；不为这些端口创建新远程服务。

```go
type Identity interface {
    AuthenticateRequest(context.Context, *http.Request) (domain.Actor, error)
}
type Directory interface {
    ListRooms(context.Context, domain.Actor) ([]domain.Room, error)
    Room(context.Context, domain.Actor, string) (domain.Room, error)
}
type EventContext struct {
    EventID string
    CorrelationID string
}
type TransactionalEvents interface {
    Append(context.Context, *sql.Tx, domain.Change, EventContext) error
}
```

Identity 由公共会话 Store 认证后映射 ID、Department、Permissions、Areas，认证成功才给 Enabled=true。写请求沿用 identity.HTTP.CSRFMiddleware，不接受服务 Bearer 冒充用户。当前学生权限不包含 workorder.report，不按角色名自动补权限。

Directory 返回可信完整房间状态及座位归属，并按当前访问范围过滤；不把 Active=false 猜成 RETIRED，也不从客户端接受名称、区域或部门快照。目录无权限或目标不存在统一 NOT_FOUND，依赖暂不可用返回 DEPENDENCY_UNAVAILABLE。初始上报允许 ACTIVE/DISABLED，拒绝 RETIRED，具体授权由 M2/M3 确认。

TransactionalEvents 同一 sql.Tx 写审计和通知 outbox；任何本地事件写失败回滚工单与命令，网络投递失败在提交后重试。m3_events 保存本模块时间线事实，不代替 D 公共库；公共审计/outbox 表归属和 schema 不在本任务擅自创建。接口未获评审不得用 no-op 假装满足验收。

## 4. 第一轮数据提案

迁移候选编号006，仅供协调，尚未预留；最终迁移进入 backend/migrations 前重查所有分支。所有表 InnoDB、utf8mb4；ID 为随机 UUID 字符串，UTC DATETIME(6)，业务时间由后端产生。模块外用户/空间只保存稳定ID，不建立跨模块写外键。

| 表 | 列与约束 | 用途 |
| --- | --- | --- |
| m3_workorders | id VARCHAR(64) PK；reporter_id、room_id、room_name、area 各有明确长度；seat_id可空；category VARCHAR(32)、description TEXT、severity VARCHAR(16)；state VARCHAR(32)、sync_state VARCHAR(32)、version BIGINT；policy JSON、stage JSON；created_at/updated_at DATETIME(6)；INDEX(reporter_id,created_at,id)、INDEX(area,state,created_at,id) | 保留 Order 第一阶段字段与政策/阶段快照；后续分派/资源字段另做版本化迁移，不用 JSON 混装身份权限 |
| m3_commands | actor_id VARCHAR(64)、idem_key VARCHAR(128) 二进制精确比较组成PK；request_hash CHAR(64)、workorder_id VARCHAR(64)、response JSON、created_at DATETIME(6) | 同一操作者同键跨写动作不复用；hash含规范方法/路径/动作/请求；不能只按工单查重 |
| m3_events | event_id VARCHAR(64) PK；workorder_id VARCHAR(64)、version BIGINT、action VARCHAR(32)、actor_id VARCHAR(64)、old_state/new_state VARCHAR(32)、occurred_at DATETIME(6)、correlation_id VARCHAR(128)；UNIQUE(workorder_id,version,action) | 工单事实时间线，无凭证/密码/完整联系人数据；敏感内容不复制到宽权限日志 |

长度对齐：当前用户/资源稳定ID沿用现有VARCHAR(64)；房间名128、区域128；描述按domain规则10–1000 Unicode字符，severity NORMAL/URGENT，类别LIGHTING/POWER/FURNITURE/DOOR_WINDOW/OTHER。这些源自现有规则，具体业务值仍待评审，使用 ProposedPolicy 的 proposal-v1 快照。

状态初值 REPORTED、IDLE、version=1。不新建另一套同步状态名。首轮 policy/stage 保存字段级JSON编码，后端显式转换 duration 与UTC时间，读回后必须与原规则对象等价；不用客户端JSON直接反序列化可信Order。

命令与事件保存策略先不实现自动删除，保留期限另按DEC-12确认；幂等回放前重新检查当前登录及读取权限，不因已有缓存结果绕过撤权。

## 5. 用户接口提案 v1

这些端点仅为评审草案；实施时新增 contracts/m3.openapi.json，并同步客户端/测试。复用已有 data/error/correlationId 外层，不改公共响应。

### GET /api/v1/m3/rooms

为报修表单提供有权访问的房间及可选座位，消耗 Directory.ListRooms；只返回稳定ID、名称、区域、允许展示的状态和座位标识，不泄露其他院系目录或私人信息。与可预约查询分开，已占用房间不能因此消失；停用房间按确认后的政策展示，注销房间不得作为新报修目标。选中后提交时仍调用 Room 重新验证，不能信任缓存列表。正式目录未就绪时此入口只能接经评审的显式开发适配器，不访问对方内部表。

### POST /api/v1/m3/workorders

请求头：Cookie 会话、X-CSRF-Token、Idempotency-Key（非空，最多128字节）、可选X-Correlation-Id。请求：

```json
{
  "roomId": "room-1",
  "seatId": "seat-1",
  "category": "LIGHTING",
  "description": "教室前排照明灯持续闪烁无法正常使用",
  "severity": "NORMAL"
}
```

seatId可省略；severity省略采用现有NORMAL；不能提交reporterId、role、area或可信房间状态。描述去首尾空白后交给domain.New校验。紧急标记只存上报事实，不自动授予优先权限。

成功201，data为可读工单DTO：id、reporterId（本人）、roomId/roomName/area、seatId、category、description、severity、state、syncState、version、createdAt/updatedAt；不暴露policy内部细节、命令哈希和未授权联系方式。同键相同规范请求回放同一工单，不另生成事件；同键异内容409 IDEMPOTENCY_CONFLICT。

### GET /api/v1/m3/workorders

首轮仅view=mine（默认）；其他视图明确返回INVALID_INPUT，不通过忽略参数扩大范围。page默认1、pageSize默认20最大100，按created_at DESC,id DESC稳定排序；支持state、category、from、to，时间为带时区RFC3339，[from,to)。state/category使用已知枚举，非法值400。

响应200，data={items,page,pageSize,total}，仅当前本人；SQL先按reporter_id过滤再分页，不能查全表后前端过滤。不能接受query.userId替换登录用户。列表和详情都重新检查身份，不把前端路由守卫当权限。

### GET /api/v1/m3/workorders/{id}

首轮仅本人详情和其可见时间线。存在但非本人返回404，与不存在一致；同一用户被禁用后401。角色区域视图在派单任务实现前不提前开放。

错误：400 INVALID_INPUT；401 UNAUTHENTICATED；403 FORBIDDEN或CSRF_INVALID；404 NOT_FOUND；409 IDEMPOTENCY_CONFLICT；503 DEPENDENCY_UNAVAILABLE。服务端错误不回显SQL/DSN/凭证。资源相关202留到第二轮，首轮不返回虚假维修处理中。

## 6. 事务和并发步骤

1. 公共会话验证当前身份与CSRF，检查workorder.report；可信目录验证房间及可选座位，不在HTTP上持业务锁。
2. 规范化请求，算SHA-256；以操作者+键保存/读取命令，唯一键竞争由数据库解决。相同请求回放前复查当前读取/访问权；异内容拒绝。
3. 使用服务端now、可信目录与ProposedPolicy调用domain.New；生成的ID在同一个已持有命令归属的事务中持久化。
4. 同事务写工单、REPORT时间线、公共审计和outbox、可回放的命令响应；全部成功才提交。
5. 数据库提交后返回201。响应丢失时客户端沿用原key查回结果，不新建工单。后续独立投递outbox，不重跑REPORT。

并发第二请求等待唯一键第一事务结果：首请求提交后读同一response；首请求回滚则另一请求可取得键并执行。验证SQL唯一约束和事务实际行为，不只单元模拟。

## 7. 验收案例与范围

| 案例ID / 需求 | 输入或故障 | 必须观察到的结果 |
| --- | --- | --- |
| RQ-01 / M3-01.1 | 授权用户合法文本上报 | 201、REPORTED、本人信息来自会话；工单/命令/事件同事务入库 |
| RQ-02 / M3-01.1 | 缺会话、缺report权限、缺CSRF | 分别401/403；无新工单、命令、假通知 |
| RQ-03 / M3-01.1 | 座位不属房间、描述非法、伪造可信字段 | 拒绝；不能以body身份越权 |
| RQ-04 / M3-01.1 | 同键同请求顺序与并发重试 | 只一张工单/一次REPORT/一份业务通知意图；返回同ID |
| RQ-05 / M3-01.1 | 同键不同描述 | 409；原工单不变 |
| RQ-06 / M3-01.1 | 不同键分别真实上报相同故障 | 两张工单，不自动吞并 |
| RQ-07 / M3-01.7 | 两用户列表/详情、query伪造userId | 不泄露他人工单，详情越权404 |
| RQ-08 / M3-01.7 | 同时刻多条、翻页、筛选边界 | 稳定排序、范围正确，参数越界400 |
| RQ-09 / M3-01.1/01.7 | 上报响应丢失后重试、重建应用后读取 | 原ID、SQL记录仍存在，不依赖内存 |
| RQ-10 / M3-02.2 | 同事务审计/outbox写失败 | 工单/命令/时间线全部回滚，无假成功 |
| RQ-11 / M3-01.1/01.7 | 禁用/撤权后重试旧幂等键 | 按当前权限拒绝，不回放敏感缓存 |
| RQ-12 / M3-01.1 | 停用/注销空间 | 按共同确认目录语义处理；不得将 Active=false 猜成可上报注销空间 |
| RQ-13 / M3-01.1 | 已占用房间和其他院系房间出现在目录 | 已占用但可访问的房间可选择；无权访问的房间不泄露；提交重新验证当前状态 |

实现顺序：先真实SQL/API失败测试→最小事务实现→前端表单/本人列表详情→逐角色人工验收→破坏授权/幂等检查确认测试失败并恢复。DSN仅私密环境注入；M3测试库独立命名并在测试开头校验数据库名，不能对团队库执行清理。

## 8. 等待的具体评审与下一步

请M2/M3确认调用侧身份/目录/事务事件端口及权限目录；请迁移维护人/使用组确认三张自有表、事件归属和下一编号。M1维修协议不阻塞本轮纯文本报修设计，但必须在开始维修任务前另行确认。

只有真实确认后才写“已评审”；本轮不代写同意结论、不发消息给组员、不把此草案放入自动执行迁移目录。确认后将本稿细化为函数/文件/测试任务，再推进SQL/API与页面。
