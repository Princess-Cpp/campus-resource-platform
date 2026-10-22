import type { ActionGateway,ActionInput,Worker,ListQuery, MaintenanceGateway, Order, OrderDetail, OrderPage, ReportInput, Room } from "./types.ts"

export type MaintenanceSession = { actor: { id: string; permissions: string[] }; development: boolean }
export type Transport = <T>(path: string, init?: RequestInit) => Promise<T>
export type HTTPGateway = MaintenanceGateway & ActionGateway & { session(): Promise<MaintenanceSession> }

// The shared transport supplies Cookie/CSRF and error handling. No identity or
// authorization is inferred from form fields or cached permission strings.
export function createHTTPGateway(send: Transport): HTTPGateway {
  return {
    mode: "api",
    act:(id:string,input:ActionInput,key:string)=>send<Order>("/m3/workorders/"+encodeURIComponent(id)+"/actions",{method:"POST",headers:{"Idempotency-Key":key},body:JSON.stringify(input)}),
    assignees:(id:string)=>send<Worker[]>("/m3/workorders/"+encodeURIComponent(id)+"/assignees"),
    session: () => send<MaintenanceSession>("/m3/session"),
    rooms: () => send<Room[]>("/m3/rooms"),
    create: (input: ReportInput, key: string) => send<Order>("/m3/workorders", { method: "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify(input) }),
    list: (query: ListQuery) => {
      const params = new URLSearchParams()
      for (const [name, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(name, String(value))
      return send<OrderPage>("/m3/workorders?" + params.toString())
    },
    detail: (id: string) => send<OrderDetail>("/m3/workorders/" + encodeURIComponent(id)),
  }
}
