# M3 第三轮：维修下架、独立验收与恢复

日期：2026-10-22。分支 `feat/m3-reporting`，实施基线 `359f0b7a40596c714e373d058e1fed6aa5f83ba6`。依据任务书、总约定 v0.2、需求基线 v0.4、范围分级及[本轮计划](../superpowers/plans/2026-10-22-m3-repair-recovery.md)。本轮实施结束时保留本地开发与自动验证结果；用户随后授权“上一轮完成且检验好了之后可以上传”。本次按[上传前复核](../local-review/2026-10-22-m3-round3-prepublish.md)仅交付到团队仓库开发分支，不合并或部署，不把上传授权记成正式业务人工验收或组内评审。

## 本轮完成到哪里

已有的文本报修、查询、区域派单、重派、接单与早期取消继续保留。本轮将已有领域规则接入实际 SQL、HTTP 和页面：当前维修人开始维修，全室资源阻塞成功后进入维修中；填写处理结果后待验收；独立验收人可驳回返工或通过验收；仅在本工单来源释放回执确认后完成。新增验收工作台、资源操作进度和按原操作继续核对。

M3 只保存自身数据，通过现有 M1 v1 HTTP 操作资源。没有修改 M1/M2/identity 内部实现，也没有由 M3 访问它们的内部业务表。测试中的身份、资源清理及角色设置仅在已校验名称的 `campus_m3_check` 隔离库执行；本机演示账号通过公开注册/登录创建，物业角色用私有开发绑定，不改 D 角色种子。

## 状态、权限和成功界点

|操作|服务端规则|业务结果|
|---|---|---|
|START|当前维修人、已接单、ASSIGNED、IDLE|同事务保存请求，返回 202/PENDING；M1 下架回执确认后 IN_PROGRESS|
|FINISH|当前维修人、已接单、IN_PROGRESS、IDLE；处理结果必填|AWAITING_ACCEPTANCE；保留维修阻塞|
|REJECT_ACCEPTANCE|当前辖区验收权限；排除所有历史维修参与人；原因必填|返回 IN_PROGRESS；保留阻塞、阶段累计耗时和历史意见|
|APPROVE_ACCEPTANCE|同一独立验收规则，AWAITING_ACCEPTANCE、IDLE|先 202/PENDING；只释放本工单来源，确认后 COMPLETED|
|RETRY_RESOURCE|未知结果已暂停，当前仍有原动作相应权限|复用原操作 ID、请求和来源版本继续查询，不创建新来源|
|view=acceptance|workorder.accept 与当前区域|只显示待验收且本人未参与维修的工单；详情/动作再次检查权限|
|CANCEL/重派等|沿用原归属、版本和状态规则|资源同步未确认期间禁止矛盾动作；早期取消与开始维修只允许一个版本胜出|

历史维修参与人包含首次开始维修人及维修中重派后接单的人员；不能通过换人再换回规避自验限制。服务端授权先于历史幂等响应重放，客户端不能传入身份或权限。终态不再暴露处理按钮。页面的 202 只说明命令已可靠保存，不宣称资源已下架或已恢复。

## 事务、来源和恢复

新增候选迁移 `008_m3_resource_operations.sql`，不改已执行的 001–007。`m3_resource_operations` 保存操作 ID、工单 ID、来源版本、不可变请求、原命令用户/键、关联 ID、PENDING/PAUSED/CONFIRMED/FAILED、尝试数、下次执行时间、错误与回执；工单+来源版本唯一。迁移仍需团队编号和 schema review。

开始/验收命令在一个 MySQL 事务中保存工单快照、资源操作、命令响应、审计事实和 outbox。先提交再联网，不在持有业务事务/工单行锁时等待 HTTP。后台每两秒扫描到期操作；连接级命名锁避免多进程/并发工作者重复发送。应用重启从数据库继续，不依赖浏览器内存。

每次先 GET 查询原操作回执；404 才 POST 原始请求，`Idempotency-Key=operationId`。请求携带 `sourceModule=M3`、`sourceBusinessId=工单ID`、递增 `sourceVersion`、房间与固定短原因；不把最长 1000 字的中文意见直接塞进 M1 最多 500 字节的 reason。服务凭证仅在后端，HTTP 超时五秒，禁止重定向和浏览器 Cookie 转发。

回执必须匹配操作 ID、M3 模块、工单 ID、来源版本、终态状态、错误与生效时间。确认后再次锁定当前工单，同事务更新业务状态、原命令进度、回执、审计和 outbox。局部提交失败则事务回滚，后续重查 M1 已提交回执，不能因本地失败重复制造来源。

|资源结果|持久化处理|页面和后续|
|---|---|---|
|下架 CONFIRMED|IN_PROGRESS / IDLE|可提交维修完工|
|下架明确 FAILED|仍 ASSIGNED / IDLE，保存错误|修复资源原因后可主动重新 START，新操作/新来源版本|
|恢复 CONFIRMED|COMPLETED / IDLE|只说明本工单来源已解除，仍需查询其他限制|
|恢复明确 FAILED|仍 AWAITING_ACCEPTANCE / NEEDS_ATTENTION|保留阻塞和失败回执；需协调原因，不提供无意义的同操作重发|
|超时、丢包、无效回执、认证/传输错误|UNKNOWN，保留原操作|指数退避 2～256 秒；八次未确认后 PAUSED / NEEDS_ATTENTION|
|未知操作继续核对|原操作改回 PENDING|保留相同操作 ID、请求、来源版本，避免重复下架/错误释放|

不会以超时、重试耗尽或待验收期限推定阻塞消失，不会按预计完工时间自动恢复，不会清除其他工单、M2 来源或管理员停用限制，不会复活被 M1 取消的旧预约。

## 需求到验收案例

以下均在本地真实 MySQL 与 Gin HTTP 执行；M3→M1 经 httptest 网络服务器调用实际 M1 业务。故障注入只在 HTTP 回包边界丢弃/修改响应，不模拟资源成功逻辑。

|需求 ID|验收测试/案例|实际结果与剩余范围|
|---|---|---|
|M3-01.3|TestRepairRequiresAcceptedAssignment；TestRepairHTTPFullLifecycleAndIndependentAcceptance|未接单拒绝；202 先不改变业务状态；回执后维修中；M1 查询及直接预约均受阻|
|M3-01.3|TestRepairExplicitBlockFailureCanDeliberatelyStartAgain；TestRepairMissingResourceConfigurationFailsClosed|M1 明确失败不误进维修；无配置时不开放开始按钮，写入失败关闭；失败后主动新操作可成功|
|M3-01.4|TestRepairHTTPFullLifecycleAndIndependentAcceptance|处理结果持久化；完工仍阻塞；验收驳回返工，第二次完工/验收可以完成|
|M3-01.2、M3-01.4|TestRepairCurrentAndHistoricalParticipantsCannotSelfAccept|维修中交接保留参与历史；当前和历史维修参与人即使有验收权限也不能自验；独立验收人可以处理|
|M3-01.4、INT-02 部分|TestRepairReleasePreservesOtherM3AndM2Sources；TestRepairReleaseDoesNotEnableDisabledRoom|完成一单仍受其他 M3/M2 来源或停用约束；只解除自己的来源；旧预约不复活|
|M3-01.3/01.4 恢复|TestRepairLostStartReplyRestartsWithSameOperation；TestRepairLostReleaseReplyDoesNotPrematurelyComplete|M1 已成功但回包丢失，M3 保留 UNKNOWN；重建应用后查询原回执恢复，无重复 POST|
|M3-01.3/01.4 回执|TestRepairWrongReceiptIsUnknownAndCannotFinish|错工单来源回执不能推进；同步期间不能完工；正确回执可继续恢复|
|M3-01.3/01.4 重试|TestRepairUnavailableExhaustionAndManualSameOperationRetry|八次未知后暂停而不认定失败；仍禁止取消；手动继续使用原操作|
|M3-02.2、恢复|TestRepairLocalOutboxFailureAfterRemoteCommitRecovers|M1 成功、本地 outbox 故障时本地状态回滚；修复后重查回执，无重复资源请求|
|M3-01.4|TestRepairKnownReleaseFailureKeepsSafetyAndRejectsRetry|资源端版本栅栏明确失败时仍待验收，来源保留，不能伪造完成或把终态失败改成未知重试|
|并发/幂等|TestRepairConcurrentSameCommandAndWorkersAreIdempotent|相同命令并发返回同一个操作；多个工作者只发送一次；已确认操作重跑无重复事实|
|重启恢复|TestRepairDurablePendingOperationResumesThroughTick|命令返回时尚未发 HTTP；重建应用后 Tick 从 SQL 继续到维修中|
|M3-01.5 部分|TestRepairStartAndCancellationRaceOnlyOneVersionWins|同版本开始维修与本人取消只有一个成功，取消成功时不残留资源操作；异常关闭仍未实现|
|M3-01.7|验收工作台、处理结果和操作详情查询|当前辖区过滤、参与人排除和查询详情经 HTTP 验证；主管工作台及耗时评分统计仍未实现|
|前端动作|actions.test.ts 新增四个案例|完工/驳回意见校验；PENDING 不被当成维修成功；丢包验收请求保留原键；验收查询/继续核对走 M3 API|

最终全包 `go test -mod=readonly -race -count=1 -json -cover ./...`：**67 个顶层测试、18 个子测试通过，0 失败、0 跳过**。其中 M3 应用层 37 个顶层，领域层 12 个顶层+12 子测试；应用层语句覆盖率 81.1%。Go vet、后端构建通过。独立 DSN 分别指向 campus_m1_check/campus_m2_check/campus_identity_check/campus_m3_check，不能把缺失 DSN 导致的跳过当作通过。

列表只返回工单同步状态，操作次数/下次执行时间从详情的同一只读快照查询，避免把领域快照中的默认次数误当真实进度。该收尾调整后，完整流程、独立验收、区域查询和未知重试四项定向 race 回归通过。

前端 `bash scripts/check-m3-preview.sh`：**26 个测试通过**，TypeScript/Vite 构建通过；仍有主产物超过 500 kB 的既有警告。本轮没有性能优化或正式部署。

关键规则破坏验证在 `.git` 内的独立源码副本完成：删除回执工单 ID 检查，错工单回执被接受，测试失败；将释放请求的业务 ID 改成另一工单，来源保护测试失败。恢复副本后全部 16 项维修集成测试通过；破坏验证期间工作区领域和动作源码哈希未变。最初的未接单开始测试先以旧接口返回 400 失败，再实现后返回状态冲突并通过；旧的“不支持动作”回归改用真正的 UNSUPPORTED_ACTION，保留伪造字段断言。继续核对资源也检查时间不可倒退，避免绕过领域命令的时间约束，已有重试案例包含该断言。

OpenAPI 的 Order 和 Detail 字段同步更新。用现有 Ajv draft 2020-12 校验实际浏览器演示工单的详情和六份已保存命令响应，均通过；这次校验未开启 date-time 格式验证，业务时间约束由领域与 HTTP/SQL 测试验证。

## 浏览器和本机配置

新增虚构演示工单 `WO-b335aa6a0263f6bc1eeca978d7b420a5`，未覆盖前两轮工单。浏览器实际完成开始维修 → 待回执 → 维修中 → 填写结果完工 → 独立验收 → 待恢复回执 → 已完成。维修期间 M1 查找页显示无座位，验收恢复后同一 18:00–20:00 时段显示一个教室、两个座位。当前页面可供人工查看，不代表用户已经验收。

新增配置 `M3_RESOURCE_URL=http://127.0.0.1:8082` 和 `M3_SERVICE_TOKEN`，必须同时提供；凭证对应现有 M1 身份中的 M3 服务，不接受浏览器提交。两者都未配置时保留报修/查询/派单能力，开始维修/验收恢复失败关闭。M3_DEV_STAFF_FIXTURE 支持显式 `workorder.accept` 绑定。测试时曾因本地绑定使用未规范化的用户 ID 造成验收待办 403，现已使用真实登录响应的 actor.id 修正；未改公共用户角色。

本机仍使用 MySQL :13306、Gin :8082、Vite :5176，重启命令沿用 `.git/local-env/dev.sh api` / `web`。仅私有 `.git/local-env` 保存服务令牌和虚构账号密码，权限 0600，不提交。其他开发者应通过既有公开登录取得自己的虚构用户 actor.id，然后建立本机绑定：

```json
[{"userId":"actual-actor-id-from-local-login","permissions":["workorder.accept"],"areas":["A楼"]}]
```

审计和通知只保存 M3 事务事件/待投递意图。START、APPROVE_ACCEPTANCE 的事件说明为“申请/等待回执”；资源确认另有事件。没有发送真实邮件、站内投递或通知外部人员。

## 仍未完成与契约差异

- 目录、人员权限映射和事务事件仍为显式开发适配器。公共 schema、008 编号、默认 SLA/验收政策、公共库和人员映射仍待组内评审；审批状态不改变。
- M1 目前在资源阻塞事务内自行取消已有个人预约，尚未提供总约定中的可靠影响事件。本轮调用现有公开行为并验证旧预约不复活，不宣称影响事件的重复/崩溃/违约补偿联合验收完成。
- M2 已批准/已开始活动受维修影响的处理尚未接入，INT-04 保持未完成。M3 不直接修改审批或活动数据。
- INT-02 仅完成下架/独立验收/按来源恢复的本地 HTTP 子链路；影响事件等剩余验收未完成。INT-03 只有 M3 操作/来源/关联 ID 的证据，跨模块来源解释、跳转与管理查询尚未实现，不替 M1 标记整体完成。
- 现有 M1 对 inactive 教室新增维修阻塞返回明确失败；M3 可报修但不能宣称该教室已开始维修，仍需团队收敛规则。
- 验收失败的资源版本冲突需要协调资源端原因；不手工改库、不解除其他来源、不自动创造成功回执。
- 超时升级、评价、统计、正式通知投递、真实公共适配联合验收、生产部署及人工验收仍未完成；本轮没有将这些需求降级或删除。

本地证据：`.git/round3/2026-10-22/` 下 go-all-final.jsonl、go-result.json、go-vet.log、go-build.log、frontend-check.log、red.log、m3-expanded.log、mutation-*.log、mutation-result.json、contract-validation.json、live-order.json、live-sql-evidence.json、m3-completed-top.png、m3-completed.png。原始日志/虚构账号和截图默认只留本地。前两轮文档和个人草案保留，本轮通过新增记录补充当前进展。
