import { useMemo } from "react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { DetailPage } from "./pages/detail-page"

const gateway = createHTTPGateway(request)
export function LiveMaintenance({ page }: { page: "report" | "orders" | "detail" | "dispatch" | "assigned" | "acceptance" }) {
  const auth = useSession()
  const current = useRequest(() => gateway.session(), [auth.data?.actor.id])
  const submission = useMemo(() => {
    if (!auth.data) return undefined
    try { return new ReportSubmission(gateway, () => crypto.randomUUID(), pendingStore(auth.data.actor.id, sessionStorage)) }
    catch { return undefined }
  }, [auth.data?.actor.id])
  if (current.error !== undefined) return <Page title="物业报修"><div className="px-4 lg:px-6"><ErrorBox error={current.error} /><p className="mt-3 text-sm text-muted-foreground">请确认后端已启用 M3，再刷新页面。</p></div></Page>
  if (current.loading || !auth.data) return <p role="status" className="p-6 text-sm text-muted-foreground">正在加载报修服务…</p>
  if (current.data?.actor.id !== auth.data.actor.id) return <Page title="物业报修"><p className="px-4 text-sm text-destructive">登录身份已变化，请刷新后继续。</p></Page>
  return <>
    {current.data.development && <div className="mx-4 mt-4 lg:mx-6"><Alert><AlertTitle>本地开发环境</AlertTitle><AlertDescription>工单保存到本地 MySQL。场地目录、报修权限映射和通知事件仍使用开发适配器；尚未向真实物业或外部渠道投递。</AlertDescription></Alert></div>}
    {page === "report" ? submission ? <ReportPage gateway={gateway} submission={submission} /> : <Page title="故障报修"><p className="px-4 text-sm text-destructive">浏览器会话存储不可用，请允许后刷新。当前不会发送新报修。</p></Page> : ["orders","dispatch","assigned","acceptance"].includes(page) ? <OrdersPage key={page} gateway={gateway} view={page==="dispatch"?"dispatch":page==="assigned"?"assigned":page==="acceptance"?"acceptance":"mine"} /> : <DetailPage gateway={gateway} actions={gateway} actorId={current.data.actor.id} />}
  </>
}
