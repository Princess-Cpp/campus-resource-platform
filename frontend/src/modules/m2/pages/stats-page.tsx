import { useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Field, FieldLabel } from "@/components/ui/field"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Page } from "@/modules/common/page"
import { useRequest } from "@/modules/common/use-request"
import { getStats } from "../api"
import { ErrorBox } from "../components/shared"
export function StatsPage() {
  const [from, setFrom] = useState(""), [to, setTo] = useState("")
  const result = useRequest(() => getStats({ from, to }), [from, to])
  const d = result.data
  return <Page title="申请统计" description="按使用日期统计授权场地范围内的申请。仅运营权限可查看聚合结果。" actions={<Button variant="outline" size="sm" disabled={result.loading} onClick={result.reload}>刷新</Button>}><div className="space-y-4 px-4 lg:px-6">
    <Card><CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-2"><Field><FieldLabel htmlFor="stats-from">使用日期从</FieldLabel><Input id="stats-from" type="date" value={from} onChange={e => setFrom(e.target.value)} /></Field><Field><FieldLabel htmlFor="stats-to">使用日期至</FieldLabel><Input id="stats-to" type="date" value={to} onChange={e => setTo(e.target.value)} /></Field></CardContent></Card>
    {result.error !== undefined ? <ErrorBox error={result.error} /> : result.loading ? <p className="text-sm text-muted-foreground">正在加载统计…</p> : d && <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">{[["申请总数", d.total, "当前日期与授权范围内全部申请"], ["等待审批", d.pending, "包括场地确认中的申请"], ["审批通过率", d.approvalRate === null ? "—" : `${(d.approvalRate * 100).toFixed(1)}%`, `${d.approved} 次批准 / ${d.approved + d.rejected} 次审批决定`], ["平均审批耗时", d.averageHours === null ? "—" : `${d.averageHours.toFixed(1)} 小时`, "从提交到批准或驳回"]].map(([label, value, note]) => <Card key={String(label)}><CardHeader><CardDescription>{label}</CardDescription><CardTitle className="text-2xl tabular-nums">{value}</CardTitle></CardHeader><CardContent className="text-xs text-muted-foreground">{note}</CardContent></Card>)}</div>
      <Card><CardHeader><CardTitle>按教室汇总</CardTitle><CardDescription>撤销或终止不抹去此前的审批决定；待审批、直接撤销与过期不计入通过率分母。无审批决定时显示“—”。</CardDescription></CardHeader><CardContent>{d.rooms.length ? <Table><TableHeader><TableRow><TableHead>教室</TableHead><TableHead>申请数</TableHead><TableHead>批准决定</TableHead><TableHead>驳回决定</TableHead></TableRow></TableHeader><TableBody>{d.rooms.map(r => <TableRow key={r.roomId}><TableCell>{r.roomName}</TableCell><TableCell>{r.total}</TableCell><TableCell>{r.approved}</TableCell><TableCell>{r.rejected}</TableCell></TableRow>)}</TableBody></Table> : <p className="py-6 text-sm text-muted-foreground">当前范围内暂无申请。</p>}</CardContent></Card>
    </>}
  </div></Page>
}
