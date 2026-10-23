import { useState } from "react"
import { useNavigate } from "react-router"
import { ClipboardListIcon, PlusIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Page } from "@/modules/common/page"
import { formatDateTime } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { categories, queryDates, states } from "../reporting"
import { ErrorBox, selectClass, Status } from "../components/shared"
import type { Category, ListView, MaintenanceGateway, OrderState } from "../types"

export function OrdersPage({ gateway,view="mine",canReport=true }: { gateway: MaintenanceGateway;view?:ListView;canReport?:boolean }) {
  const navigate = useNavigate()
  const [state, setState] = useState<OrderState | "">("")
  const [category, setCategory] = useState<Category | "">("")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [fromDate, setFromDate] = useState("")
  const [toDate, setToDate] = useState("")
  const filtered = !!(state || category || fromDate || toDate)
  const clearFilters = () => { setState(""); setCategory(""); setFromDate(""); setToDate(""); setPage(1) }
  const result = useRequest(async () => gateway.list({view, page, pageSize, ...(state ? { state } : {}), ...(category ? { category } : {}), ...queryDates(fromDate, toDate) }), [gateway,view, page, pageSize, state, category, fromDate, toDate])
  const pageCount = Math.max(1, Math.ceil((result.data?.total ?? 0) / pageSize))
  return <Page title={view==="dispatch"?"区域调度台":view==="assigned"?"维修待办":view==="acceptance"?"验收工作台":"我的报修"} description={view==="dispatch"?"查看授权区域的工单并安排维修人员。":view==="assigned"?"查看当前分配给自己的工单，接单并处理维修。":view==="acceptance"?"查看授权区域的待验收工单。历史维修参与人不能验收自己的维修。":"查看自己上报的故障和处理进展。"} actions={view==="mine"&&canReport?<Button onClick={() => navigate("/m3/report")}><PlusIcon />故障报修</Button>:undefined}>
    <div className="space-y-4 px-4 lg:px-6">
      <div className="grid items-end gap-3 sm:grid-cols-2 xl:grid-cols-[repeat(4,minmax(0,1fr))_auto]">
        <Field><FieldLabel htmlFor="m3-filter-state">工单状态</FieldLabel><select id="m3-filter-state" className={selectClass} value={state} onChange={e => { setState(e.target.value as OrderState | ""); setPage(1) }}><option value="">全部状态</option>{Object.entries(states).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field>
        <Field><FieldLabel htmlFor="m3-filter-category">故障类别</FieldLabel><select id="m3-filter-category" className={selectClass} value={category} onChange={e => { setCategory(e.target.value as Category | ""); setPage(1) }}><option value="">全部类别</option>{categories.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</select></Field>
        <Field><FieldLabel htmlFor="m3-filter-from">上报开始日期</FieldLabel><Input id="m3-filter-from" type="date" value={fromDate} onChange={e => { setFromDate(e.target.value); setPage(1) }} /></Field>
        <Field><FieldLabel htmlFor="m3-filter-to">上报结束日期</FieldLabel><Input id="m3-filter-to" type="date" value={toDate} onChange={e => { setToDate(e.target.value); setPage(1) }} /></Field>
        <Button variant="outline" disabled={result.loading} onClick={result.reload}>刷新列表</Button>
      </div>
      {result.error !== undefined ? <ErrorBox error={result.error} /> : result.loading ? <p role="status" className="py-8 text-sm text-muted-foreground">正在加载报修记录…</p> : !result.data?.items.length ? <Empty className="min-h-64 border"><EmptyHeader><EmptyMedia variant="icon"><ClipboardListIcon /></EmptyMedia><EmptyTitle>{filtered ? "没有符合筛选条件的报修" : view==="mine"?"还没有报修记录":view==="dispatch"?"授权区域暂无工单":view==="acceptance"?"当前没有可独立验收的工单":"当前没有维修待办"}</EmptyTitle><EmptyDescription>{filtered ? "调整状态、类别或日期查看其他记录。" : view==="mine"?(canReport?"遇到教室设施问题时，可以在这里上报。":"你上报过的工单会显示在这里。"):view==="dispatch"?"新报修到达后会显示在这里。":view==="acceptance"?"需要你独立验收的工单会显示在这里。":"分派给你的工单会显示在这里。"}</EmptyDescription></EmptyHeader><Button variant="outline" onClick={() => filtered ? clearFilters() : view==="mine"&&canReport?navigate("/m3/report"):result.reload()}>{filtered ? "清除筛选" : view==="mine"&&canReport?"填写报修":"刷新列表"}</Button></Empty> : <div className="space-y-3" aria-label="报修记录">{result.data.items.map(order => <Card key={order.id} className="min-w-0 py-0"><CardContent className="flex min-w-0 flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-2"><div className="flex flex-wrap items-center gap-2"><h3 className="font-medium">{order.roomName}</h3><Status order={order} />{order.acceptedAt && <span className="text-xs text-muted-foreground">已接单</span>}{order.severity === "URGENT" && <span className="text-xs text-amber-700 dark:text-amber-400">上报为紧急</span>}</div><p className="line-clamp-2 break-words text-sm text-muted-foreground">{order.description}</p><p className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground"><span>{categories.find(item => item.value === order.category)?.label}</span><time dateTime={order.createdAt}>{formatDateTime(order.createdAt)}</time><span className="break-all">{order.id}</span></p></div>
        <Button variant="outline" className="self-start" onClick={() => navigate(`/m3/workorders/${encodeURIComponent(order.id)}`,{state:{backTo:view==="dispatch"?"/m3/dispatch":view==="assigned"?"/m3/assigned":view==="acceptance"?"/m3/acceptance":"/m3/workorders"}})}>查看详情</Button>
      </CardContent></Card>)}</div>}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-3 text-sm text-muted-foreground"><div className="flex flex-wrap items-center gap-2"><span>共 {result.error === undefined ? result.data?.total ?? 0 : "—"} 条</span><label htmlFor="m3-page-size" className="sr-only">每页条数</label><select id="m3-page-size" className={`${selectClass} w-auto`} value={pageSize} onChange={e => { setPageSize(Number(e.target.value)); setPage(1) }}>{[5, 10, 20].map(size => <option key={size} value={size}>每页 {size} 条</option>)}</select></div><div className="flex items-center gap-2"><Button variant="outline" disabled={page <= 1 || result.loading || result.error !== undefined} onClick={() => setPage(page - 1)}>上一页</Button><span>{result.error === undefined ? `${page} / ${pageCount}` : "—"}</span><Button variant="outline" disabled={page >= pageCount || result.loading || result.error !== undefined} onClick={() => setPage(page + 1)}>下一页</Button></div></div>
    </div>
  </Page>
}
