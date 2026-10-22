import { useEffect } from "react"
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
import { OrderActions } from "../components/order-actions"
import type { ActionGateway,MaintenanceGateway } from "../types"

export function DetailPage({ gateway,actions,actorId }: { gateway: MaintenanceGateway;actions?:ActionGateway;actorId?:string }) {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const result = useRequest(() => gateway.detail(id), [gateway, id])
  const order = result.data
  useEffect(()=>{
    if(!order || order.operation?.state!=="PENDING" || result.error!==undefined)return
    const timer=window.setInterval(result.reload,2500)
    return ()=>window.clearInterval(timer)
  },[order?.id,order?.operation?.state,result.reload,result.error])
  const backTo=["/m3/dispatch","/m3/assigned","/m3/acceptance"].includes(location.state?.backTo)?location.state.backTo:"/m3/workorders"
  return <Page title="报修详情" description={order?.id} actions={<Button variant="outline" onClick={() => navigate(backTo)}><ArrowLeftIcon />返回列表</Button>}>
    <div className="space-y-4 px-4 lg:px-6">
      {result.error !== undefined ? <div className="space-y-3"><ErrorBox error={result.error} /><Button variant="outline" onClick={result.reload}>重新加载详情</Button></div> : result.loading || !order ? <p role="status" className="py-8 text-sm text-muted-foreground">正在加载报修详情…</p> : <>
        {location.state?.created && <Alert><CheckIcon /><AlertTitle>{gateway.mode === "preview" ? "已创建示例报修" : "报修已提交"}</AlertTitle><AlertDescription>{gateway.mode === "preview" ? "这条记录只用于界面预览，未发送至物业，刷新页面后会清空。" : "可在此查看后续处理进展。"}</AlertDescription></Alert>}
        <Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-lg">{order.roomName}</CardTitle><Status order={order} /></div></CardHeader><CardContent className="space-y-6">
          <dl className="grid gap-x-8 gap-y-4 text-sm sm:grid-cols-2 lg:grid-cols-4">{[["故障类别", categories.find(item => item.value === order.category)?.label ?? order.category], ["紧急程度", order.severity === "URGENT" ? "紧急（待复核）" : "一般"], ["上报时间", formatDateTime(order.createdAt)], ["座位", order.seatLabel ? `${order.seatLabel} 号座位` : order.seatId ? "已指定座位" : "整个教室 / 未指定"]].map(([label, value]) => <div key={label} className="space-y-1"><dt className="text-muted-foreground">{label}</dt><dd className="break-words font-medium">{value}</dd></div>)}</dl>
          {order.assigneeId && <div className="space-y-2 border-t pt-4 text-sm"><p>{order.state==="CANCELLED"?"取消前维修人员：":"当前维修人员："}<span className="font-medium">{order.assigneeName??order.assigneeId}</span></p><p className="text-muted-foreground">{order.state==="CANCELLED"?"工单已取消，无需接单":order.acceptedAt?`已接单 · ${formatDateTime(order.acceptedAt)}`:"等待维修人员接单"}</p>{order.firstAssignedAt && <p className="text-muted-foreground">首次派单：{formatDateTime(order.firstAssignedAt)}</p>}</div>}
          <div className="space-y-2 border-t pt-4"><h3 className="text-sm font-medium">故障描述</h3><p className="whitespace-pre-wrap break-words text-sm leading-7">{order.description}</p></div>
          {order.firstStartedAt && <p className="text-sm text-muted-foreground">首次开始维修：{formatDateTime(order.firstStartedAt)}</p>}
          {order.result && <div className="space-y-2 border-t pt-4"><h3 className="text-sm font-medium">最新维修处理结果</h3><p className="whitespace-pre-wrap break-words text-sm leading-7">{order.result}</p></div>}
          {order.syncState !== "IDLE" && <Alert><AlertTitle>{order.operation?.state==="FAILED"?"资源操作明确失败，需要协调处理":order.syncState==="NEEDS_ATTENTION"?"自动核对已暂停，需要继续处理":"正在核对资源状态"}</AlertTitle><AlertDescription><p>{order.operation?.state==="FAILED"?"资源接口已明确拒绝本次恢复。工单保持待验收和维修限制，请联系资源接口负责人员核对错误原因。":"处理结果尚未确认。当前状态不代表教室已恢复可预约；核对期间暂不能改派、取消或改做其他动作。"}</p>{order.operation && <><p className="break-all">操作：{order.operation.id} · 已尝试 {order.operation.attempts} 次</p>{order.operation.nextAttempt && <p>下次核对：{formatDateTime(order.operation.nextAttempt)}</p>}</>}{order.lastResourceError && <p>资源错误：{order.lastResourceError}</p>}</AlertDescription></Alert>}
          {order.syncState==="IDLE" && order.lastResourceError && <Alert variant="destructive"><AlertTitle>上次资源操作未成功</AlertTitle><AlertDescription>{order.lastResourceError}。本次未确认开始维修，请核对资源情况后处理。</AlertDescription></Alert>}
          {order.state==="AWAITING_ACCEPTANCE" && order.syncState==="IDLE" && <Alert><AlertTitle>等待独立验收</AlertTitle><AlertDescription>教室仍保留本工单的维修限制。参与过本工单维修的人员不能进行验收。</AlertDescription></Alert>}
          {order.state==="COMPLETED" && <Alert><CheckIcon/><AlertTitle>工单已完成</AlertTitle><AlertDescription>已核对解除本工单来源的回执。教室是否可以预约，还取决于其他维修、活动占用、停用和开放规则。{order.completedAt && <p>完成时间：{formatDateTime(order.completedAt)}</p>}</AlertDescription></Alert>}
        </CardContent></Card>
        {actions && actorId && <OrderActions gateway={actions} actorId={actorId} order={order} changed={result.reload}/>}
        <Button variant="outline" onClick={result.reload}>刷新详情</Button>
        <Card><CardHeader><CardTitle>处理记录</CardTitle></CardHeader><CardContent><ol className="space-y-0">{order.timeline.map((entry, index) => <li key={entry.id} className={`relative ml-1 border-l pl-6 ${index < order.timeline.length - 1 ? "pb-6" : "pb-1"}`}><span className="absolute -left-1 top-1 size-2 rounded-full bg-foreground" /><p className="break-words text-sm leading-6">{entry.text}</p>{entry.comment && <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">{entry.comment}</p>}<time className="text-xs text-muted-foreground" dateTime={entry.at}>{formatDateTime(entry.at)}</time></li>)}</ol></CardContent></Card>
      </>}
    </div>
  </Page>
}
