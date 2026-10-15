import { useState } from "react"
import { Link } from "react-router"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Field, FieldLabel } from "@/components/ui/field"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Empty, EmptyHeader, EmptyTitle, EmptyDescription } from "@/components/ui/empty"
import { Page } from "@/modules/common/page"
import { useRequest } from "@/modules/common/use-request"
import { formatRange, formatDateTime } from "@/modules/common/time"
import { getSession, listApplications } from "../api"
import { Choice, ErrorBox, Status, statuses } from "../components/shared"
import { ApplicationDetail } from "../components/application-detail"
import { canDeleteApplication, DeleteApplicationButton } from "../components/delete-application-button"
import { canWithdrawApplication, WithdrawApplicationButton } from "../components/withdraw-application-button"
export function ApplicationsPage({ inbox = false }: { inbox?: boolean }) {
  const [page, setPage] = useState(1), [status, setStatus] = useState("all"), [from, setFrom] = useState(""), [to, setTo] = useState(""), [selected, setSelected] = useState<string>()
  const session = useRequest(getSession, [])
  const result = useRequest(() => listApplications(inbox, { page: String(page), pageSize: "20", status: status === "all" ? "" : status, from, to }), [inbox, page, status, from, to])
  return <Page title={inbox ? "审批待办" : "我的申请"} description={inbox ? "仅显示当前步骤与你的审批权限、管辖范围相符的申请。最终批准需确认场地占用。" : "查看本人申请和审批轨迹。场地确认期间不能撤销，确认后可按规则继续处理。"} actions={<><Button variant="outline" size="sm" disabled={result.loading} onClick={result.reload}>刷新</Button>{!inbox && <Button size="sm" render={<Link to="/m2/apply" />}>申请教室</Button>}</>}>
    <div className="space-y-4 px-4 lg:px-6">
      <Card><CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-3"><Field><FieldLabel>申请状态</FieldLabel><Choice label="申请状态" value={status} onChange={v => { setStatus(v); setPage(1) }} options={[{ value: "all", label: "全部状态" }, ...Object.entries(statuses).filter(([value]) => !inbox || ["SUBMITTED", "DEPARTMENT_APPROVED"].includes(value)).map(([value, label]) => ({ value, label }))]} /></Field><Field><FieldLabel htmlFor="m2-from">使用日期从</FieldLabel><Input id="m2-from" type="date" value={from} onChange={e => { setFrom(e.target.value); setPage(1) }} /></Field><Field><FieldLabel htmlFor="m2-to">使用日期至</FieldLabel><Input id="m2-to" type="date" value={to} onChange={e => { setTo(e.target.value); setPage(1) }} /></Field></CardContent></Card>
      {result.error !== undefined ? <ErrorBox error={result.error} /> : result.loading ? <p className="text-sm text-muted-foreground">正在加载申请…</p> : !result.data?.items.length ? <Empty className="border"><EmptyHeader><EmptyTitle>{inbox ? "暂无待处理申请" : "暂无符合条件的申请"}</EmptyTitle><EmptyDescription>{inbox ? "申请完成上一步审批后，才会进入下一步审批人的待办。" : "提交申请后可在这里查看进度。"}</EmptyDescription></EmptyHeader></Empty> : <Card className="py-0"><CardContent className="px-0"><Table><TableHeader><TableRow><TableHead className="pl-4">教室 / 活动</TableHead><TableHead>使用时段</TableHead><TableHead>人数</TableHead><TableHead>状态</TableHead><TableHead>审批截止</TableHead><TableHead className="pr-4 text-right">操作</TableHead></TableRow></TableHeader><TableBody>{result.data.items.map(b => <TableRow key={b.id}><TableCell className="pl-4"><p className="font-medium">{b.roomName}</p><p className="max-w-48 truncate text-xs text-muted-foreground">{b.details.purpose}</p></TableCell><TableCell>{formatRange(b.startAt, b.endAt)}</TableCell><TableCell>{b.details.attendees}</TableCell><TableCell><Status value={b} /></TableCell><TableCell>{formatDateTime(b.deadlineAt)}</TableCell><TableCell className="pr-4"><div className="flex justify-end gap-2"><Button variant="outline" size="sm" onClick={() => setSelected(b.id)}>{inbox ? "查看并处理" : "查看详情"}</Button>{!inbox && canWithdrawApplication(b) && <WithdrawApplicationButton application={b} onDone={result.reload} />}{!inbox && canDeleteApplication(b) && <DeleteApplicationButton application={b} onDone={result.reload} />}</div></TableCell></TableRow>)}</TableBody></Table></CardContent></Card>}
      {result.data && <div className="flex flex-wrap items-center justify-between gap-3 text-sm"><span className="text-muted-foreground">共 {result.data.total} 条 · 第 {page} 页</span><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page === 1 || result.loading} onClick={() => setPage(p => p - 1)}>上一页</Button><Button variant="outline" size="sm" disabled={page * 20 >= result.data.total || result.loading} onClick={() => setPage(p => p + 1)}>下一页</Button></div></div>}
    </div>
    {selected && <ApplicationDetail key={selected} id={selected} actor={session.data?.actor} onClose={() => setSelected(undefined)} onChange={result.reload} />}
  </Page>
}
