import { useState } from "react"
import { Link } from "react-router"
import { BellIcon, CheckIcon, RefreshCwIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Page } from "@/modules/common/page"
import { formatDateTime } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { ErrorBox } from "../components/shared"
import type { NotificationsGateway, WorkorderNotification } from "../types"

export function NotificationsPage({gateway}:{gateway:NotificationsGateway}){
 const [page,setPage]=useState(1),[unread,setUnread]=useState(false)
 const [saving,setSaving]=useState<string>(),[error,setError]=useState<unknown>()
 const data=useRequest(()=>gateway.notifications({page,pageSize:20,unread}),[gateway,page,unread])
 async function markRead(n:WorkorderNotification){
  setSaving(n.eventId+":"+n.kind);setError(undefined)
  try{await gateway.readNotification(n.eventId,n.kind);data.reload()}catch(e){setError(e)}finally{setSaving(undefined)}
 }
 return <Page title="报修消息" description="查看报修进展和处理提醒。">
  <div className="flex flex-col gap-4 px-4 lg:px-6">
   <div className="flex flex-wrap items-center justify-between gap-3">
    <div className="flex items-center gap-2"><BellIcon className="size-4"/><span className="text-sm">未读 {data.data?.unreadCount??"—"} 条</span></div>
    <div className="flex gap-2"><Button variant={unread?"secondary":"outline"} onClick={()=>{setUnread(!unread);setPage(1)}}>{unread?"显示全部":"只看未读"}</Button><Button variant="outline" onClick={data.reload} disabled={data.loading}><RefreshCwIcon data-icon="inline-start"/>刷新</Button></div>
   </div>
   {error!==undefined&&<ErrorBox error={error}/>}
   {data.error!==undefined?<ErrorBox error={data.error}/>:data.loading?<p role="status" className="text-sm text-muted-foreground">正在加载消息…</p>:data.data&&<>
    {data.data.items.length===0?<Card><CardContent className="py-8 text-center text-sm text-muted-foreground">{unread?"暂无未读报修消息":"暂无报修消息"}</CardContent></Card>:data.data.items.map(n=><Card key={n.eventId+":"+n.kind}>
     <CardHeader><CardTitle className="flex flex-wrap items-center justify-between gap-2 text-base"><span>{n.summary}</span><Badge variant={n.readAt?"secondary":"default"}>{n.readAt?"已读":"未读"}</Badge></CardTitle></CardHeader>
     <CardContent className="flex flex-col gap-3"><p className="text-xs text-muted-foreground">{formatDateTime(n.createdAt)}</p>
      <div className="flex flex-wrap gap-2">{n.canOpen?<Button variant="outline" render={<Link to={"/m3/workorders/"+encodeURIComponent(n.workorderId)}/>}>查看工单</Button>:<span className="self-center text-xs text-muted-foreground">当前权限不允许打开该工单。</span>}{!n.readAt&&<Button variant="outline" disabled={saving!==undefined} onClick={()=>markRead(n)}><CheckIcon data-icon="inline-start"/>{saving===n.eventId+":"+n.kind?"正在标记…":"标为已读"}</Button>}</div>
     </CardContent>
    </Card>)}
    <div className="flex items-center justify-between gap-3 text-sm"><span>共 {data.data.total} 条 · 第 {page} 页</span><div className="flex gap-2"><Button variant="outline" disabled={page===1} onClick={()=>setPage(page-1)}>上一页</Button><Button variant="outline" disabled={page*20>=data.data.total} onClick={()=>setPage(page+1)}>下一页</Button></div></div>
   </>}
  </div>
 </Page>
}
