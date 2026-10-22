import { ChartColumnIcon, ClipboardCheckIcon, FileTextIcon, SchoolIcon } from "lucide-react"
import type { ModuleDefinition, RouteHandle } from "@/modules/common/navigation"
import { withPermission } from "@/modules/common/require-permission"
import { ApplyPage } from "./pages/apply-page"
import { ApplicationsPage } from "./pages/applications-page"
import { StatsPage } from "./pages/stats-page"

const section = "教室申请"
const pages = [
  { path: "m2/apply", title: "申请教室", icon: <SchoolIcon />, element: <ApplyPage />, permissions: ["classroom.apply"] },
  { path: "m2/applications", title: "我的申请", icon: <FileTextIcon />, element: <ApplicationsPage />, permissions: ["classroom.apply"] },
  { path: "m2/approvals", title: "审批待办", icon: <ClipboardCheckIcon />, element: <ApplicationsPage inbox />, permissions: ["approval.department", "approval.final"] },
  { path: "m2/stats", title: "申请统计", icon: <ChartColumnIcon />, element: <StatsPage />, permissions: ["classroom.stats"] },
]
export const m2Module: ModuleDefinition = {
  id: "m2", label: section,
  nav: pages.map(p => ({ title: p.title, url: `/${p.path}`, icon: p.icon, permissions: p.permissions })),
  routes: pages.map(p => ({ path: p.path, element: withPermission(p.element, p.permissions), handle: { section, title: p.title } satisfies RouteHandle })),
}
