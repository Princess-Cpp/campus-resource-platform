import * as React from "react"
import { Link } from "react-router"
import { GraduationCapIcon } from "lucide-react"

import { NavMain } from "@/components/nav-main"
import { NavUser } from "@/components/nav-user"
import { Sidebar, SidebarContent, SidebarFooter, SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"
import type { ModuleDefinition } from "@/modules/common/navigation"

export function AppSidebar({ modules, ...props }: React.ComponentProps<typeof Sidebar> & { modules: ModuleDefinition[] }) {
  return (
    <Sidebar collapsible="offcanvas" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton className="data-[slot=sidebar-menu-button]:p-1.5!" render={<Link to="/" />}>
              <GraduationCapIcon className="size-5!" />
              <span className="text-base font-semibold">校园资源服务平台</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain modules={modules} />
      </SidebarContent>
      <SidebarFooter>
        <NavUser />
      </SidebarFooter>
    </Sidebar>
  )
}
