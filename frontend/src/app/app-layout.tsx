import type { CSSProperties } from "react"
import { Outlet } from "react-router"

import { AppSidebar } from "@/components/app-sidebar"
import { SiteHeader } from "@/components/site-header"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { modules } from "@/app/modules"

export function AppLayout() {
  return (
    <SidebarProvider style={{ "--sidebar-width": "calc(var(--spacing) * 64)", "--header-height": "calc(var(--spacing) * 12)" } as CSSProperties}>
      <AppSidebar variant="inset" modules={modules} />
      <SidebarInset className="min-w-0">
        <SiteHeader />
        <div className="flex min-w-0 flex-1 flex-col">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
