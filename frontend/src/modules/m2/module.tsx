import { ChartColumnIcon, ClipboardCheckIcon, FileTextIcon, SchoolIcon } from "lucide-react"
import type { ModuleDefinition, RouteHandle } from "@/modules/common/navigation"
import { ApplyPage } from "./pages/apply-page"
import { ApplicationsPage } from "./pages/applications-page"
import { StatsPage } from "./pages/stats-page"

const section = "教室申请"
const pages = [
  { path: "m2/apply", title: "申请教室", icon: <SchoolIcon />, element: <ApplyPage /> },
  { path: "m2/applications", title: "我的申请", icon: <FileTextIcon />, element: <ApplicationsPage /> },
  { path: "m2/approvals", title: "审批待办", icon: <ClipboardCheckIcon />, element: <ApplicationsPage inbox /> },
  { path: "m2/stats", title: "申请统计", icon: <ChartColumnIcon />, element: <StatsPage /> },
]
export const m2Module: ModuleDefinition = {
  id: "m2", label: section,
  nav: pages.map(p => ({ title: p.title, url: `/${p.path}`, icon: p.icon })),
  routes: pages.map(p => ({ path: p.path, element: p.element, handle: { section, title: p.title } satisfies RouteHandle })),
}
