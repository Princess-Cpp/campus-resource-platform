import { ClipboardListIcon, WrenchIcon } from "lucide-react"
import type { ModuleDefinition, RouteHandle } from "@/modules/common/navigation"
import { ReportSubmission } from "./reporting"
import type { MaintenanceGateway } from "./types"
import { ReportPage } from "./pages/report-page"
import { OrdersPage } from "./pages/orders-page"
import { DetailPage } from "./pages/detail-page"
import { LiveMaintenance } from "./live"

// Injected factory for the isolated development preview; the app uses m3Module below.
export function createM3Module(gateway: MaintenanceGateway, submission: ReportSubmission): ModuleDefinition {
  const section = "物业报修"
  return {
    id: "m3", label: section,
    nav: [
      { title: "故障报修", url: "/m3/report", icon: <WrenchIcon /> },
      { title: "我的报修", url: "/m3/workorders", icon: <ClipboardListIcon /> },
    ],
    routes: [
      { path: "m3/report", element: <ReportPage gateway={gateway} submission={submission} />, handle: { section, title: "故障报修" } satisfies RouteHandle },
      { path: "m3/workorders", element: <OrdersPage gateway={gateway} />, handle: { section, title: "我的报修" } satisfies RouteHandle },
      { path: "m3/workorders/:id", element: <DetailPage gateway={gateway} />, handle: { section, title: "报修详情" } satisfies RouteHandle },
    ],
  }
}

export const m3Module: ModuleDefinition = {
  id: "m3", label: "物业报修",
  nav: [
    { title:"区域调度台",url:"/m3/dispatch",icon:<ClipboardListIcon/> },
    { title:"维修待办",url:"/m3/assigned",icon:<WrenchIcon/> },
    { title: "故障报修", url: "/m3/report", icon: <WrenchIcon /> },
    { title: "我的报修", url: "/m3/workorders", icon: <ClipboardListIcon /> },
  ],
  routes: [
    {path:"m3/dispatch",element:<LiveMaintenance page="dispatch"/>,handle:{section:"物业报修",title:"区域调度台"} satisfies RouteHandle},
    {path:"m3/assigned",element:<LiveMaintenance page="assigned"/>,handle:{section:"物业报修",title:"维修待办"} satisfies RouteHandle},
    { path: "m3/report", element: <LiveMaintenance page="report" />, handle: { section: "物业报修", title: "故障报修" } satisfies RouteHandle },
    { path: "m3/workorders", element: <LiveMaintenance page="orders" />, handle: { section: "物业报修", title: "我的报修" } satisfies RouteHandle },
    { path: "m3/workorders/:id", element: <LiveMaintenance page="detail" />, handle: { section: "物业报修", title: "报修详情" } satisfies RouteHandle },
  ],
}
