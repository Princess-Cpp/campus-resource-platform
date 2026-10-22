# 校园资源服务平台

软件过程管理与工程伦理课程实践项目。以自习座位预约为核心，提供整教室申请审批和物业报修服务，关注需求追踪、人机协作、跨模块集成和可验证的工程交付。

> 当前阶段：开发实现与局部验证。M1、M2 已有后端和前端初稿并通过部分真实 MySQL/HTTP 测试，D-01 身份已接入 SQL 会话；M3 仅完成领域规则；整体尚未人工验收、未做跨组完整联调、未部署服务器。具体业务政策与 D 分工提案仍待组内评审。

## 技术栈

| 层次 | 选择 |
| --- | --- |
| 前端 | React + Vite |
| 后端 | Go + Gin |
| 数据库 | MySQL |
| 组织与部署 | 同仓库、单应用部署；Go 后端提供前端构建产物，MySQL 独立运行 |

依赖版本已在 `go.mod`、`package.json` 和锁文件中固定。本地启动脚本为 `scripts/dev.sh`（M1）和 `scripts/dev-m2.sh`（M1+M2），构建脚本为 `scripts/build.sh`，MySQL 由 `docker-compose.yml` 启动；运行前提和开发账号见 [身份实现记录](docs/implementation/identity-code.md) 和 [M2 代码说明](docs/implementation/m2-code.md)。详细设计见 [架构文档](docs/architecture/proposal.md)。

## 模块与责任

| 模块组 | 业务职责 | D 公共能力（建议分配） |
| --- | --- | --- |
| M1 组 | 搜索、座位预约、签到、提醒与违约、历史与统计 | 资源冲突仲裁、来源追溯、资源 HTTP 契约 |
| M2 组 | 整教室申请、多级审批、撤销与超时处理 | 身份/服务身份、角色权限、空间主数据、审计、前端外壳与最小任务基础 |
| M3 组 | 故障上报、派单、维修、超时升级、验收与评价 | 通知、站内消息、迁移与部署；指定发布协调人 |

D 不单设第四组，各公共子域指定负责组，其他组负责各自接入。每组维护自己的前后端、测试和数据库迁移。详见 [模块组分工与协作边界](docs/process/module-ownership.md)。

## 文档阅读路径

文档按“来源 → 需求 → 集成与设计 → 执行 → 验证”组织。第一次了解项目按下面顺序阅读；开发某个模块时先看对应需求，再查它的公共依赖和集成约束。

### 1. 课程要求：为什么做、必须交付什么

阅读 [原始任务书 PDF](docs/source/Pj-任务书.pdf)，确认考核与交付约束。[来源对应表](docs/source/README.md)说明课程章节落在哪份项目文档，区分课程原文和项目补充。

### 2. 需求基线：系统要做什么

从 [需求入口](docs/requirements/README.md)了解共用规则，结合 [基线与加固范围](docs/requirements/scope.md)确定首版交付，再按业务读：

| 业务需求 | 公共依赖与关系 |
| --- | --- |
| [M1 座位预约](docs/requirements/m1.md) | 从 PDF 附件 1 整理；依赖 D 身份、空间、资源仲裁与通知 |
| [M2 整室申请审批](docs/requirements/m2.md) | 依赖 D 权限；最终审批占用 M1 所在教室的座位 |
| [M3 物业报修](docs/requirements/m3.md) | 依赖 D 派单权限与通知；维修下架、验收恢复 M1 资源 |
| [D 公共能力](docs/requirements/d.md) | 三个业务模块共同使用，按子域分配维护责任 |

然后读 [跨模块集成与待定决策](docs/requirements/integration-and-decisions.md)，确认模块之间怎样互相影响。模块验收与集成验收都要满足，不能各组单独完成就视为整个系统完成。

### 3. 设计与分工：怎么做、谁负责

**三组和 AI 先共同读取 [模块设计与集成总约定](docs/architecture/module-design-and-integration.md)**，统一边界、字段、接口、失败处理和验收，再读各自模块需求。

[架构](docs/architecture/proposal.md)将需求落实到 React、Go/Gin、MySQL 和模块接口；[分工](docs/process/module-ownership.md)指定业务及 D 子域负责组、接入责任和联调方。技术已选，业务政策与 D 分工仍有待评审项。接着读 [模块协作与集成开发](docs/process/module-collaboration.md)，明确接口如何共同确认、两端如何分别实现、失败怎样恢复和联调如何验收。

### 4. 迭代与交付：先做什么、如何发布

[项目计划](docs/project-plan.md)按迭代列出三组并行交付和共同出口；[AI 协作规则](AGENTS.md)约束每项任务的实现和验证；[Review 与版本流程](docs/process/reviews-and-versioning.md)约束评审、提交、部署与回滚。

[第一次项目讨论](docs/process/first-discussion.md)把 3×45 分钟的讨论安排、代码走读、待决问题、风险登记和下一迭代任务放在一起；其中未决内容不会被当成已经批准的需求。

### 5. 追踪与验收：完成了什么、证据在哪

[M1 代码说明](docs/implementation/m1-code.md)说明当前实现和适配边界；[进度](docs/progress.md)展示当前阶段和阻塞；[需求索引](docs/requirements/baseline.json)将每项需求关联到负责组、实现、测试和真实证据。它们跟踪前述要求，不另立一套需求。当前只有 M1 初始实现，不能据此宣称全系统已完成。
[M1 代码说明](docs/implementation/m1-code.md)、[M2 代码说明](docs/implementation/m2-code.md)、[M3 方案与证据](docs/implementation/m3-plan.md)和[身份实现记录](docs/implementation/identity-code.md)分别说明各模块当前实现与边界；[进度](docs/progress.md)展示当前阶段和阻塞；[需求索引](docs/requirements/baseline.json)将每项需求关联到负责组、实现、测试和真实证据。它们跟踪前述要求，不另立一套需求。目前已有 M1/M2 开发实现和自动测试证据，但人工业务验收、完整跨组联调和远程部署仍未完成。

## 开发与验收原则

- 先明确方案、约束和验收，再实现可审查的迭代；未完成需求持续记录。
- M2/M3 通过公开 HTTP 资源接口写回，不直接改写 M1 资源数据；状态记录 M1/M2/M3 来源。
- SQL 结构使用版本化迁移，种子幂等；测试验证权限、并发、时间边界和重试恢复。
- AI 变更经人工验收后提交，公共层、跨模块与 schema 变更共同评审；代码冲突由人处理。
- 构建测试失败阻止后续部署；生产人工审批并保留可追溯版本与回滚方案。

下一阶段先评审待定业务政策、公共层分工和资源契约差异，再补 M3 持久化/API/页面、跨组联调与人工验收。已实现模块的启动方式和测试记录见各模块实现文档；未执行的验证继续按需求索引保持 `not_run`，不以开发 fixture 结果替代生产验收。

## DeepSeek Harness 业务代理（附加交付）

任务书要求在主功能完成后以 DeepSeek Harness（Everything is a Plugin.，DSH）插件形式实现自然语言业务代理。代理通过 M1/M2/M3 的公开 HTTP 接口完成查询、预约、申请和报修，不能直连数据库或模块内部实现；缺少参数时追问，写操作先确认，结果必须以真实接口响应为准。DSH 不新增第四模块组。

[DSH 与豆包适配设计](docs/references/doubao-integration.md)记录插件边界、工具契约、授权、幂等和部署要求；豆包只是可替换的模型适配器，不等于 DSH 运行时。该附加交付安排在项目计划第 6 迭代，不替代主功能验收。
