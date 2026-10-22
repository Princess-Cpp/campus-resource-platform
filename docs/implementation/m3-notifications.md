# M3 第五轮：站内消息与本地邮件投递

日期：2026-10-23（Asia/Shanghai）。基线 `5fd6cc7536430f79b94b52636d5ae90044ba00ea`，现有 `feat/m3-reporting` 工作区；总约定 v0.2、需求 v0.4。需求 ID：M3-02.2、D-03.1/03.2；相关权限修复 M3-01.2/01.7。初次交付为本地开发；用户随后授权检查后上传既有 M3 分支，见[复核记录](../local-review/2026-10-23-m3-round5-prepublish.md)。不合并或部署；组内 schema/公共接口评审、人工业务验收未完成。

## 本轮行为与数据

- 本人站内消息、未读数、未读筛选、稳定分页、单条已读。HTTP Cookie/CSRF 来自已有身份层；不接受 userId 查询他人消息，派单权限也不能读别人的收件箱。重复已读自然幂等，不触发工单动作。
- 只展示固定历史事件摘要，不返回 outbox 原始 payload、报修描述、人员身份、联系方式或评价内容。详情链接按当前权限计算，实际详情 API 再次授权。
- 现有 `m3_outbox` 同事务意图沿用，不迁移或接管 M1/M2 outbox。候选 `010_m3_notifications.sql` 增加 read_at、next_attempt、lease_until、lease_token、delivered_at、last_error 和索引；001～009 不变，历史记录保留，默认未读。该迁移已在隔离数据库和本机开发库执行，仍待组内评审。
- 投递使用短事务领取并提交，事务外核对身份/业务和 SMTP；随机令牌保护结果写回，旧任务不能覆盖新租约。30 秒租约、SMTP 最多 5 秒、每轮最多 50 条；已发起网络发送次数持久化，失败最多 5 次，1/2/4/8 分钟退避。进程重启从持久化租约/期限继续。
- 无目录保留 PENDING/DIRECTORY_UNAVAILABLE；无配置保留 UNCONFIGURED/CHANNEL_UNCONFIGURED，不消耗发送次数。无效测试地址 FAILED/INVALID_RECIPIENT，普通发送失败达到上限为 FAILED。仅记录有限错误码，不存 SMTP 原始响应或凭证。
- 投递前重新查公开身份 Principal 和 M3 当前状态。禁用、维修撤权或失去区域权限后停止发送；重派后旧维修人仅有交接摘要，不再收到原处理通知。主管升级离开原阶段即抑制。SUPPRESSED 只表示停止外部投递，历史站内摘要和阅读记录仍保留。

外部邮件只含“请登录查看”的通用内容。开发 SMTP 仅接受 loopback IP，发件/收件地址限 `.test`；不把未经验证的 ContactRefs 当邮箱。DATA 最终 250 是接收成功界点；SMTP 响应丢失可能重复，重试使用同一 Message-ID，不能宣称外部严格一次。

## 状态、接口与代码边界

邮件状态：PENDING → SENDING → DELIVERED；暂时失败回 PENDING，达到上限 FAILED；缺渠道 UNCONFIGURED；收件人/阶段变化 SUPPRESSED。租约到期的 SENDING 可被新任务领取，最后一次尝试崩溃时保留结果未知并停止越过发送上限。站内 read_at 与邮件状态、工单状态相互独立。

开始维修/验收释放意图在资源确认前明确写为“等待确认”，资源确认后单列确认消息。`TestNotificationResourceIntentDoesNotClaimSuccess` 先复现提前报成功，再经真实 M1 HTTP/MySQL 验证准备阶段与确认完成的差别。

新增草案接口（仍为 pending-team-review）：

|接口|行为|
|---|---|
|GET `/api/v1/m3/notifications?page=1&pageSize=20&unread=false`|当前本人消息、全部本人未读数；未知或重复查询参数拒绝|
|POST `/api/v1/m3/notifications/{eventId}/read`，body `{kind}`|当前本人消息标为已读；他人/不存在为 404，重复成功；Cookie/CSRF 必需|

M3 页面为 `/m3/notifications`。`maintenance.NotificationDirectory`/`NotificationSender` 是调用端端口；`internal/notification` 只有 SMTP 传输，不拥有任务、outbox 或业务仓储。`maintenance/dev/notifications.go` 显式本地路由；现有 M3 开发 ticker 调用投递回调，没有增加第四套调度器或远程通知服务。

开发配置（不含真实凭证）：`M3_DEV_SMTP_ADDR` 为本机接收器的 IP:端口，`M3_DEV_NOTIFICATION_RECIPIENTS` 为私有 JSON 文件路径，内容映射“当前稳定用户 ID → 虚构 .test 地址”。两项都不设置时保留未配置状态；不要把配置或真实联系方式提交。当前交互式开发环境没有配置长期邮件接收器，页面显示 UNCONFIGURED；SMTP 真实投递证据来自下列隔离测试接收器。

## 本轮发现并修复的已有权限问题

维修人员撤销区域后，旧实现仍按 assignee_id 返回维修待办，领域详情与维修动作也缺区域检查。新增 `TestNotificationRevokedAreaAlsoRemovesBusinessAccess` 在旧实现真实返回一张工单而失败；补齐 M3 列表、领域 CanRead/canRepair、应用 canAct 的当前区域条件后通过。报修人仍可查询本人历史，但“本人也是维修人”不能绕过维修动作的区域权限。

## 实际验证

|检查|真实结果|
|---|---|
|新增消息 HTTP 测试|先因接口 404 失败，再通过；真实 Cookie、CSRF、Gin、MySQL|
|新投递与权限测试|先因投递端口缺失编译失败；补齐后验证并发领取、重启、失败边界、缺渠道、重派/禁用/撤区、旧租约回执、目录缺失与坏地址、失效升级|
|真实 SMTP|`TestNotificationRealSMTPPipeline` 在真实 loopback TCP 接收器 MAIL/RCPT/DATA 后观察邮件内容与 DELIVERED；`TestNotificationSMTPUnknownReplySameMessageIdentity` 在已收信但断开确认后重试，Message-ID 不变，工单/事件不重复|
|全后端|四个隔离 DSN 的 `go test -mod=readonly -race -count=1 -json -cover ./...`：91 个顶层 +18 个子测试通过，0 失败/跳过。M3 应用 60 顶层，覆盖率 81.3%；无测试文件的包不算测试通过|
|静态与构建|Go vet、后端构建、前端 30 测试与 TypeScript/Vite 构建通过；既有 >500kB bundle 提醒仍存在|
|破坏验证|只在私有副本移除消息本人限制，真实读到他人消息而失败；移除结果回写租约令牌检查，旧失败覆盖新处理中租约而失败；恢复后全部 13 个 M3 新通知测试通过，工作区产品源码未被破坏|
|浏览器|虚构主管两条历史升级消息：标一条已读，未读 2→1；只看未读显示 1 条；恢复全部显示 2 条，未配置渠道/0 次发送明确可见。实际 API 随后复核一致|

首次本人隔离破坏探针误造非法 SQL 得到 500，没有计入有效业务破坏验证；修正探针后取得“他人可读”的真实失败，恢复通过。OpenAPI JSON/引用一致性单独核对；本轮没有运行标准 JSON Schema 验证器，不将类型断言冒充该项验证。

原始证据位于本地 `.git/round5/2026-10-23/`：go-all.jsonl、go-result.json、frontend-check.log、各 red/green 日志、mutation-result.json、live-api-evidence.json 与 m3-notifications.png。不包含登录响应/密码/会话/CSRF。保留原有个人草案。

## 尚未完成与接入条件

|项|现状与下一步|
|---|---|
|M2 共享任务/outbox|代码中没有正式公共实现；当前 M3 领取适配器为开发过渡。评审共享 Enqueue/领取/确认/重试/可控时钟签名后替换存储适配，不复制第二套生产公共基础|
|M2 公共审计/空间/完整授权|保留已有开发目录与事务事件，M3 不改 identity/classroom 内部数据；正式适配需提供方交付与双方验证|
|M1/M2 维修影响 INT-04|M1 当前在资源事务中处理自己的预约；M2 尚无公开维修影响接收端点。需要三组确认字段、成功界点、来源版本/时间范围、重复乱序及回执，再各自实现，不由 M3 写 m2_applications 或替 M2 发活动通知|
|通知 D-03 公共完成|只有 M3 本人接口和测试渠道，M1/M2 还未接入共同事件/查询入口；生产邮箱归属验证、生产渠道与统一失败处置仍未完成；不能标为全部实现|
|联合人工验收、CI 与部署|本轮是本地验证；未运行远程 CI、没有服务器部署或组内评审结论。最后交付轮仍需真实跨组联调、全流程人工验收、review/部署与回滚材料|

远程 main 的新增文档仍将 M3 描述为只有领域规则，与本分支四轮实现不同；不自动合并或覆盖，需人工在后续 PR 对照真实实现统一说明。用户授权的后续上传只面向 M3 开发分支，main 与组内评审状态不变。

上传前最终回归日志：`.git/round5/2026-10-23/prepublish-go-all.jsonl`、prepublish-vet.log、prepublish-build.log、resource-summary-red.log；最新破坏与恢复证据在 `.git/round5-publish/2026-10-23/`。前端源码与已测快照哈希一致，未修改或重复运行前端测试。
