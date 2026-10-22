// Browser-only interaction examples. No SQL, authenticated identity, network,
// shared audit or business acceptance is provided by this adapter.
import { normalizeReport, validateReport } from "../reporting.ts"
import type { ListQuery, MaintenanceGateway, Order, OrderDetail, OrderPage, Room } from "../types.ts"

const sampleRooms: Room[] = [
  { id: "preview-room-a", name: "示例教学楼 A101", area: "A楼", status: "ACTIVE", seats: [{ id: "preview-seat-1", label: "01" }, { id: "preview-seat-2", label: "02" }] },
  { id: "preview-room-b", name: "示例教学楼 A203", area: "A楼", status: "DISABLED", seats: [{ id: "preview-seat-3", label: "03" }] },
]
export function createPreviewGateway(options: { rooms?: Room[]; seed?: boolean; now?: () => Date } = {}) {
  const rooms = structuredClone(options.rooms ?? sampleRooms)
  const orders = new Map<string, OrderDetail>()
  const commands = new Map<string, { hash: string; order: Order }>()
  const now = options.now ?? (() => new Date())
  let loseReply = false
  let failReads = false
  let sequence = 0
  if (options.seed !== false && rooms.length > 0) {
    const statuses = ["REPORTED", "ASSIGNED", "IN_PROGRESS", "AWAITING_ACCEPTANCE", "COMPLETED", "CANCELLED", "REPORTED"] as const
    statuses.forEach((state, index) => {
      const at = new Date(now().getTime() - (index + 1) * 3_600_000).toISOString()
      const id = `PREVIEW-${String(++sequence).padStart(4, "0")}`
      orders.set(id, { id, roomId: rooms[0].id, roomName: rooms[0].name, area: rooms[0].area, category: index % 2 ? "POWER" : "LIGHTING", description: index % 2 ? "示例：靠窗区域电源插座松动，使用时会出现接触不良。" : "示例：教室前排照明灯持续闪烁，影响正常阅读。", severity: "NORMAL", state, syncState: "IDLE", version: 1, createdAt: at, updatedAt: at, timeline: [{ id: `${id}-report`, action: "REPORT", text: "示例故障已上报", at }, ...(state === "REPORTED" ? [] : [{ id: `${id}-state`, action: "EXAMPLE", text: "此状态用于展示，不代表实际处理记录", at }])] })
    })
  }
  const checkRead = () => { if (failReads) throw new Error("示例读取失败，请关闭故障模拟后重试") }
  const gateway: MaintenanceGateway & { loseNextCreateReply(): void; setReadFailure(value: boolean): void } = {
    mode: "preview",
    loseNextCreateReply() { loseReply = true },
    setReadFailure(value) { failReads = value },
    async rooms() { checkRead(); return structuredClone(rooms.filter(room => room.status !== "RETIRED")) },
    async create(raw, key) {
      const input = normalizeReport(raw)
      const errors = validateReport(input, rooms)
      if (Object.keys(errors).length) throw new Error(Object.values(errors)[0])
      if (!key) throw new Error("缺少请求标识")
      const hash = JSON.stringify(input)
      const previous = commands.get(key)
      if (previous) {
        if (previous.hash !== hash) throw new Error("同一请求标识不能提交不同报修")
        return structuredClone(previous.order)
      }
      const room = rooms.find(room => room.id === input.roomId)!
      const id = `PREVIEW-${String(++sequence).padStart(4, "0")}`
      const at = now().toISOString()
      const order: Order = { ...input, id, roomName: room.name, ...(input.seatId ? { seatLabel: room.seats.find(seat => seat.id === input.seatId)?.label } : {}), area: room.area, state: "REPORTED", syncState: "IDLE", version: 1, createdAt: at, updatedAt: at }
      orders.set(id, { ...order, timeline: [{ id: `${id}-report`, action: "REPORT", text: "已创建示例报修，未发送至物业", at }] })
      commands.set(key, { hash, order: structuredClone(order) })
      if (loseReply) { loseReply = false; throw new Error("已模拟响应丢失。请重试原报修，核对同一条示例记录。") }
      return structuredClone(order)
    },
    async list(query: ListQuery): Promise<OrderPage> {
      checkRead()
      if (!Number.isInteger(query.page) || query.page < 1 || !Number.isInteger(query.pageSize) || query.pageSize < 1 || query.pageSize > 100) throw new Error("分页参数无效")
      const filtered = [...orders.values()].filter(order => (!query.state || query.state === order.state) && (!query.category || query.category === order.category) && (!query.from || Date.parse(order.createdAt) >= Date.parse(query.from)) && (!query.to || Date.parse(order.createdAt) < Date.parse(query.to)))
        .sort((a, b) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id))
      return structuredClone({ items: filtered.slice((query.page - 1) * query.pageSize, query.page * query.pageSize), page: query.page, pageSize: query.pageSize, total: filtered.length })
    },
    async detail(id) {
      checkRead()
      const order = orders.get(id)
      if (!order) throw new Error("记录不存在或无权查看")
      return structuredClone(order)
    },
  }
  return gateway
}
