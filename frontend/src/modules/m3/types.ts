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
export type ActionName = "ASSIGN" | "ACCEPT_ASSIGNMENT" | "CANCEL" | "START" | "FINISH" | "REJECT_ACCEPTANCE" | "APPROVE_ACCEPTANCE" | "RETRY_RESOURCE" | "EVALUATE"
export type ActionInput = {action:ActionName;expectedVersion:number;assigneeId?:string;comment?:string;score?:number}
export type Worker = {id:string;name:string}
export interface ActionGateway {
 act(id:string,input:ActionInput,key:string):Promise<Order>
 assignees(id:string):Promise<Worker[]>
}
export type ListView = "mine" | "dispatch" | "assigned" | "acceptance"
export type Order = ReportInput & {
 evaluation?:{score:number;comment:string;at:string}
 evaluationDeadlineAt?:string
 stageNumber?:number
 escalated?:boolean
 policyVersion?:string
 repairSeconds?:number
 acceptanceSeconds?:number
 result?:string
 firstStartedAt?:string
 completedAt?:string
 lastResourceError?:string
 operation?:{id:string;action:string;state:string;attempts:number;nextAttempt?:string;lastError?:string}
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

export type Metric={count:number;averageSeconds:number|null}
export type Statistics={asOf:string;total:number;byState:Partial<Record<OrderState,number>>;eligibleCount:number;recordedOverdueCount:number;recordedOverdueRate:number|null;resourceUnresolvedCount:number;ratedCount:number;averageScore:number|null;dispatch:Metric;firstRepair:Metric;repair:Metric;acceptance:Metric}
export type Escalation={id:string;workorderId:string;roomName:string;area:string;stageNumber:number;level:number;policyVersion:string;stageState:OrderState;currentState:OrderState;deadlineAt:string;detectedAt:string;routingState:string;active:boolean;canOpen:boolean}
export type EscalationPage={items:Escalation[];page:number;pageSize:number;total:number}
export type OperationsQuery={from?:string;to?:string;category?:Category}
export interface OperationsGateway {stats(query:OperationsQuery):Promise<Statistics>;escalations(query:OperationsQuery & {page:number;pageSize:number;active:boolean}):Promise<EscalationPage>}
