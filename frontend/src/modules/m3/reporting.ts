import type { MaintenanceGateway, Order, OrderState, ReportInput, Room, SyncState } from "./types.ts"

export const categories = [
  { value: "LIGHTING", label: "照明" },
  { value: "POWER", label: "电源" },
  { value: "FURNITURE", label: "桌椅" },
  { value: "DOOR_WINDOW", label: "门窗" },
  { value: "OTHER", label: "其他" },
] as const
export const states: Record<OrderState, string> = {
  REPORTED: "待派单", ASSIGNED: "已派单", IN_PROGRESS: "维修中",
  AWAITING_ACCEPTANCE: "待验收", COMPLETED: "已完成", CANCELLED: "已取消",
}
export function normalizeReport(input: ReportInput): ReportInput {
  return { roomId: input.roomId, ...(input.seatId ? { seatId: input.seatId } : {}), category: input.category, description: input.description.trim(), severity: input.severity }
}
export function queryDates(fromDate: string, toDate: string): { from?: string; to?: string } {
  function midnight(date: string) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error("请选择有效日期")
    const value = new Date(`${date}T00:00:00+08:00`)
    const [year, month, day] = date.split("-").map(Number)
    const calendar = new Date(Date.UTC(year, month - 1, day))
    if (!Number.isFinite(value.getTime()) || calendar.getUTCFullYear() !== year || calendar.getUTCMonth() + 1 !== month || calendar.getUTCDate() !== day) throw new Error("请选择有效日期")
    return value
  }
  if (fromDate && toDate && fromDate > toDate) throw new Error("开始日期不能晚于结束日期")
  const from = fromDate ? midnight(fromDate).toISOString() : undefined
  const to = toDate ? new Date(midnight(toDate).getTime() + 86_400_000).toISOString() : undefined
  return { ...(from ? { from } : {}), ...(to ? { to } : {}) }
}
export function validateReport(input: ReportInput, rooms: Room[]): Partial<Record<keyof ReportInput, string>> {
  const errors: Partial<Record<keyof ReportInput, string>> = {}
  const room = rooms.find(room => room.id === input.roomId && room.status !== "RETIRED")
  if (!room) errors.roomId = "请选择可报修的教室"
  if (input.seatId && !room?.seats.some(seat => seat.id === input.seatId)) errors.seatId = "所选座位不属于当前教室，请重新选择"
  if (!categories.some(option => option.value === input.category)) errors.category = "请选择故障类别"
  const length = Array.from(input.description.trim()).length
  if (length < 10 || length > 1000) errors.description = "请填写 10–1000 字的故障描述"
  if (input.severity !== "NORMAL" && input.severity !== "URGENT") errors.severity = "请选择一般或紧急"
  return errors
}
export function statusText(order: { state: OrderState; syncState: SyncState }): string {
  if (order.syncState !== "IDLE") {
    if (order.syncState === "PENDING" && order.state === "ASSIGNED") return "维修准备中"
    if (order.syncState === "PENDING" && order.state === "AWAITING_ACCEPTANCE") return "恢复处理中"
    return "状态待核对"
  }
  return states[order.state]
}

export type PendingReport = { input: ReportInput; key: string }
export interface PendingStore { load(): PendingReport | undefined; save(value: PendingReport): void; clear(): void }
// Client recovery complements (and never replaces) server SQL idempotency.
export class ReportSubmission {
  private pending?: PendingReport
  private inFlight?: Promise<Order>
  private gateway: MaintenanceGateway
  private key: () => string
  private store?: PendingStore
  private uncertain = false
  readonly problem?: Error
  constructor(gateway: MaintenanceGateway, key: () => string = () => crypto.randomUUID(), store?: PendingStore) {
    this.gateway = gateway; this.key = key; this.store = store
    try { this.pending = store?.load(); this.uncertain = !!this.pending }
    catch { this.problem = new Error("无法读取未确认报修，请检查浏览器会话存储后刷新；当前不会发送新请求") }
  }
  get locked() { return this.pending !== undefined }
  get draft() { return this.pending ? structuredClone(this.pending.input) : undefined }
  send(input: ReportInput): Promise<Order> {
    if (this.problem) return Promise.reject(this.problem)
    const normalized = normalizeReport(input)
    if (this.pending && JSON.stringify(this.pending.input) !== JSON.stringify(normalized)) return Promise.reject(new Error("原报修尚待核对，请重试原报修"))
    if (this.inFlight) return this.inFlight
    if (!this.pending) {
      const pending = { input: normalized, key: this.key() }
      try { this.store?.save(pending) } catch { return Promise.reject(new Error("浏览器无法保存请求，请允许会话存储后重试；报修尚未发送")) }
      this.pending = pending
    }
    const request = this.pending
    this.inFlight = this.gateway.create(structuredClone(request.input), request.key)
      .then(order => { this.store?.clear(); this.pending = undefined; this.uncertain = false; return order })
      .catch(error => {
        const status = error && typeof error === "object" && "status" in error ? Number(error.status) : 0
        if (!this.uncertain && status >= 400 && status < 500 && status !== 408 && status !== 429) {
          this.store?.clear(); this.pending = undefined
        } else this.uncertain = true
        throw error
      })
      .finally(() => { this.inFlight = undefined })
    return this.inFlight
  }
}
