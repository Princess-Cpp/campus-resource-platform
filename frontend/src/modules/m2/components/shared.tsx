import { Badge } from "@/components/ui/badge"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { describeError } from "@/modules/common/api"
import type { Application } from "../api"
export const statuses: Record<string, string> = { SUBMITTED: "待院系审批", DEPARTMENT_APPROVED: "待场地审批", APPROVED: "已批准", REJECTED: "已驳回", CANCELLED: "已撤销", EXPIRED: "已过期", TERMINATED: "已终止" }
export function Status({ value }: { value: Application }) {
  const pending = value.syncStatus !== "IDLE"
  const text = pending ? (value.syncStatus === "FAILED" ? "同步待处理" : value.status === "APPROVED" ? "释放确认中" : "场地确认中") : statuses[value.status] ?? value.status
  return <Badge variant={value.status === "REJECTED" || value.syncStatus === "FAILED" ? "destructive" : "secondary"}>{text}</Badge>
}
export function ErrorBox({ error }: { error: unknown }) { return <Alert variant="destructive"><AlertTitle>操作未完成</AlertTitle><AlertDescription>{describeError(error)}</AlertDescription></Alert> }
export function Choice({ value, onChange, options, label, disabled }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[]; label: string; disabled?: boolean }) {
  return <Select items={options} value={value} onValueChange={v => v !== null && onChange(v)} disabled={disabled}><SelectTrigger aria-label={label} className="w-full"><SelectValue /></SelectTrigger><SelectContent>{options.map(o => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent></Select>
}
