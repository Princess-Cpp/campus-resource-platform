import { useMemo } from "react"
import { Page } from "@/modules/common/page"
import { request } from "@/modules/common/api"
import { useSession } from "@/modules/common/session"
import { useRequest } from "@/modules/common/use-request"
import { ErrorBox } from "./components/shared"
import { createHTTPGateway } from "./transport"
import { pendingStore } from "./pending-store"
import { ReportSubmission } from "./reporting"
import { ReportPage } from "./pages/report-page"
import { OrdersPage } from "./pages/orders-page"
import { OperationsPage } from "./pages/operations-page"
import { DetailPage } from "./pages/detail-page"
import { NotificationsPage } from "./pages/notifications-page"
import { canOpenMaintenancePage, type MaintenancePage } from "./navigation"

const gateway = createHTTPGateway(request)
export function LiveMaintenance({ page }: { page: MaintenancePage }) {
  const auth = useSession()
  const current = useRequest(() => gateway.session(), [auth.data?.actor.id])
  const submission = useMemo(() => {
    if (!auth.data) return undefined
    try { return new ReportSubmission(gateway, () => crypto.randomUUID(), pendingStore(auth.data.actor.id, sessionStorage)) }
    catch { return undefined }
  }, [auth.data?.actor.id])
  if (current.error !== undefined) return <Page title="物业报修"><div className="px-4 lg:px-6"><ErrorBox error={current.error} /><p className="mt-3 text-sm text-muted-foreground">报修服务暂时无法加载，请稍后刷新重试。</p></div></Page>
  if (current.loading || !auth.data) return <p role="status" className="p-6 text-sm text-muted-foreground">正在加载报修服务…</p>
  if (current.data?.actor.id !== auth.data.actor.id) return <Page title="物业报修"><p className="px-4 text-sm text-destructive">登录身份已变化，请刷新后继续。</p></Page>
  if (!canOpenMaintenancePage(page, current.data.actor.permissions)) return <Page title="无权访问此页面"><p className="px-4 text-sm text-muted-foreground lg:px-6">你当前的账号没有此业务的操作权限，请从菜单选择可用功能。</p></Page>
  return <>
    {page==="notifications"?<NotificationsPage gateway={gateway}/>:page==="stats"||page==="escalations"?<OperationsPage key={page} gateway={gateway} view={page}/>:page === "report" ? submission ? <ReportPage gateway={gateway} submission={submission} /> : <Page title="故障报修"><p className="px-4 text-sm text-destructive">浏览器无法保存报修进度，请允许本站存储数据后刷新。报修尚未发送。</p></Page> : ["orders","dispatch","assigned","acceptance"].includes(page) ? <OrdersPage key={page} gateway={gateway} canReport={canOpenMaintenancePage("report",current.data.actor.permissions)} view={page==="dispatch"?"dispatch":page==="assigned"?"assigned":page==="acceptance"?"acceptance":"mine"} /> : <DetailPage gateway={gateway} actions={gateway} actorId={current.data.actor.id} />}
  </>
}
