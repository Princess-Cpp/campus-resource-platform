import { useState } from "react"
import { useNavigate } from "react-router"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Page } from "@/modules/common/page"
import { useRequest } from "@/modules/common/use-request"
import { formatDateTime } from "@/modules/common/time"
import { categories,queryDates,states } from "../reporting"
import { durationText,ratioText,routingText } from "../operations"
import { ErrorBox,selectClass } from "../components/shared"
import type {Category,OperationsGateway,Statistics} from "../types"

export function OperationsPage({gateway,view}:{gateway:OperationsGateway;view:"stats"|"escalations"}){
 const navigate=useNavigate()
 const [from,setFrom]=useState("");const [to,setTo]=useState("");const [category,setCategory]=useState<Category|"">("");const [page,setPage]=useState(1);const [active,setActive]=useState(true)
 const query=()=>({...queryDates(from,to),...(category?{category}: {})})
 const summary=useRequest(()=>view==="stats"?gateway.stats(query()):Promise.resolve(undefined),[gateway,view,from,to,category])
 const queue=useRequest(()=>view==="escalations"?gateway.escalations({...query(),page,pageSize:20,active}):Promise.resolve(undefined),[gateway,view,from,to,category,page,active])
 const error=view==="stats"?summary.error:queue.error;const loading=view==="stats"?summary.loading:queue.loading;const reload=view==="stats"?summary.reload:queue.reload
 return <Page title={view==="stats"?"报修基础统计":"超时升级与异常"} description={view==="stats"?"仅显示当前授权区域的聚合数据。按上报日期筛选工单。":"查看授权区域的阶段超时和主管通知路由；按检测日期筛选。"} actions={<Button variant="outline" onClick={reload} disabled={loading}>刷新数据</Button>}>
 <div className="space-y-4 px-4 lg:px-6">
  <div className="grid gap-3 sm:grid-cols-3"><Field><FieldLabel htmlFor="m3-ops-from">{view==="stats"?"上报开始日期":"检测开始日期"}</FieldLabel><Input id="m3-ops-from" type="date" value={from} onChange={e=>{setFrom(e.target.value);setPage(1)}}/></Field><Field><FieldLabel htmlFor="m3-ops-to">结束日期</FieldLabel><Input id="m3-ops-to" type="date" value={to} onChange={e=>{setTo(e.target.value);setPage(1)}}/></Field><Field><FieldLabel htmlFor="m3-ops-category">故障类别</FieldLabel><select id="m3-ops-category" className={selectClass} value={category} onChange={e=>{setCategory(e.target.value as Category|"");setPage(1)}}><option value="">全部类别</option>{categories.map(c=><option key={c.value} value={c.value}>{c.label}</option>)}</select></Field></div>
  {view==="escalations" && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={active} onChange={e=>{setActive(e.target.checked);setPage(1)}}/>只看仍处于原阶段的工单</label>}
  {error!==undefined?<ErrorBox error={error}/>:loading?<p role="status" className="py-8 text-sm text-muted-foreground">正在加载…</p>:view==="stats"&&summary.data?<StatisticsCards data={summary.data}/>:view==="escalations"&&queue.data?<>
   <Alert><AlertTitle>升级不等于通知已送达</AlertTitle><AlertDescription>未配置主管或目录不可用时，升级事实仍保留并继续核对。已入队仅代表保存待投递意图。主管不会因此自动取得派单或验收权限。</AlertDescription></Alert>
   {!queue.data.items.length?<p className="rounded-lg border p-6 text-sm text-muted-foreground">当前筛选下没有升级记录。</p>:queue.data.items.map(item=><Card key={item.id}><CardHeader><CardTitle className="flex flex-wrap justify-between gap-2 text-base"><span>{item.roomName} · {item.area}</span><span>{item.active?"当前阶段":"历史阶段"}</span></CardTitle></CardHeader><CardContent className="space-y-2 text-sm"><p>阶段 {item.stageNumber} · {states[item.stageState]} · 当前{states[item.currentState]}</p><p className="text-muted-foreground">期限 {formatDateTime(item.deadlineAt)} · 检测 {formatDateTime(item.detectedAt)}</p><p>{routingText(item.routingState)}</p><p className="break-all text-xs text-muted-foreground">{item.workorderId}</p>{item.canOpen?<Button variant="outline" onClick={()=>navigate(`/m3/workorders/${encodeURIComponent(item.workorderId)}`,{state:{backTo:"/m3/escalations"}})}>查看工单并协调处理</Button>:<p className="text-muted-foreground">当前权限仅允许读取升级异常元数据。</p>}</CardContent></Card>)}
   <div className="flex flex-wrap items-center justify-between gap-3 text-sm"><span>共 {queue.data.total} 条 · 第 {page} 页</span><div className="flex gap-2"><Button variant="outline" disabled={page===1} onClick={()=>setPage(page-1)}>上一页</Button><Button variant="outline" disabled={page*20>=queue.data.total} onClick={()=>setPage(page+1)}>下一页</Button></div></div>
  </>:null}
 </div></Page>
}
function StatisticsCards({data}:{data:Statistics}){
 const counts=[["工单总数",data.total],["已完成",data.byState.COMPLETED??0],["已取消（单列）",data.byState.CANCELLED??0],["资源待确认",data.resourceUnresolvedCount]]
 const times=[["平均派单耗时",data.dispatch],["平均首次维修响应",data.firstRepair],["平均累计维修耗时",data.repair],["平均累计验收处理耗时",data.acceptance]] as const
 return <>
  <p className="text-xs text-muted-foreground">数据时间：{formatDateTime(data.asOf)}</p>
  <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{counts.map(([label,value])=><Card key={label}><CardHeader><CardTitle className="text-sm text-muted-foreground">{label}</CardTitle></CardHeader><CardContent className="text-3xl font-semibold tabular-nums">{value}</CardContent></Card>)}</div>
  <div className="grid gap-3 sm:grid-cols-2">{times.map(([label,value])=><Card key={label}><CardHeader><CardTitle className="text-sm">{label}</CardTitle></CardHeader><CardContent><p className="text-2xl font-semibold tabular-nums">{durationText(value.averageSeconds)}</p><p className="mt-2 text-sm text-muted-foreground">样本 {value.count} 张工单</p></CardContent></Card>)}</div>
  <div className="grid gap-3 sm:grid-cols-2"><Card><CardHeader><CardTitle>已记录超时率</CardTitle></CardHeader><CardContent><p className="text-2xl font-semibold">{ratioText(data.recordedOverdueRate)}</p><p className="mt-2 text-sm text-muted-foreground">已有升级记录 {data.recordedOverdueCount} / 非取消工单 {data.eligibleCount}。只统计已持久化的升级，不代表所有未确认阶段都已判定。</p></CardContent></Card><Card><CardHeader><CardTitle>维修满意度</CardTitle></CardHeader><CardContent><p className="text-2xl font-semibold">{data.averageScore===null?"暂无评价":`${data.averageScore.toFixed(1)} / 5 分`}</p><p className="mt-2 text-sm text-muted-foreground">已评价 {data.ratedCount} 张，未评价不计入均分。</p></CardContent></Card></div>
  <Alert><AlertTitle>统计口径</AlertTitle><AlertDescription>取消工单单列，排除耗时、评分和超时率分母。派单与首修采用已有首次时间；累计维修、验收处理均值只取已完成工单，返工各段相加。验收处理计至通过申请，资源同步等待不计入。时限仍使用本工单保存的待评审提案。</AlertDescription></Alert>
 </>
}
