import { useMemo, useState } from "react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import { useRequest } from "@/modules/common/use-request"
import { ActionSubmission, actionStore } from "../action-submission"
import type { ActionGateway, ActionName, OrderDetail } from "../types"
import { ErrorBox, selectClass } from "./shared"

export function OrderActions({ gateway, actorId, order, changed }: { gateway:ActionGateway;actorId:string;order:OrderDetail;changed:()=>void }) {
 const controller=useMemo(()=>{
  try{return new ActionSubmission(gateway,order.id,actionStore(actorId,order.id,sessionStorage))}catch{return undefined}
 },[gateway,actorId,order.id])
 const [comment,setComment]=useState("")
 const [assignee,setAssignee]=useState("")
 const [busy,setBusy]=useState(false)
 const [error,setError]=useState<unknown>()
 const allowed=order.allowedActions??[]
 const canAssign=allowed.includes("ASSIGN")
 const workers=useRequest(()=>canAssign?gateway.assignees(order.id):Promise.resolve([]),[gateway,order.id,order.version,canAssign])
 const eligible=workers.data?.filter(w=>w.id!==order.assigneeId)??[]
 const pending=controller?.draft
 async function send(action:ActionName) {
  if(!controller || busy)return
  setBusy(true);setError(undefined)
  try{await controller.send(pending??{action,expectedVersion:order.version,...(action==="ASSIGN"?{assigneeId:assignee}:{}),comment});changed()}
  catch(e){setError(e)}finally{setBusy(false)}
 }
 if(!allowed.length && !pending && !controller?.problem)return null
 return <Card><CardHeader><CardTitle>处理工单</CardTitle></CardHeader><CardContent className="space-y-4">
  {!controller ? <ErrorBox error={new Error("浏览器会话存储不可用，请允许后刷新；当前不会发送处理请求")} /> : controller.problem ? <ErrorBox error={controller.problem} /> : <>
   {error!==undefined && <div className="space-y-2"><ErrorBox error={error}/><Button variant="outline" onClick={changed}>刷新工单</Button></div>}
   {pending ? <Alert><AlertTitle>上次操作结果尚待核对</AlertTitle><AlertDescription>已保留原操作、目标与版本，请重试原操作。核对期间不能改派给其他人或换成另一动作。</AlertDescription><Button disabled={busy} className="mt-3" onClick={()=>send(pending.action)}>{busy?"正在核对…":"重试原操作"}</Button></Alert> : <>
    {canAssign && <Field><FieldLabel htmlFor="m3-assignee">维修人员</FieldLabel><select id="m3-assignee" className={selectClass} value={assignee} disabled={busy||workers.loading||!!workers.error} onChange={e=>setAssignee(e.target.value)}><option value="">请选择维修人员</option>{eligible.map(w=><option key={w.id} value={w.id}>{w.name}</option>)}</select>
     {workers.loading?<p className="text-sm text-muted-foreground">正在核对可派人员…</p>:workers.error!==undefined?<><ErrorBox error={workers.error}/><Button variant="outline" onClick={workers.reload}>重新加载人员</Button></>:!eligible.length?<p className="text-sm text-muted-foreground">当前没有其他可派维修人员，请联系物业主管协调。工单状态保持不变。</p>:null}
    </Field>}
    {(canAssign||allowed.includes("CANCEL")) && <Field><FieldLabel htmlFor="m3-action-comment">{canAssign?"处理建议（必填）":"取消说明（选填）"}</FieldLabel><Textarea id="m3-action-comment" value={comment} disabled={busy} onChange={e=>setComment(e.target.value)} placeholder={canAssign?"说明故障排查和处理建议":"可补充取消原因"}/><p className="text-xs text-muted-foreground">最多 1000 字。</p></Field>}
    <div className="flex flex-wrap gap-2">
     {canAssign && <Button disabled={busy||workers.loading||!!workers.error||!assignee||!comment.trim()} onClick={()=>send("ASSIGN")}>{busy?"正在提交…":order.assigneeId?"确认重派":"确认派单"}</Button>}
     {allowed.includes("ACCEPT_ASSIGNMENT") && <Button disabled={busy} onClick={()=>send("ACCEPT_ASSIGNMENT")}>{busy?"正在提交…":"确认接单"}</Button>}
     {allowed.includes("CANCEL") && <Button variant="outline" disabled={busy} onClick={()=>send("CANCEL")}>取消报修</Button>}
    </div>
    {allowed.includes("ACCEPT_ASSIGNMENT") && <p className="text-sm text-muted-foreground">接单后等待开始维修；本轮尚未接入维修下架。</p>}
   </>}
  </>}
 </CardContent></Card>
}
