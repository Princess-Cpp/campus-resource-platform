import type { CSSProperties } from "react"
import { Outlet } from "react-router"

import { AppSidebar } from "@/components/app-sidebar"
import { SiteHeader } from "@/components/site-header"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { modules } from "@/app/modules"
import { visibleModules } from "@/modules/common/navigation"
import { useSession } from "@/modules/common/session"

export function AppLayout() {
  const session = useSession()
  const visible = visibleModules(modules, session.data?.actor.permissions)
  return (
    <SidebarProvider style={{ "--sidebar-width": "calc(var(--spacing) * 64)", "--header-height": "calc(var(--spacing) * 12)" } as CSSProperties}>
      <AppSidebar variant="inset" modules={visible} />
      <SidebarInset className="min-w-0">
        <SiteHeader />
        <div className="flex min-w-0 flex-1 flex-col">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
