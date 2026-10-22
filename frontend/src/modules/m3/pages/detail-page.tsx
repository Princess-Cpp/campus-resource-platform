import { useLocation, useNavigate, useParams } from "react-router"
import { ArrowLeftIcon, CheckIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Page } from "@/modules/common/page"
import { formatDateTime } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { categories } from "../reporting"
import { ErrorBox, Status } from "../components/shared"
import type { MaintenanceGateway } from "../types"

export function DetailPage({ gateway }: { gateway: MaintenanceGateway }) {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const result = useRequest(() => gateway.detail(id), [gateway, id])
  const order = result.data
  return <Page title="报修详情" description={order?.id} actions={<Button variant="outline" onClick={() => navigate("/m3/workorders")}><ArrowLeftIcon />返回列表</Button>}>
    <div className="space-y-4 px-4 lg:px-6">
      {result.error !== undefined ? <div className="space-y-3"><ErrorBox error={result.error} /><Button variant="outline" onClick={result.reload}>重新加载详情</Button></div> : result.loading || !order ? <p role="status" className="py-8 text-sm text-muted-foreground">正在加载报修详情…</p> : <>
        {location.state?.created && <Alert><CheckIcon /><AlertTitle>{gateway.mode === "preview" ? "已创建示例报修" : "报修已提交"}</AlertTitle><AlertDescription>{gateway.mode === "preview" ? "这条记录只用于界面预览，未发送至物业，刷新页面后会清空。" : "可在此查看后续处理进展。"}</AlertDescription></Alert>}
        <Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-lg">{order.roomName}</CardTitle><Status order={order} /></div></CardHeader><CardContent className="space-y-6">
          <dl className="grid gap-x-8 gap-y-4 text-sm sm:grid-cols-2 lg:grid-cols-4">{[["故障类别", categories.find(item => item.value === order.category)?.label ?? order.category], ["紧急程度", order.severity === "URGENT" ? "紧急（待复核）" : "一般"], ["上报时间", formatDateTime(order.createdAt)], ["座位", order.seatLabel ? `${order.seatLabel} 号座位` : order.seatId ? "已指定座位" : "整个教室 / 未指定"]].map(([label, value]) => <div key={label} className="space-y-1"><dt className="text-muted-foreground">{label}</dt><dd className="break-words font-medium">{value}</dd></div>)}</dl>
          <div className="space-y-2 border-t pt-4"><h3 className="text-sm font-medium">故障描述</h3><p className="whitespace-pre-wrap break-words text-sm leading-7">{order.description}</p></div>
          {order.syncState !== "IDLE" && <Alert><AlertTitle>正在核对资源状态</AlertTitle><AlertDescription>处理结果尚未确认，请稍后刷新。当前状态不代表教室已恢复可预约。</AlertDescription></Alert>}
        </CardContent></Card>
        <Card><CardHeader><CardTitle>处理记录</CardTitle></CardHeader><CardContent><ol className="space-y-0">{order.timeline.map((entry, index) => <li key={entry.id} className={`relative ml-1 border-l pl-6 ${index < order.timeline.length - 1 ? "pb-6" : "pb-1"}`}><span className="absolute -left-1 top-1 size-2 rounded-full bg-foreground" /><p className="break-words text-sm leading-6">{entry.text}</p><time className="text-xs text-muted-foreground" dateTime={entry.at}>{formatDateTime(entry.at)}</time></li>)}</ol></CardContent></Card>
      </>}
    </div>
  </Page>
}
