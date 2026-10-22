# M3 验收与交接入口

2026-10-23；当前基线 `5137d28205daf67ec0c022bd8f955f9bd19627af`，总约定 v0.2、需求 v0.4。本轮沿用原计划 Task 8/T12，完成本地交付检查入口和验收材料，不增加业务接口、表或默认政策。个人原有草案保留，不自动提交或上传。

## 复跑自动检查

前提：仓库要求的 Go、Node/npm、Python 3、bash 已安装，frontend/node_modules 与既有 Go embed 目录已按项目环境准备。启动独立 MySQL 并建立四个专用测试库，不得指向日常演示库或真实业务库。凭证只在私有环境变量注入，不写入本文或提交。

|环境变量|唯一允许的测试库|
|---|---|
|M1_TEST_DSN|campus_m1_check|
|M2_TEST_DSN|campus_m2_check|
|IDENTITY_TEST_DSN|campus_identity_check|
|M3_TEST_DSN|campus_m3_check|

DSN 使用 Go MySQL 驱动的 tcp(...) 或 unix(...) 格式，带 `parseTime=true&multiStatements=true`。测试会清理这些库里的测试记录，不使用演示环境。不能并发启动两个针对同一测试库的验收进程。

在仓库目录执行：

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -v
python3 scripts/check-m3-delivery.py
```

检查入口先拒绝缺失/错误库名，再执行全后端 race 行为测试；要求六个业务/身份/通知包都有非零顶层测试，并检查核心维修来源、响应丢失、评价、升级、消息隔离和真实 SMTP 用例确实执行成功。有测试失败、跳过、关键测试缺失或零执行时返回非零，不继续后续构建。没有测试文件的包不当成通过的业务测试，也不误算为跳过的业务用例。然后执行 vet、后端构建、既有前端30测试及 Vite 构建。

每次产生独立 `.git/m3-checks/<UTC运行标识>/`，含源码哈希、Git 基线、分项退出码、实际测试数、脱敏日志和 summary.json。控制台不输出 DSN 或原始测试错误；日志替换已知凭证，仍只保存到本地，分享前检查是否有学生数据。失败时看 summary.json 的具体阶段；不得把单个命令退出0替代完整入口通过。

此入口不安装依赖、不自动建库或启动数据库、不推送、不部署，不改变业务 schema。后端构建只验证当前 embed 目录可编译；本轮没有重做单应用静态资源打包或发布演练，不能将它称为可发布全组制品。

## 当前实现与界面

|需求|本地实现/界面|保留的未完成内容|
|---|---|---|
|M3-01.1|文本上报 `/m3/report`，授权空间校验、幂等；MySQL持久化|图片/相似单管理暂缓；正式目录接入待完成|
|M3-01.2|`/m3/dispatch` 派单/重派，`/m3/assigned` 接单；当前区域撤权生效|正式公共权限/人员目录评审|
|M3-01.3|开始维修持久化操作并真实调用 M1 HTTP，确认才 IN_PROGRESS|M2既有活动影响、停用空间规则差异待协调|
|M3-01.4|完工、独立验收/拒绝返工、按来源恢复|跨组联合人工验收|
|M3-01.5|本人早期取消，维修中无普通取消|双人异常关闭属加固，未提供入口|
|M3-01.6|本人完成后一次评价|7天/分值等仍为提案；评价内容处置未交付|
|M3-01.7|本人/区域/维修/验收查询，`/m3/stats`基础统计|高级报表暂缓；正式参数评审|
|M3-02.1|`/m3/escalations`阶段升级及缺主管异常、重复/重启补扫|共享任务接入、正式时限与主管配置评审|
|M3-02.2、D-03.1/03.2|`/m3/notifications`本人消息/已读，本地SMTP与有限重试|公共审计/任务/outbox、全组通知入口、生产验证邮箱/渠道|

页面权限由后端强制检查；看见菜单不能当作具备动作权限。开发绑定只有私有配置明确列出的稳定用户 ID 有效，不靠学工号模式、自助注册或客户端角色授予权限。

## 逐角色人工验收及录屏顺序

使用虚构报修人、另一个学生、区域调度员、两名维修人、独立验收人、主管、统计员。角色可用多个浏览器会话或依次退出登录，不修改公开种子冒充团队正式授权。本表是待执行清单，本轮不代填成员姓名、组内决定或人工通过。

|案例/需求|动作与观察|现有自动证据入口|人工结果|
|---|---|---|---|
|AC-01 /01.1、01.7|学生文本报修，刷新仍存在；重试同键仅一单；另一个学生读该详情拒绝|TestReportingPersistsAndMigrationReplays、TestOwnerQueriesFiltersAndPagination|not_run|
|AC-02 /01.2|调度员给有效辖区维修人派单；维修人接单后仍为ASSIGNED，尚未下架|TestDispatchQueriesCandidatesAndAcceptance|not_run|
|AC-03 /01.3、INT-02|开始维修先显示待确认；确认后IN_PROGRESS；M1搜索排除且直接预约拒绝|TestRepairHTTPFullLifecycleAndIndependentAcceptance|not_run|
|AC-04 /01.2|维修中交接，旧人失去动作权；资源阻塞保留，计时不重置|TestReassignmentRevokesOldWorkerAndPreservesClock、TestRepairHTTPFullLifecycleAndIndependentAcceptance|not_run|
|AC-05 /01.4|提交完工仍阻塞；维修人/曾参与者自验收拒绝；独立验收人拒绝后继续维修|TestRepairCurrentAndHistoricalParticipantsCannotSelfAccept、完整维修链路|not_run|
|AC-06 /01.4、INT-03|两张同室工单只完成一张，另一M3和M2来源保留，停用标记不被恢复|TestRepairReleasePreservesOtherM3AndM2Sources、TestRepairReleaseDoesNotEnableDisabledRoom|not_run|
|AC-07 /01.5|本人未维修前可取消，终态不能再派单；维修后普通取消拒绝|TestEarlyCancellationTerminalAndForgery、TestRepairStartAndCancellationRaceOnlyOneVersionWins|not_run|
|AC-08 /01.6|本人对完成单评价一次，其他人/过期/非法分值失败；评价不改资源|TestEvaluationOnlyReporterAndSameKeyReplays、TestEvaluationInputWindowAndHistoricalReplay|not_run|
|AC-09 /02.1|可控时钟在期限边界升级一次，重启不重复；缺主管可见且后续路由重试|TestEscalationBoundaryConcurrentAndRestart、TestEscalationMissingDirectoryRoutesSameEventAfterRecovery|not_run|
|AC-10 /02.2|本人未读→已读与筛选；他人/管理员不能标记别人的消息；无渠道明确未配置|TestNotificationInboxOwnerReadIsolation、TestNotificationDeliveryRetryLimitAndUnconfigured|not_run|
|AC-11 /02.2|本地SMTP接收器真实收信；确认响应丢失仍沿用同一标识重试，工单不重复流转|TestNotificationRealSMTPPipeline、TestNotificationSMTPUnknownReplySameMessageIdentity|not_run|
|AC-12 /01.7|授权统计员核对分母/样本，无评分为null；跨区域不泄露明细|TestOperationsPermissionScopePrivacyAndStatistics、TestStatisticsRealDurationsReworkCancellationAndEmptySet|not_run|

录屏先展示版本、开发环境标记和虚构角色，再录 AC-01→02→03→05→06→08→10→12；并发、期限边界和失联恢复用真实测试日志补证，不靠录屏等候24小时或手改业务库。所有资源准备/释放显示等待确认，不将收到202录成成功。

每个实际验收记录：日期、参与者、基线/工作区哈希、案例编号、工单/operationId、correlationId、HTTP结果、来源变化、截图/视频位置、通过/失败/未运行。不要录密码、完整Cookie/签到码、真实联系人。失败项保留并关联后续修复，不清空记录制造通过。

## 联调与发布剩余清单

|负责边界|需要的交付|验收出口|
|---|---|---|
|M2 提供、M3接入|公开主数据/生命周期与参数、完整RBAC和当前人员信息|替换开发目录，撤权与禁用及停用报修组合通过|
|M2 共享基础、M3通知|同事务审计/outbox、任务领取/确认/重试/时钟接口版本|替换过渡存储/调度，三组用同一公共库；公共调用不跨写业务表|
|M1/M2/M3 INT-04|维护影响事件/回执端点、来源版本、时间范围及重复乱序处理规则|M2自己取消/终止活动并通知；重复/迟到事件不误伤新业务；M3不写预约/申请表|
|三组共同评审|资源契约OpenAPI、迁移010与字段、DEC-02至12|记录真实参与者与结论，再更新约定；M1现有事务内处理与文档事件链差异待确认|
|M3牵头交付|空库重放/升级、幂等种子、全组单应用打包、CI门禁、备份恢复|在隔离环境实演，之后有服务器再做测试部署、生产审批/版本/回滚|

远程 main 仍有旧M3进度文档，与当前M3分支实现不同；保留差异，不自动合并或覆盖。以上依赖未解决前，“第六轮本地检查通过”不代表全部基线、人工联合验收或课程最终交付完成。

## 本轮实际执行结果

- 新入口完整跑通：后端91个顶层、18个子测试，0失败/跳过；M3应用60个顶层，六个必测包与八个关键流程用例均实际通过。vet、后端构建、前端30测试及Vite构建通过，原有包大小提醒保留。
- 检查入口自身5个测试通过；实际缺DSN、错误指向演示业务库的调用均以2退出，没有启动业务测试。只在私有副本移除“跳过即失败”条件，门禁测试按预期失败；工作区恢复版本5个测试通过。该验证只证明检查器，不冒充业务破坏验证；既有业务权限/来源/租约破坏证据见各轮实施记录。
- 原始分项日志、实际计数、源码哈希位于 `.git/m3-checks/20261022T171655128746Z/`；门禁负例与副本破坏日志位于 `.git/round6/2026-10-23/`。运行标识采用UTC，对应本机2026-10-23。
- 本轮没有改业务代码、接口或SQL，没有运行空库部署/备份恢复、远程CI或生产发布。已有Go embed内容可编译，但未将新Vite产物重新打包为全组单应用制品；该项明确未验收。
- AC-01至AC-12仍等待成员逐项人工验收；公共依赖与INT-04仍待提供方交付和双方评审。本轮没有提交、推送或合并。

后续本地验证已进一步完成新前端内嵌打包、空库001至010迁移、二次启动种子/会话持久化，以及31表测试数据库备份恢复；具体成功与失败记录见[单应用验证](local-package-validation.md)。这更新了上节“未执行”的后续状态，不表示已完成正式发布脚本、生产部署或人工验收。

用户随后授权第六轮交付；上传前复核进一步收紧特殊DSN库名检查与前端非零/无跳过门禁，检查器现有6个测试，Node两种摘要格式均受验证。历史5测试记录保持原样；后续最终结果及未完成边界见[第六轮复核](../local-review/2026-10-23-m3-round6-prepublish.md)。
