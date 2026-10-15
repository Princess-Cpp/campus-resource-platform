import { useEffect, useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import { ApiError, errorMessage } from "@/modules/common/api"
import { formatDateTime, formatRange } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { act, getApplication, retry, type Actor, type Application } from "../api"
import { ErrorBox, Status, statuses } from "./shared"
const actions: Record<string, string> = { SUBMIT: "提交申请", APPROVE: "通过审批", REJECT: "驳回申请", CANCEL: "撤销申请", TERMINATE: "终止活动", RETRY: "重试同步", EXPIRE: "到期撤销", RESOURCE_CONFIRMED: "资源确认成功", RESOURCE_FAILED: "资源确认失败" }
export function ApplicationDetail({ id, actor, onClose, onChange }: { id: string; actor?: Actor; onClose: () => void; onChange: () => void }) {
  const detail = useRequest(() => getApplication(id), [id])
  const b = detail.data
  const [comment, setComment] = useState("")
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>()
  const [pending, setPending] = useState<{ key: string; action: string; expectedVersion: number; comment: string }>()
  useEffect(() => { if (!b || b.syncStatus === "IDLE" || b.syncStatus === "FAILED") return; const timer = window.setInterval(detail.reload, 3000); return () => clearInterval(timer) }, [b?.syncStatus, detail.reload])
  const run = async (action: string) => {
    if (!b) return
    setBusy(true); setError(undefined)
    try {
      let result: Application
      if (action === "RETRY") result = await retry(b.id, b.version)
      else { const command = pending ?? { key: crypto.randomUUID(), action, expectedVersion: b.version, comment: comment.trim() }; setPending(command); result = await act(b.id, { action: command.action, expectedVersion: command.expectedVersion, comment: command.comment }, command.key) }
      setPending(undefined); setComment(""); detail.reload(); onChange(); toast.success(result.syncStatus === "IDLE" ? "操作已完成" : "操作已接收，正在确认场地状态")
    } catch (e) { setError(e); if (e instanceof ApiError && e.status >= 400 && e.status < 500) { setPending(undefined); detail.reload(); onChange() } }
    finally { setBusy(false) }
  }
  const now = Date.now()
  const finalRole = !!actor?.permissions.includes("approval.final") && !!actor?.areas.some(a => a === "*" || a === b?.area)
  const canApprove = b && actor && b.userId !== actor.id && now < Date.parse(b.deadlineAt) && ((b.status === "SUBMITTED" && actor.permissions.includes("approval.department") && actor.department === b.department && b.details.departmentApprovers.includes(actor.id)) || (b.status === "DEPARTMENT_APPROVED" && finalRole && b.details.finalApprovers.includes(actor.id) && b.details.departmentActor !== actor.id))
  const canCancel = b && actor?.id === b.userId && ["SUBMITTED", "DEPARTMENT_APPROVED", "APPROVED"].includes(b.status) && now < Date.parse(b.startAt)
  const canTerminate = b && finalRole && actor?.permissions.includes("classroom.terminate") && b.status === "APPROVED" && now >= Date.parse(b.startAt) && now < Date.parse(b.endAt)
  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose() }}><DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>申请详情</DialogTitle><DialogDescription className="break-all">申请编号 {id}</DialogDescription></DialogHeader>
    {detail.error !== undefined && <ErrorBox error={detail.error} />}
    {error !== undefined && <ErrorBox error={error} />}
    {detail.loading && !b && <p className="text-sm text-muted-foreground">正在加载申请…</p>}
    {b && <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-medium">{b.roomName}</h3><Status value={b} /></div>
      <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">{[["使用时段", formatRange(b.startAt, b.endAt)], ["审批截止", formatDateTime(b.deadlineAt)], ["区域 / 院系", `${b.area} / ${b.department}`], ["人数 / 容量", `${b.details.attendees} / ${b.details.capacity}`], ["提交时间", formatDateTime(b.createdAt)], ["当前版本", String(b.version)]].map(([label, value]) => <div key={label}><dt className="text-muted-foreground">{label}</dt><dd className="mt-1">{value}</dd></div>)}</dl>
      <div className="space-y-1 text-sm"><p className="text-muted-foreground">活动用途</p><p className="whitespace-pre-wrap break-words">{b.details.purpose}</p></div>
      {b.lastError && <p className="text-sm text-destructive">最近同步结果：{errorMessage(b.lastError)}</p>}
      {b.syncStatus !== "IDLE" && <div className="space-y-2 rounded-lg border p-3 text-sm"><p>场地结果尚未确认，期间不能执行相反操作。请刷新查看进度。</p>{b.operationId && <p className="break-all text-xs text-muted-foreground">操作编号：{b.operationId}</p>}{["UNKNOWN", "FAILED"].includes(b.syncStatus) && <Button variant="outline" disabled={busy} onClick={() => run("RETRY")}>重试同一操作</Button>}</div>}
      {pending ? <div className="space-y-2 rounded-lg border p-3 text-sm"><p>上次请求结果未知，请核对并重试原操作。</p><Button disabled={busy} onClick={() => run(pending.action)}>核对并重试</Button></div> : b.syncStatus === "IDLE" && (canApprove || canCancel || canTerminate) && <div className="space-y-3">
        {(canApprove || canTerminate) && <Field><FieldLabel htmlFor="m2-comment">处理意见（驳回或终止必填）</FieldLabel><Textarea id="m2-comment" maxLength={500} value={comment} onChange={e => setComment(e.target.value)} /></Field>}
        <div className="flex flex-wrap gap-2">{canApprove && <><Button disabled={busy} onClick={() => run("APPROVE")}>{b.status === "SUBMITTED" ? "通过院系审批" : "批准并确认场地"}</Button><Button variant="destructive" disabled={busy || !comment.trim()} onClick={() => run("REJECT")}>驳回申请</Button></>}{canCancel && <Button variant="outline" disabled={busy} onClick={() => run("CANCEL")}>确认撤销申请</Button>}{canTerminate && <Button variant="destructive" disabled={busy || !comment.trim()} onClick={() => run("TERMINATE")}>确认终止活动</Button>}</div>
      </div>}
      <Separator /><h4 className="text-sm font-medium">审批与处理记录</h4><ol className="space-y-3">{b.events?.map(ev => <li key={ev.id} className="rounded-lg border p-3 text-sm"><div className="flex flex-wrap justify-between gap-2"><span className="font-medium">{actions[ev.action] ?? ev.action}</span><time className="text-xs text-muted-foreground">{formatDateTime(ev.createdAt)}</time></div><p className="mt-1 text-xs text-muted-foreground">{ev.actorId} · {ev.fromStatus ? `${statuses[ev.fromStatus] ?? ev.fromStatus} → ` : ""}{statuses[ev.toStatus] ?? ev.toStatus}</p>{ev.comment && <p className="mt-2 whitespace-pre-wrap break-words">{ev.comment}</p>}</li>)}</ol>
      <div className="flex justify-end"><Button variant="outline" disabled={detail.loading} onClick={detail.reload}>刷新状态</Button></div>
    </div>}
  </DialogContent></Dialog>
}
