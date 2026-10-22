import type { ReactNode } from "react"

import { NotFoundPage } from "@/app/not-found-page"
import { hasAnyPermission } from "@/modules/common/navigation"
import { useSession } from "@/modules/common/session"

export function RequirePermission({ anyOf, children }: { anyOf: string[]; children: ReactNode }) {
  const session = useSession()
  if (!session.data || !hasAnyPermission(session.data.actor.permissions, anyOf)) {
    return <NotFoundPage />
  }
  return children
}

export function withPermission(element: ReactNode, permissions?: string[]) {
  if (!permissions?.length) return element
  return <RequirePermission anyOf={permissions}>{element}</RequirePermission>
}
