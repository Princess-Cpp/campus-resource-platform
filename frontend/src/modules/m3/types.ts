// M3 caller-side draft types. These do not freeze a shared HTTP contract.
export type Category = "LIGHTING" | "POWER" | "FURNITURE" | "DOOR_WINDOW" | "OTHER"
export type Severity = "NORMAL" | "URGENT"
export type OrderState = "REPORTED" | "ASSIGNED" | "IN_PROGRESS" | "AWAITING_ACCEPTANCE" | "COMPLETED" | "CANCELLED"
export type SyncState = "IDLE" | "PENDING" | "UNKNOWN" | "NEEDS_ATTENTION"
export type Room = {
  id: string
  name: string
  area: string
  status: "ACTIVE" | "DISABLED" | "RETIRED"
  seats: { id: string; label: string }[]
}
export type ReportInput = { roomId: string; seatId?: string; category: Category; description: string; severity: Severity }
export type ActionName = "ASSIGN" | "ACCEPT_ASSIGNMENT" | "CANCEL"
export type ActionInput = {action:ActionName;expectedVersion:number;assigneeId?:string;comment?:string}
export type Worker = {id:string;name:string}
export interface ActionGateway {
 act(id:string,input:ActionInput,key:string):Promise<Order>
 assignees(id:string):Promise<Worker[]>
}
export type ListView = "mine" | "dispatch" | "assigned"
export type Order = ReportInput & {
 assigneeId?:string
 assigneeName?:string
 acceptedAt?:string
 firstAssignedAt?:string
 deadlineAt?:string
  id: string
  roomName: string
  seatLabel?: string
  area: string
  state: OrderState
  syncState: SyncState
  version: number
  createdAt: string
  updatedAt: string
}
export type TimelineEntry = { id: string; action: string; text: string; at: string; actorId?:string; comment?:string }
export type OrderDetail = Order & { timeline: TimelineEntry[]; allowedActions?:ActionName[] }
export type ListQuery = { view?:ListView; page: number; pageSize: number; state?: OrderState; category?: Category; from?: string; to?: string }
export type OrderPage = { items: Order[]; page: number; pageSize: number; total: number }
export interface MaintenanceGateway {
  readonly mode: "preview" | "api"
  rooms(): Promise<Room[]>
  create(input: ReportInput, key: string): Promise<Order>
  list(query: ListQuery): Promise<OrderPage>
  detail(id: string): Promise<OrderDetail>
}
