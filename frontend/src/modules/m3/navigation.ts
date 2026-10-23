import type { MaintenanceSession } from "./transport.ts"

const permissions: Record<string, readonly string[]> = {
  "/m3/notifications": [],
  "/m3/workorders": [],
  "/m3/report": ["workorder.report"],
  "/m3/dispatch": ["workorder.dispatch"],
  "/m3/assigned": ["workorder.repair"],
  "/m3/acceptance": ["workorder.accept"],
  "/m3/stats": ["workorder.stats"],
  "/m3/escalations": ["workorder.supervise", "workorder.escalation.read"],
}

export type MaintenancePage = "notifications" | "report" | "orders" | "detail" | "dispatch" | "assigned" | "acceptance" | "stats" | "escalations"
export function canOpenMaintenancePage(page: MaintenancePage, granted: readonly string[]): boolean {
  if (page === "detail") return true // The detail API checks ownership and area.
  const required = permissions[page === "orders" ? "/m3/workorders" : `/m3/${page}`]
  return required !== undefined && (required.length === 0 || required.some(p => granted.includes(p)))
}

// Presentation only. APIs still check current permission, area and ownership.
// Bind the response to the active identity; never display a previous user's menu.
export function visibleMaintenanceItems<T extends { url: string }>(
  items: readonly T[], actorId: string | undefined, session: MaintenanceSession | undefined,
  loading: boolean, error: unknown,
): T[] {
  if (!actorId || loading || error !== undefined || session?.actor.id !== actorId) return []
  return items.filter(item => {
    const required = permissions[item.url]
    return required !== undefined && (required.length === 0 || required.some(p => session.actor.permissions.includes(p)))
  })
}
