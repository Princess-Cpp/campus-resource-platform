import { ArmchairIcon, BellIcon, CalendarCheckIcon } from "lucide-react"

import type { ModuleDefinition, RouteHandle } from "@/modules/common/navigation"
import { BookingsPage } from "@/modules/m1/pages/bookings-page"
import { NotificationsPage } from "@/modules/m1/pages/notifications-page"
import { SearchPage } from "@/modules/m1/pages/search-page"

const section = "座位预约"

export const m1Module: ModuleDefinition = {
  id: "m1",
  label: section,
  nav: [
    { title: "查找座位", url: "/m1/search", icon: <ArmchairIcon /> },
    { title: "我的预约", url: "/m1/bookings", icon: <CalendarCheckIcon /> },
    { title: "通知记录", url: "/m1/notifications", icon: <BellIcon /> },
  ],
  routes: [
    { path: "m1/search", element: <SearchPage />, handle: { section, title: "查找座位" } satisfies RouteHandle },
    { path: "m1/bookings", element: <BookingsPage />, handle: { section, title: "我的预约" } satisfies RouteHandle },
    { path: "m1/notifications", element: <NotificationsPage />, handle: { section, title: "通知记录" } satisfies RouteHandle },
  ],
}
