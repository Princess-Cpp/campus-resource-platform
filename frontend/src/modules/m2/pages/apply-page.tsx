import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"
import { SearchIcon, SchoolIcon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Field, FieldLabel, FieldDescription } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle, EmptyDescription } from "@/components/ui/empty"
import { Page } from "@/modules/common/page"
import { ApiError } from "@/modules/common/api"
import { useRequest } from "@/modules/common/use-request"
import { campusToday, campusInstant, formatRange } from "@/modules/common/time"
import { getCurrentSession } from "@/modules/common/session"
import { createApplication, getSession, searchRooms, type CreateInput, type Room } from "../api"
import { Choice, ErrorBox } from "../components/shared"

type Pending = { key: string; input: CreateInput; roomName: string }
const pendingKey = () => `campus.m2.pending.${getCurrentSession()?.actor.id ?? "anonymous"}`
function readPending(): Pending | undefined { try { return JSON.parse(sessionStorage.getItem(pendingKey()) || "null") ?? undefined } catch { return undefined } }
export function ApplyPage() {
  const navigate = useNavigate()
  const session = useRequest(getSession, [])
  const [date, setDate] = useState(campusToday(new Date(Date.now() + 86400000)))
  const [hour, setHour] = useState("9"), [duration, setDuration] = useState("2"), [area, setArea] = useState(""), [capacity, setCapacity] = useState("10")
  const [result, setResult] = useState<{ rooms: Room[]; startAt: string; endAt: string }>()
  const [selected, setSelected] = useState<Room>(), [purpose, setPurpose] = useState(""), [attendees, setAttendees] = useState("10"), [contact, setContact] = useState("")
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [formError, setFormError] = useState<unknown>()
  const [pending, setPending] = useState<Pending | undefined>(readPending)
  const search = async (e: FormEvent) => {
    e.preventDefault(); setBusy(true); setError(undefined)
    const start = campusInstant(date, Number(hour)), end = new Date(start.getTime() + Number(duration) * 3600000)
    try { const rooms = await searchRooms({ startAt: start.toISOString(), endAt: end.toISOString(), area: area.trim(), capacity }); setResult({ rooms, startAt: start.toISOString(), endAt: end.toISOString() }) } catch (e) { setError(e); setResult(undefined) } finally { setBusy(false) }
  }
  const submit = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!pending && (!selected || !result)) return
    const request = pending ?? { key: crypto.randomUUID(), roomName: selected!.name, input: { roomId: selected!.id, startAt: result!.startAt, endAt: result!.endAt, purpose: purpose.trim(), attendees: Number(attendees), contactRef: contact } }
    try { sessionStorage.setItem(pendingKey(), JSON.stringify(request)) } catch { setFormError(new Error("浏览器无法保存本次请求，请允许会话存储后再提交。")); return }
    setPending(request); setBusy(true); setFormError(undefined)
    try { await createApplication(request.input, request.key); sessionStorage.removeItem(pendingKey()); setPending(undefined); toast.success("申请已提交，等待院系审批"); navigate("/m2/applications") }
    catch (e) { setFormError(e); if (e instanceof ApiError && e.status >= 400 && e.status < 500) { sessionStorage.removeItem(pendingKey()); setPending(undefined) } }
    finally { setBusy(false) }
  }
  return <Page title="申请教室" description="查找整间教室，提交活动申请。待审批申请不占用场地，最终结果以场地审批为准。">
    <div className="space-y-4 px-4 lg:px-6">
      {session.error !== undefined && <ErrorBox error={session.error} />}
      {error !== undefined && <ErrorBox error={error} />}
      {pending && !selected && <Card><CardHeader><CardTitle>有一笔申请待确认</CardTitle><CardDescription>{pending.roomName} · {formatRange(pending.input.startAt, pending.input.endAt)}。沿用原请求核对，避免重复申请。</CardDescription></CardHeader><CardContent className="space-y-3">{formError !== undefined && <ErrorBox error={formError} />}<Button disabled={busy} onClick={() => submit()}>核对并继续提交</Button></CardContent></Card>}
      <Card><CardHeader><CardTitle>选择使用时段</CardTitle><CardDescription>目前采用待评审规则：最长 {session.data?.policy.maxHours ?? 4} 小时，提前 {session.data?.policy.advanceDays ?? 14} 天内申请。</CardDescription></CardHeader><CardContent>
        <form onSubmit={search} className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-6 xl:items-end">
          <Field><FieldLabel htmlFor="m2-date">日期</FieldLabel><Input id="m2-date" type="date" value={date} min={campusToday()} onChange={e => setDate(e.target.value)} required /></Field>
          <Field><FieldLabel>开始时间</FieldLabel><Choice label="开始时间" value={hour} onChange={setHour} options={Array.from({ length: 24 }, (_, i) => ({ value: String(i), label: `${String(i).padStart(2, "0")}:00` }))} /></Field>
          <Field><FieldLabel>时长</FieldLabel><Choice label="时长" value={duration} onChange={setDuration} options={Array.from({ length: session.data?.policy.maxHours ?? 4 }, (_, i) => ({ value: String(i + 1), label: `${i + 1} 小时` }))} /></Field>
          <Field><FieldLabel htmlFor="m2-capacity">预计人数</FieldLabel><Input id="m2-capacity" type="number" min={1} max={10000} value={capacity} onChange={e => setCapacity(e.target.value)} required /></Field>
          <Field><FieldLabel htmlFor="m2-area">区域</FieldLabel><Input id="m2-area" placeholder="例如 A楼" value={area} onChange={e => setArea(e.target.value)} /></Field>
          <Button disabled={busy || session.loading} type="submit"><SearchIcon />{busy ? "处理中…" : "查找教室"}</Button>
        </form>
      </CardContent></Card>
      {result && result.rooms.length > 0 ? <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">{result.rooms.map(room => <Card key={room.id}><CardHeader><CardTitle>{room.name}</CardTitle><CardDescription>{room.area} · 容量 {room.capacity} 人 · {room.department || "全校开放"}</CardDescription></CardHeader><CardContent className="space-y-4"><p className="text-sm text-muted-foreground">{formatRange(result.startAt, result.endAt)}</p><Button disabled={!!pending || !session.data?.actor.permissions.includes("classroom.apply")} onClick={() => { setSelected(room); setAttendees(capacity); setPurpose(""); setContact(session.data?.actor.contactRefs[0] ?? ""); setFormError(undefined) }}>填写申请</Button></CardContent></Card>)}</div> : <Empty className="border"><EmptyHeader><EmptyMedia variant="icon"><SchoolIcon /></EmptyMedia><EmptyTitle>{result ? "没有符合条件的教室" : "选择日期和时段开始查找"}</EmptyTitle><EmptyDescription>{result ? "尝试调整时段、区域或人数。" : "仅显示有权访问且整个时段可用的教室。"}</EmptyDescription></EmptyHeader></Empty>}
      {session.data && !session.data.actor.permissions.includes("classroom.apply") && <p className="text-sm text-muted-foreground">当前身份没有提交申请权限，可以切换至具备申请权限的开发身份。</p>}
    </div>
    <Dialog open={!!selected} onOpenChange={open => { if (!open && !busy) setSelected(undefined) }}><DialogContent className="max-h-[90dvh] overflow-y-auto"><form onSubmit={submit} className="space-y-4"><DialogHeader><DialogTitle>申请 {selected?.name}</DialogTitle><DialogDescription>{result && formatRange(result.startAt, result.endAt)} · 提交后不可直接修改，需撤销后重新申请。</DialogDescription></DialogHeader>
      {formError !== undefined && <ErrorBox error={formError} />}
      <Field><FieldLabel htmlFor="m2-purpose">活动用途</FieldLabel><Textarea id="m2-purpose" value={purpose} onChange={e => setPurpose(e.target.value)} minLength={10} maxLength={500} disabled={!!pending} required /><FieldDescription>请填写 10–500 字，说明活动内容。</FieldDescription></Field>
      <Field><FieldLabel htmlFor="m2-attendees">预计人数</FieldLabel><Input id="m2-attendees" type="number" min={1} max={selected?.capacity} value={attendees} onChange={e => setAttendees(e.target.value)} disabled={!!pending} required /></Field>
      <Field><FieldLabel>通知联系方式</FieldLabel><Choice label="通知联系方式" value={contact} onChange={setContact} options={(session.data?.actor.contactRefs ?? []).map(value => ({ value, label: value === "campus-email" ? "校园邮箱（开发配置）" : value }))} disabled={!!pending} /><FieldDescription>使用已配置的联系方式；外部通知投递尚未接入。</FieldDescription></Field>
      {pending && <p className="text-sm text-muted-foreground">正在核对同一笔请求，表单已锁定。</p>}
      <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={() => setSelected(undefined)}>关闭</Button><Button type="submit" disabled={busy || !contact}>{busy ? "提交中…" : pending ? "重试原申请" : "确认提交"}</Button></DialogFooter>
    </form></DialogContent></Dialog>
  </Page>
}
