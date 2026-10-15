import { useNavigate } from "react-router"
import { EllipsisVerticalIcon, LogOutIcon } from "lucide-react"
import { toast } from "sonner"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "@/components/ui/sidebar"
import { describeError } from "@/modules/common/api"
import { logout, useSession } from "@/modules/common/session"

export function NavUser() {
  const { isMobile } = useSidebar()
  const navigate = useNavigate()
  const session = useSession().data
  const actor = session?.actor

  const signOut = async () => {
    try {
      await logout()
      navigate("/login", { replace: true })
    } catch (error) {
      toast.error(describeError(error))
    }
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger render={<SidebarMenuButton size="lg" className="aria-expanded:bg-muted" />}>
            <Avatar className="size-8 rounded-lg">
              <AvatarFallback className="rounded-lg">{actor?.name.slice(0, 2) || "用户"}</AvatarFallback>
            </Avatar>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{actor?.name || "已登录用户"}</span>
              <span className="truncate text-xs text-foreground/70">{actor ? `${actor.schoolNumber} · ${actor.department}` : ""}</span>
            </div>
            <EllipsisVerticalIcon className="ml-auto size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent className="min-w-56" side={isMobile ? "bottom" : "right"} align="end" sideOffset={4}>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-muted-foreground">
                {actor?.roles.join("、") || "当前账号"}
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => void signOut()}>
              <LogOutIcon />
              退出登录
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
