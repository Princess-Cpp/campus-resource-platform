import type { ModuleDefinition } from "@/modules/common/navigation"
import { request } from "@/modules/common/api"
import { useSession } from "@/modules/common/session"
import { useRequest } from "@/modules/common/use-request"
import { createHTTPGateway } from "./transport"
import { visibleMaintenanceItems } from "./navigation"

const gateway = createHTTPGateway(request)

// Owned by M3; the shell's ModuleDefinition contract stays unchanged.
export function useMaintenanceNavigation(modules: ModuleDefinition[]) {
  const auth = useSession()
  const actorId = auth.data?.actor.id
  const current = useRequest(() => actorId ? gateway.session() : Promise.resolve(undefined), [actorId])
  return modules.map(module => module.id !== "m3" ? module : {
    ...module,
    nav: visibleMaintenanceItems(module.nav, actorId, current.data, current.loading, current.error),
  })
}
