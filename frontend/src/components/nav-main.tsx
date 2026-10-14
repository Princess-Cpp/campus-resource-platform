import { NavLink, useLocation } from "react-router"
import { BellIcon, CirclePlusIcon, LayoutDashboardIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import type { ModuleDefinition } from "@/modules/common/navigation"

export function NavMain({ modules }: { modules: ModuleDefinition[] }) {
  const { pathname } = useLocation()
  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent className="flex flex-col gap-2">
          <SidebarMenu>
            <SidebarMenuItem className="flex items-center gap-2">
              <SidebarMenuButton
                tooltip="预约座位"
                render={<NavLink to="/m1/search" />}
                className="min-w-8 bg-primary text-primary-foreground duration-200 ease-linear hover:bg-primary/90 hover:text-primary-foreground active:bg-primary/90 active:text-primary-foreground"
              >
                <CirclePlusIcon />
                <span>预约座位</span>
              </SidebarMenuButton>
              <Button
                size="icon"
                className="size-8 group-data-[collapsible=icon]:opacity-0"
                variant="outline"
                render={<NavLink to="/m1/notifications" />}
              >
                <BellIcon />
                <span className="sr-only">通知记录</span>
              </Button>
            </SidebarMenuItem>
          </SidebarMenu>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton tooltip="工作台" isActive={pathname === "/"} render={<NavLink to="/" end />}>
                <LayoutDashboardIcon />
                <span>工作台</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
      {modules.map((module) => (
        <SidebarGroup key={module.id}>
          <SidebarGroupLabel className="gap-2">
            {module.label}
            {module.status === "planned" && (
              <Badge variant="outline" className="h-4 px-1.5 text-[10px] font-normal">
                开发中
              </Badge>
            )}
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {module.nav.map((item) => (
                <SidebarMenuItem key={item.url}>
                  <SidebarMenuButton tooltip={item.title} isActive={pathname.startsWith(item.url)} render={<NavLink to={item.url} />}>
                    {item.icon}
                    <span>{item.title}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      ))}
    </>
  )
}
