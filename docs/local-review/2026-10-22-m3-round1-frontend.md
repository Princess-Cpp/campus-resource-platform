# M3 第一轮：前端独立交付与真实边界

日期：2026-10-22，Asia/Shanghai。用户已要求开始第一轮，并明确回复“尚未确认，保留待评审状态”，指 M3 schema/迁移编号及 M2 权限、目录、事务审计/outbox。故本轮实现不依赖这些结论的前端组件和交互逻辑；SQL/API/业务集成仍未完成，不能把整轮标为完成。

## 基线与方案

工作区：codex/m3-alignment@3b7d897。远程main@aff197a已合并业务目录调整；本轮核对 frontend 在两版本间无差异。因本地未提交文档与远程文档存在重叠，本轮不自动切换、覆盖、stash、rebase或merge；保留原文件。本轮新增的前端代码可用于新后端目录基线，未来后端仍采用 maintenance/identity 等最新路径。

采用既有 React/TypeScript、Vite、shadcn样式与公共 Page/useRequest/time/UI组件；没有修改前端外壳或其他模块。调用侧类型为草案，不是正式OpenAPI或M2已承诺的接口。

对应需求：M3-01.1 的文本表单；M3-01.7 的本人列表/详情展示。领域规则已由他人提交，本轮不重写；服务端仍须独立执行权限与规则检查。

## 新增文件

| 位置 | 职责 |
| --- | --- |
| frontend/src/modules/m3/types.ts | 调用侧房间、报修、列表、详情和Gateway草案 |
| frontend/src/modules/m3/reporting.ts | Unicode校验、规范化、日期区间、状态显示、同请求重试控制 |
| frontend/src/modules/m3/components/shared.tsx | 状态、错误和表单控件样式 |
| frontend/src/modules/m3/pages/{report,orders,detail}-page.tsx | 文本上报、筛选分页列表、详情与时间线 |
| frontend/src/modules/m3/module.tsx | 可注入Gateway的模块定义；未注册进公共外壳 |
| frontend/src/modules/m3/dev/{gateway.ts,preview.tsx} | 只用于交互预览的示例数据与独立入口 |
| frontend/dev/m3-preview.html | 独立开发预览HTML |
| frontend/src/modules/m3/reporting.test.ts | 11个前端逻辑/适配器测试 |
| scripts/check-m3-preview.sh | 专用测试与构建命令，不修改原package.json默认测试入口 |

## 如何查看

现有Vite开发服务运行时，打开 http://127.0.0.1:5176/dev/m3-preview.html 。若未启动，使用已有本地启动方式 `bash .git/local-env/dev.sh web`。

预览明确标注“示例数据、不发送报修、刷新重置”；不读取真实学生信息、不访问后端、MySQL或公共身份。页面可以创建示例记录、筛选、分页、查看详情、模拟读取失败和创建响应丢失。已完成/已取消等种子状态仅用于展示，不代表本轮已实现这些动作。

该适配器保存在浏览器模块内存，仅模拟UI交互，不作为业务数据库、不用于正式权限/幂等/恢复验收。公共主路由未注册M3；默认Vite生产构建不以预览HTML为入口，预览脚本另有DEV保护。不得将此预览作为已完成报修模块或已通过真实跨模块联调的证据。

## 实际验证

| 项目 | 真实结果 |
| --- | --- |
| 首次新增测试 | 因 reporting.ts 尚未实现而失败；随后实现并复跑 |
| 前端测试 | M3 11个 + 原会话2个，共13通过、0失败、0跳过；不包含后端权限/SQL测试 |
| TypeScript/Vite | npm run build 通过；约1MB既有主JS大chunk警告保留 |
| 破坏验证 | 只在.git内副本中把pending请求的“仅首次生成键”改成“每次重新生成键”；响应丢失重试测试失败；恢复后M3全11测试通过。未修改正在预览的源文件；不代替后端幂等破坏验证 |
| 浏览器操作 | 空表单教室/描述提示；合法示例创建和详情；响应丢失后锁定/重试，示例总数仅增加1；每页5条第二页；类别空结果/清除筛选；读取失败隐藏旧列表且能恢复；最终详情显示座位名称 |
| 日期过滤 | 逻辑测试验证Shanghai整日与API右开区间、反向/非法日期；未逐项浏览器验收日期输入控件 |
| 视觉 | 在应用内浏览器约571px宽视口检查表单与详情布局；截图.git/round1/2026-10-22/m3-detail-preview.png；未完成390/1440全尺寸及屏幕阅读器验收 |
| M3 SQL/API/权限/通知/资源 | not_run；本轮无实现、无迁移或新公共权限 |

验证命令：`bash scripts/check-m3-preview.sh`。默认npm test目前仍仅执行原会话测试，不能用该命令声称M3已测。测试覆盖的是客户端真实逻辑与独立示例适配器，不模拟或替代尚未实现的后端。

## 已知限制与后续

- 第一轮尚未形成真实落库闭环。前端校验只帮助用户输入，服务端必须重验；预览没有认证或授权保证。
- 待评审：M3 schema/编号、report权限及区域、可信空间/座位目录、同事务审计/outbox端口。没有虚构评审结论。
- 正式Gateway与HTTP API尚无实现。接入前还需actor隔离的持久化未决请求、会话失效与撤权、响应丢失后的持久化核对、列表与详情授权，不能把目前浏览器内控制器当完整恢复。
- 后续新增SQL迁移须由版本化runner执行；不修改M1/M2内部表，不手工改业务库。
- 待真实API就绪后，替换示例Gateway、评审公共路由注册，再执行SQL/HTTP并发、权限与浏览器人工验收；移除验收中的开发适配器。
- 本轮未提交、推送、合并、远程写入或部署。用户已有文档不整文件覆盖；仅对baseline.json中M3-01.1/M3-01.7增量记录本轮局部实现及证据，保留此前修改。
