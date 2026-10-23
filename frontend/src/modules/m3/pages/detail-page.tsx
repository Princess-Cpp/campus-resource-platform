import { useEffect } from "react"
import { useLocation, useNavigate, useParams } from "react-router"
import { ArrowLeftIcon, CheckIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Page } from "@/modules/common/page"
import { formatDateTime } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { durationText } from "../operations"
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
  const backTo=["/m3/dispatch","/m3/assigned","/m3/acceptance","/m3/escalations"].includes(location.state?.backTo)?location.state.backTo:"/m3/workorders"
  return <Page title="报修详情" description={order?.id} actions={<Button variant="outline" onClick={() => navigate(backTo)}><ArrowLeftIcon />返回列表</Button>}>
    <div className="space-y-4 px-4 lg:px-6">
      {result.error !== undefined ? <div className="space-y-3"><ErrorBox error={result.error} /><Button variant="outline" onClick={result.reload}>重新加载详情</Button></div> : result.loading || !order ? <p role="status" className="py-8 text-sm text-muted-foreground">正在加载报修详情…</p> : <>
        {location.state?.created && <Alert><CheckIcon /><AlertTitle>{gateway.mode === "preview" ? "已创建示例报修" : "报修已提交"}</AlertTitle><AlertDescription>{gateway.mode === "preview" ? "这条记录只用于界面预览，未发送至物业，刷新页面后会清空。" : "可在此查看后续处理进展。"}</AlertDescription></Alert>}
        <Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><CardTitle className="text-lg">{order.roomName}</CardTitle><Status order={order} /></div></CardHeader><CardContent className="space-y-6">
          <dl className="grid gap-x-8 gap-y-4 text-sm sm:grid-cols-2 lg:grid-cols-4">{[["故障类别", categories.find(item => item.value === order.category)?.label ?? order.category], ["紧急程度", order.severity === "URGENT" ? "紧急（待复核）" : "一般"], ["上报时间", formatDateTime(order.createdAt)], ["座位", order.seatLabel ? `${order.seatLabel} 号座位` : order.seatId ? "已指定座位" : "整个教室 / 未指定"]].map(([label, value]) => <div key={label} className="space-y-1"><dt className="text-muted-foreground">{label}</dt><dd className="break-words font-medium">{value}</dd></div>)}</dl>
          {order.assigneeId && <div className="space-y-2 border-t pt-4 text-sm"><p>{order.state==="CANCELLED"?"取消前维修人员：":"当前维修人员："}<span className="font-medium">{order.assigneeName??order.assigneeId}</span></p><p className="text-muted-foreground">{order.state==="CANCELLED"?"工单已取消，无需接单":order.acceptedAt?`已接单 · ${formatDateTime(order.acceptedAt)}`:"等待维修人员接单"}</p>{order.firstAssignedAt && <p className="text-muted-foreground">首次派单：{formatDateTime(order.firstAssignedAt)}</p>}</div>}
          <div className="space-y-2 border-t pt-4"><h3 className="text-sm font-medium">故障描述</h3><p className="whitespace-pre-wrap break-words text-sm leading-7">{order.description}</p></div>
          {order.escalated && <Alert><AlertTitle>当前阶段已超时升级</AlertTitle><AlertDescription>处理已超出当前阶段期限，请联系物业主管跟进。超时不会自动结束工单或恢复教室预约。</AlertDescription></Alert>}
          {order.deadlineAt && !["COMPLETED","CANCELLED"].includes(order.state) && <p className="text-sm text-muted-foreground">当前阶段期限：{formatDateTime(order.deadlineAt)}</p>}
          {order.firstStartedAt && <p className="text-sm text-muted-foreground">首次开始维修：{formatDateTime(order.firstStartedAt)}</p>}
          {order.result && <div className="space-y-2 border-t pt-4"><h3 className="text-sm font-medium">最新维修处理结果</h3><p className="whitespace-pre-wrap break-words text-sm leading-7">{order.result}</p></div>}
          {order.syncState !== "IDLE" && <Alert><AlertTitle>{order.operation?.state==="FAILED"?"教室恢复未完成":order.syncState==="NEEDS_ATTENTION"?"教室状态需要人工核对":"正在确认教室状态"}</AlertTitle><AlertDescription>{order.operation?.state==="FAILED"?"教室仍保留维修限制，请联系物业管理人员核对后再处理。":"处理尚未完成，请稍后刷新。确认前暂不能改派、取消或进行其他处理，也不能据此判断教室已恢复预约。"}</AlertDescription></Alert>}
          {order.syncState==="IDLE" && order.lastResourceError && <Alert variant="destructive"><AlertTitle>暂时无法开始维修</AlertTitle><AlertDescription>教室状态更新未成功，本次尚未开始维修。请联系物业管理人员核对后再试。</AlertDescription></Alert>}
          {order.state==="AWAITING_ACCEPTANCE" && order.syncState==="IDLE" && <Alert><AlertTitle>等待独立验收</AlertTitle><AlertDescription>教室仍保留本工单的维修限制。参与过本工单维修的人员不能进行验收。</AlertDescription></Alert>}
          {order.state==="COMPLETED" && <Alert><CheckIcon/><AlertTitle>工单已完成</AlertTitle><AlertDescription>本工单的维修限制已解除。教室是否可以预约，还取决于其他维修、活动占用、停用和开放时间。{order.completedAt && <p>完成时间：{formatDateTime(order.completedAt)}</p>}</AlertDescription></Alert>}
        </CardContent></Card>
        {order.state==="COMPLETED" && <Card><CardHeader><CardTitle>维修评价</CardTitle></CardHeader><CardContent className="space-y-3 text-sm">
          <p className="text-muted-foreground">累计维修处理：{durationText(order.repairSeconds??null)} · 累计验收处理：{durationText(order.acceptanceSeconds??null)}</p>
          {order.evaluation?<><p className="font-medium">已评价 · {order.evaluation.score} / 5 分</p>{order.evaluation.comment && <p className="whitespace-pre-wrap break-words">{order.evaluation.comment}</p>}<p className="text-muted-foreground">{formatDateTime(order.evaluation.at)} · 评价提交后不修改</p></>:<p className="text-muted-foreground">尚未评价{order.evaluationDeadlineAt?` · 截止 ${formatDateTime(order.evaluationDeadlineAt)}`:""}。仅报修人可在评价期内提交一次。</p>}
        </CardContent></Card>}
        {actions && actorId && <OrderActions gateway={actions} actorId={actorId} order={order} changed={result.reload}/>}
        <Button variant="outline" onClick={result.reload}>刷新详情</Button>
        <Card><CardHeader><CardTitle>处理记录</CardTitle></CardHeader><CardContent><ol className="space-y-0">{order.timeline.map((entry, index) => <li key={entry.id} className={`relative ml-1 border-l pl-6 ${index < order.timeline.length - 1 ? "pb-6" : "pb-1"}`}><span className="absolute -left-1 top-1 size-2 rounded-full bg-foreground" /><p className="break-words text-sm leading-6">{entry.text}</p>{entry.comment && <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">{entry.comment}</p>}<time className="text-xs text-muted-foreground" dateTime={entry.at}>{formatDateTime(entry.at)}</time></li>)}</ol></CardContent></Card>
      </>}
    </div>
  </Page>
}
