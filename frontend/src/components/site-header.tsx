import { Fragment } from "react"
import { useMatches } from "react-router"

import { ModeToggle } from "@/components/mode-toggle"
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from "@/components/ui/breadcrumb"
import { Separator } from "@/components/ui/separator"
import { SidebarTrigger } from "@/components/ui/sidebar"
import type { RouteHandle } from "@/modules/common/navigation"

export function SiteHeader() {
  const handle = useMatches().findLast((m) => (m.handle as RouteHandle | undefined)?.title)?.handle as RouteHandle | undefined
  const crumbs = [handle?.section, handle?.title ?? "工作台"].filter(Boolean)
  return (
    <header className="flex h-(--header-height) shrink-0 items-center gap-2 border-b transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-(--header-height)">
      <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
        <SidebarTrigger className="-ml-1" />
        <Separator orientation="vertical" className="mx-2 h-4 data-vertical:self-auto" />
        <Breadcrumb>
          <BreadcrumbList>
            {crumbs.map((crumb, i) => (
              <Fragment key={crumb}>
                {i > 0 && <BreadcrumbSeparator />}
                <BreadcrumbItem className={i === crumbs.length - 1 ? "text-base font-medium" : undefined}>
                  {i === crumbs.length - 1 ? <BreadcrumbPage>{crumb}</BreadcrumbPage> : crumb}
                </BreadcrumbItem>
              </Fragment>
            ))}
          </BreadcrumbList>
        </Breadcrumb>
        <div className="ml-auto">
          <ModeToggle />
        </div>
      </div>
    </header>
  )
}
