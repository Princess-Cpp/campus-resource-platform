import { request } from "@/modules/common/api"
export type Actor = { id: string; department: string; permissions: string[]; areas: string[]; contactRefs: string[] }
export type Session = { actor: Actor; policy: { version: string; maxHours: number; advanceDays: number; approvalHours: number }; development: boolean }
export type Room = { id: string; name: string; area: string; department: string; capacity: number; openHour: number; closeHour: number }
export type Application = {
  id: string; userId: string; department: string; roomId: string; roomName: string; area: string
  startAt: string; endAt: string; createdAt: string; deadlineAt: string; status: string; version: number; syncStatus: string; lastError?: string; operationId?: string
  details: { purpose: string; attendees: number; capacity: number; contactRef?: string; departmentApprovers: string[]; finalApprovers: string[]; departmentActor: string }
  events?: { id: string; actorId: string; action: string; fromStatus: string; toStatus: string; comment: string; createdAt: string }[]
}
export type CreateInput = { roomId: string; startAt: string; endAt: string; purpose: string; attendees: number; contactRef: string }
export type PageResult = { items: Application[]; total: number; page: number; pageSize: number }
export type Stats = { total: number; approved: number; rejected: number; pending: number; approvalRate: number | null; averageHours: number | null; rooms: { roomId: string; roomName: string; total: number; approved: number; rejected: number }[] }
export const getSession = () => request<Session>("/m2/session")
export const searchRooms = (q: Record<string, string>) => request<Room[]>("/m2/rooms?" + new URLSearchParams(q))
export const createApplication = (q: CreateInput, key: string) => request<Application>("/m2/applications", { method: "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify(q) })
export const listApplications = (inbox: boolean, q: Record<string, string>) => request<PageResult>(`/m2/${inbox ? "approvals" : "applications"}?` + new URLSearchParams(q))
export const getApplication = (id: string) => request<Application>(`/m2/applications/${encodeURIComponent(id)}`)
export const act = (id: string, q: { action: string; expectedVersion: number; comment: string }, key: string) => request<Application>(`/m2/applications/${encodeURIComponent(id)}/actions`, { method: "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify(q) })
export const retry = (id: string, expectedVersion: number) => request<Application>(`/m2/applications/${encodeURIComponent(id)}/retry`, { method: "POST", body: JSON.stringify({ expectedVersion }) })
export const getStats = (q: Record<string, string>) => request<Stats>("/m2/stats?" + new URLSearchParams(q))
