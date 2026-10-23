import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"
import { InfoIcon, WrenchIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import { Page } from "@/modules/common/page"
import { useRequest } from "@/modules/common/use-request"
import { categories, normalizeReport, ReportSubmission, validateReport } from "../reporting"
import { ErrorBox, FormError, selectClass } from "../components/shared"
import type { MaintenanceGateway, ReportInput } from "../types"

const emptyInput: ReportInput = { roomId: "", seatId: "", category: "LIGHTING", severity: "NORMAL", description: "" }

export function ReportPage({ gateway, submission }: { gateway: MaintenanceGateway; submission: ReportSubmission }) {
  const navigate = useNavigate()
  const rooms = useRequest(() => gateway.rooms(), [gateway])
  const [input, setInput] = useState<ReportInput>(() => submission.draft ?? { ...emptyInput })
  const [errors, setErrors] = useState<ReturnType<typeof validateReport>>({})
  const [failure, setFailure] = useState<unknown>()
  const [busy, setBusy] = useState(false)
  const room = rooms.data?.find(room => room.id === input.roomId)
  const locked = busy || submission.locked || !!submission.problem
  const count = Array.from(input.description.trim()).length
  function change<K extends keyof ReportInput>(key: K, value: ReportInput[K]) {
    setInput(previous => ({ ...previous, [key]: value, ...(key === "roomId" ? { seatId: "" } : {}) }))
    setErrors(previous => ({ ...previous, [key]: undefined, ...(key === "roomId" ? { seatId: undefined } : {}) }))
  }
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy || !rooms.data || submission.problem) return
    const errors = validateReport(input, rooms.data)
    setErrors(errors)
    if (Object.keys(errors).length) return
    setBusy(true); setFailure(undefined)
    try {
      const order = await submission.send(normalizeReport(input))
      navigate(`/m3/workorders/${encodeURIComponent(order.id)}`, { state: { created: true } })
    } catch (error) { setFailure(error) }
    finally { setBusy(false) }
  }
  return <Page title="故障报修" description="说明教室或座位的问题，便于物业人员定位和处理。" actions={<Button variant="outline" onClick={() => navigate("/m3/workorders")}>我的报修</Button>}>
    <div className="grid min-w-0 gap-6 px-4 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)] lg:px-6">
      <Card className="min-w-0"><CardHeader><CardTitle>填写故障信息</CardTitle><CardDescription>教室、故障类别和描述为必填项。</CardDescription></CardHeader><CardContent>
        <form noValidate onSubmit={submit} className="space-y-6">
          {rooms.error !== undefined && <div className="space-y-2"><ErrorBox error={rooms.error} /><Button type="button" variant="outline" onClick={rooms.reload}>重新加载教室</Button></div>}
          {rooms.loading && <p role="status" className="text-sm text-muted-foreground">正在加载可报修教室…</p>}
          {!rooms.loading && rooms.data?.length === 0 && <Alert><AlertTitle>暂无可报修教室</AlertTitle><AlertDescription>请联系管理员确认你的场地访问范围。</AlertDescription></Alert>}
          {failure !== undefined && <ErrorBox error={failure} />}
          {submission.problem && <ErrorBox error={submission.problem} />}
          {submission.locked && !busy && <Alert><InfoIcon /><AlertTitle>原报修结果待核对</AlertTitle><AlertDescription>表单已锁定。请重试原报修，避免重复上报同一请求。</AlertDescription></Alert>}
          <div className="grid gap-5 sm:grid-cols-2">
            <Field><FieldLabel htmlFor="m3-room">教室 <span aria-hidden="true">*</span></FieldLabel>
              <select id="m3-room" className={selectClass} value={input.roomId} onChange={e => change("roomId", e.target.value)} disabled={locked || rooms.loading || rooms.error !== undefined} aria-required="true" aria-invalid={!!errors.roomId} aria-describedby={errors.roomId ? "m3-room-error" : undefined}>
                <option value="">请选择教室</option>{rooms.data?.map(room => <option key={room.id} value={room.id}>{room.name}{room.status === "DISABLED" ? "（已停用，可报修）" : ""}</option>)}
              </select><FormError id="m3-room-error" message={errors.roomId} />
            </Field>
            <Field><FieldLabel htmlFor="m3-seat">座位 <span className="font-normal text-muted-foreground">选填</span></FieldLabel>
              <select id="m3-seat" className={selectClass} value={input.seatId ?? ""} onChange={e => change("seatId", e.target.value)} disabled={locked || !room || rooms.error !== undefined} aria-invalid={!!errors.seatId} aria-describedby={errors.seatId ? "m3-seat-error" : undefined}>
                <option value="">整个教室 / 不确定座位</option>{room?.seats.map(seat => <option key={seat.id} value={seat.id}>{seat.label} 号座位</option>)}
              </select><FormError id="m3-seat-error" message={errors.seatId} />
            </Field>
            <Field><FieldLabel htmlFor="m3-category">故障类别 <span aria-hidden="true">*</span></FieldLabel>
              <select id="m3-category" className={selectClass} value={input.category} onChange={e => change("category", e.target.value as ReportInput["category"])} disabled={locked} aria-required="true" aria-invalid={!!errors.category} aria-describedby={errors.category ? "m3-category-error" : undefined}>
                {categories.map(category => <option key={category.value} value={category.value}>{category.label}</option>)}
              </select><FormError id="m3-category-error" message={errors.category} />
            </Field>
            <Field><FieldLabel htmlFor="m3-severity">紧急程度</FieldLabel>
              <select id="m3-severity" className={selectClass} value={input.severity} onChange={e => change("severity", e.target.value as ReportInput["severity"])} disabled={locked} aria-describedby="m3-severity-help">
                <option value="NORMAL">一般</option><option value="URGENT">紧急</option>
              </select><FieldDescription id="m3-severity-help">紧急标记由调度员复核。</FieldDescription>
            </Field>
          </div>
          <Field><FieldLabel htmlFor="m3-description">故障描述 <span aria-hidden="true">*</span></FieldLabel>
            <Textarea id="m3-description" rows={6} className="min-h-40 resize-y" placeholder="例如：A101 教室前排第二盏灯持续闪烁，从今天上午开始影响阅读。" value={input.description} onChange={e => change("description", e.target.value)} disabled={locked} aria-required="true" aria-invalid={!!errors.description} aria-describedby="m3-description-help m3-description-error" />
            <div className="flex justify-between gap-3 text-sm"><FieldDescription id="m3-description-help">请写明位置、现象和发生时间。</FieldDescription><span className={count > 1000 ? "shrink-0 text-destructive" : "shrink-0 text-muted-foreground"}>{count} / 1000</span></div>
            <FormError id="m3-description-error" message={errors.description} />
          </Field>
          <div className="flex flex-wrap items-center gap-3 border-t pt-4"><Button type="submit" size="lg" disabled={busy || !!submission.problem || rooms.loading || rooms.error !== undefined || rooms.data?.length === 0}><WrenchIcon />{busy ? "处理中…" : gateway.mode === "preview" ? submission.locked ? "重试原示例报修" : "创建示例报修" : submission.locked ? "重试原报修" : "提交报修"}</Button><span className="text-xs text-muted-foreground">故障描述请填写 10–1000 字。</span></div>
        </form>
      </CardContent></Card>
      <aside className="space-y-5 self-start text-sm">
        <div className="space-y-3 border-l-2 border-foreground/20 pl-4"><h3 className="font-medium">让问题更容易被找到</h3><p className="leading-6 text-muted-foreground">选择故障所在教室；涉及具体座位时，可补充座位编号。没有准确编号也可以上报。</p></div>
        <div className="space-y-2 pl-4"><h3 className="font-medium">保护个人信息</h3><p className="leading-6 text-muted-foreground">无需填写学号、电话或密码。描述只需包含处理故障所必需的信息。</p></div>
        <div className="space-y-2 pl-4"><h3 className="font-medium">上报后</h3><p className="leading-6 text-muted-foreground">可以在“我的报修”查看状态和处理记录。派单、维修与验收由相应工作人员完成。</p></div>
      </aside>
    </div>
  </Page>
}
