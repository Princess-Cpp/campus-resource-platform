import { ArmchairIcon, BellIcon, CalendarCheckIcon } from "lucide-react"

import type { ModuleDefinition, RouteHandle } from "@/modules/common/navigation"
import { withPermission } from "@/modules/common/require-permission"
import { BookingsPage } from "@/modules/m1/pages/bookings-page"
import { NotificationsPage } from "@/modules/m1/pages/notifications-page"
import { SearchPage } from "@/modules/m1/pages/search-page"

const section = "座位预约"
const booking = ["booking.create"]
const pages = [
  { path: "m1/search", title: "查找座位", icon: <ArmchairIcon />, element: <SearchPage />, permissions: booking },
  { path: "m1/bookings", title: "我的预约", icon: <CalendarCheckIcon />, element: <BookingsPage />, permissions: booking },
  { path: "m1/notifications", title: "通知记录", icon: <BellIcon />, element: <NotificationsPage />, permissions: booking },
]

export const m1Module: ModuleDefinition = {
  id: "m1",
  label: section,
  nav: pages.map((p) => ({ title: p.title, url: `/${p.path}`, icon: p.icon, permissions: p.permissions })),
  routes: pages.map((p) => ({
    path: p.path,
    element: withPermission(p.element, p.permissions),
    handle: { section, title: p.title } satisfies RouteHandle,
  })),
}
