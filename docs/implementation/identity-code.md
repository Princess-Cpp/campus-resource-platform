# SQL 身份、注册与角色跳转实现记录

身份公共能力现位于 `backend/internal/identity`，由 M2 维护。目录调整见 [后端目录说明](backend-package-layout.md)。

依据：D-01.1、D-01.2、D-05.1、D-05.2，模块设计与集成总约定 v0.2，实施基线 Git `b7f44e9`。本轮用户确认任意学工号和姓名可自助注册学生账号，不校验名册；院系与角色由服务端决定，审批人和管理员不能自行注册。本记录是本地开发与自动验证结果，不代表组内评审、人工验收、远程部署或生产就绪。

## 方案与契约

- `backend/migrations/005_identity.sql` 保存学生名册、用户、角色、权限、范围、联系方式、服务端会话和登录失败记录。密码使用 bcrypt；浏览器只持有随机 `HttpOnly`、`SameSite=Lax` 会话 Cookie，数据库仅保存令牌 SHA-256 摘要。
- `POST /api/v1/auth/register` 接受任意未占用学工号与姓名，忽略客户端额外角色字段并固定授予 `student`，院系默认为 `CS`。`login`、`session`、`logout` 及错误语义见 `contracts/identity.openapi.json`。
- 会话有效期 12 小时。Cookie 写请求要求 `X-CSRF-Token`；登录按学工号和来源 IP 进行 15 分钟窗口限流，5 次失败后阻断 15 分钟。进程关闭 Gin 默认代理信任（`SetTrustedProxies(nil)`），不采用客户端 `X-Forwarded-For`。禁用和 `auth_version` 变化使旧会话失效；每次鉴权重新读取角色、权限和范围。
- 登录/注册要求 `Content-Type: application/json`，并拒绝 `Sec-Fetch-Site: cross-site` 或 Origin/Referer 与请求 Host 不一致的跨站请求。此项仅 HTTP 层验证，未做浏览器端攻击演练。
- M1/M2 用户身份来自 SQL 会话，M1 预约入口校验当前 `booking.create`。M2→M1 的 Bearer 服务身份仍由开发 fixture 提供，且只接受显式服务账号；服务凭证放入浏览器 Cookie 不会被 M1 当作服务身份。
- 前端提供 `/login`、`/register` 公共路由，其余页面由会话守卫保护。登录或注册成功后按服务端 `landingPath` 跳转；侧栏显示真实姓名、学工号、院系并支持退出。通用 API 客户端使用同源 Cookie 和 CSRF，不再保存或切换开发令牌。

## 开发数据与运行

`AUTH_DEV_SEED=true` 时才注入虚构数据：名册学生 `20260001 / 演示学生 / CS`，以及 `DEV-DEP-001`、`DEV-VENUE-001`、`DEV-OP-001` 三个受控账号。三者密码来自未提交的 `AUTH_DEV_PASSWORD`；种子不会覆盖已有密码。启动示例：

```sh
cp .env.example .env   # 填入强数据库密码，.env 已被 .gitignore 忽略
docker compose up -d mysql
cd backend && go build -o server ./cmd/server
cd ..
export MYSQL_DSN='campus:<MYSQL_PASSWORD>@tcp(127.0.0.1:3306)/campus?parseTime=true&multiStatements=true'
export M1_CODE_SECRET="$(openssl rand -hex 32)"
export M2_SERVICE_TOKEN="$(openssl rand -hex 32)"
AUTH_DEV_PASSWORD='自行设置至少10字节的本地密码' scripts/dev-m2.sh
```

`MYSQL_DSN` 中的密码要与 `.env` 的 `MYSQL_PASSWORD` 一致；`M1_CODE_SECRET` 与 `M2_SERVICE_TOKEN` 必须至少 32 字节，脚本不再提供默认值。

另开终端执行 `cd frontend && npm run dev`。学生可使用任意学工号和姓名注册；审批与运营账号使用本地设置的同一开发密码登录。当前空间主数据和服务身份仍依赖 `M1_DEV_MODE=true` 的 fixture，因此不能视为正式 D-02 或生产身份交付。

## 2026-10-15 自动验证

- 独立数据库 `campus_identity_check` 上的真实 MySQL + HTTP 集成测试通过：任意学工号可注册、弱密码/重复注册拒绝，角色不可自选，Cookie 属性、会话查询与退出，错误密码和限流，过期、禁用、`auth_version` 失效，撤权即时生效，CSRF，SQL 学生访问 M1/M2，以及浏览器会话与服务身份隔离。开发种子重复执行不产生重复账号，三类受控账号登录后的角色落点符合约定。
- 2026-10-16 审查修复：撤除用户全部角色后，同一会话带有效 CSRF 再预约返回 403，不能得到 `RESERVED`。伪造不同 `X-Forwarded-For` 的第 6 次错误登录返回 429。`text/plain` 与跨站 Origin 登录/注册被拒绝且不 Set-Cookie。
- 故意关闭 `POST /m1/bookings` 的 `booking.create` 检查后，`TestSQLIdentityHTTPAndModuleIsolation` 按预期失败（撤权后预约 200/`RESERVED`）；恢复实现后测试重新通过。
- 故意将自助注册角色从 `student` 改为 `operator` 后，`TestSQLIdentityHTTPAndModuleIsolation` 按预期失败并显示落点变为 `/m2/stats`；恢复实现后全量测试重新通过。
- `go vet ./...` 通过。`M1_TEST_DSN=… M2_TEST_DSN=… IDENTITY_TEST_DSN=… go test -race -count=1 ./...` 通过，身份、M1、M2 测试均实际连接各自隔离数据库。普通测试用户首次执行 M2 触发器测试因 MySQL 1419 权限失败；改用本地测试库 root 账号后通过，未修改业务实现。
- `npm run build` 与身份 OpenAPI JSON 解析通过；构建仍提示主 JS chunk 超过 500 kB。IDE 对本轮前端文件未报告错误。

## 未完成与人工验收

- 未实现找回密码、角色/范围管理 UI、授权变更审计、保底系统管理员及正式服务凭证签发/轮换，所以 D-01.1、D-01.2 仍是部分开发，D-01.3 未完成。
- D-02 正式空间主数据、正式参数配置、M3 角色与业务接入未完成；当前角色集合只覆盖已经实现的 M1/M2。
- 2026-10-20：退出接口返回 401 时清除本地会话；`node --experimental-strip-types --test src/modules/common/session.test.ts` 通过（模拟 HTTP，无浏览器操作）。临时去掉 401 清会话后该测试失败并留下旧会话，恢复后通过。
- 登录、注册、刷新保持登录、各角色落点、退出和响应式页面尚待用户浏览器人工验收。未执行浏览器端 login CSRF 攻击流程。
- 未进行组内契约/schema 评审、远程部署、生产 Cookie/TLS 配置或生产数据迁移演练。当前直出服务不信任代理头；若日后经反向代理，需显式配置可信代理，不能默认信任。
