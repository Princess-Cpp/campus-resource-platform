import { useState, type FormEvent } from "react"
import { EllipsisVerticalIcon, KeyRoundIcon } from "lucide-react"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "@/components/ui/sidebar"
import { KNOWN_DEV_TOKENS, setDevToken, useDevToken } from "@/modules/common/session"

// 登录由 M2（D-01 身份/RBAC）交付前，这里只切换开发 fixture 令牌，不代表真实登录。
export function NavUser() {
  const { isMobile } = useSidebar()
  const token = useDevToken()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(token)

  const save = (event: FormEvent) => {
    event.preventDefault()
    if (!draft.trim()) return
    setDevToken(draft.trim())
    setOpen(false)
    // 身份变化后重新加载，确保所有页面数据按新身份获取。
    window.location.reload()
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger render={<SidebarMenuButton size="lg" className="aria-expanded:bg-muted" />}>
            <Avatar className="size-8 rounded-lg">
              <AvatarFallback className="rounded-lg">{token.slice(0, 2).toUpperCase()}</AvatarFallback>
            </Avatar>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{token}</span>
              <span className="truncate text-xs text-foreground/70">开发身份</span>
            </div>
            <EllipsisVerticalIcon className="ml-auto size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent className="min-w-56" side={isMobile ? "bottom" : "right"} align="end" sideOffset={4}>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-muted-foreground">M1_DEV_MODE 令牌，正式登录由 M2 提供</DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => {
                setDraft(token)
                setOpen(true)
              }}
            >
              <KeyRoundIcon />
              切换开发令牌
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent>
            <form onSubmit={save} className="grid gap-4">
              <DialogHeader>
                <DialogTitle>切换开发令牌</DialogTitle>
                <DialogDescription>令牌需在后端 dev fixture 中配置，仅用于本地开发。</DialogDescription>
              </DialogHeader>
              <Field>
                <FieldLabel htmlFor="dev-token">令牌</FieldLabel>
                <Input id="dev-token" value={draft} onChange={(e) => setDraft(e.target.value)} autoComplete="off" className="font-mono" />
                <FieldDescription>
                  示例：
                  {KNOWN_DEV_TOKENS.map((t) => (
                    <button key={t} type="button" className="mx-1 font-mono underline underline-offset-4" onClick={() => setDraft(t)}>
                      {t}
                    </button>
                  ))}
                </FieldDescription>
              </Field>
              <DialogFooter>
                <Button type="submit">保存并刷新</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
