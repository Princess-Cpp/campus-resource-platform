import { ChartColumnIcon, ClipboardCheckIcon, FileTextIcon, SchoolIcon } from "lucide-react"

import type { ModuleDefinition, RouteHandle } from "@/modules/common/navigation"
import { PlannedModulePage, type PlannedPage } from "@/modules/m2/pages/planned-page"

const section = "教室申请"

// 页面清单对应 m2.md“状态与界面”：教室搜索与申请表、本人申请/详情、分步骤审批待办、运营统计。
const pages: (PlannedPage & { path: string })[] = [
  {
    path: "m2/apply",
    title: "申请教室",
    description: "按区域、容量、日期和时间搜索整间教室，填写活动用途并提交申请。",
    icon: <SchoolIcon />,
    requirements: ["M2-01.1", "M2-01.2"],
    capabilities: ["申请权限单独授予，可按角色和院系限定", "填写起止时间、用途、预计人数，人数不超过容量", "待审批申请不占位，搜索结果不保证最终批准"],
  },
  {
    path: "m2/applications",
    title: "我的申请",
    description: "查看本人申请的状态、审批轨迹与失败原因，开始前可撤销。",
    icon: <FileTextIcon />,
    requirements: ["M2-01.5", "M2-02.1", "M2-02.2"],
    capabilities: ["按状态、日期查看本人申请与审批步骤、截止时间、驳回理由", "展示“场地确认中”、可重试与终态", "开始前可撤销；到期仍未完成审批自动过期"],
  },
  {
    path: "m2/approvals",
    title: "审批待办",
    description: "院系审批人与场地审批人按步骤处理管辖范围内的申请。",
    icon: <ClipboardCheckIcon />,
    requirements: ["M2-01.3", "M2-01.4"],
    capabilities: ["第一级：本院系且非本人申请；最终审批：管辖场地且非本人申请", "最终批准经 M1 资源接口确认整室区间无占用后才显示已批准", "驳回必须填写理由，全过程记录审批轨迹"],
  },
  {
    path: "m2/stats",
    title: "申请统计",
    description: "运营管理员查看脱敏后的申请与审批统计。",
    icon: <ChartColumnIcon />,
    requirements: ["M2-01.5"],
    capabilities: ["按空间、日期统计申请数、通过率和审批耗时", "运营角色只看授权范围内的脱敏统计"],
  },
]

export const m2Module: ModuleDefinition = {
  id: "m2",
  label: section,
  status: "planned",
  nav: pages.map((p) => ({ title: p.title, url: `/${p.path}`, icon: p.icon })),
  routes: pages.map((p) => ({
    path: p.path,
    element: <PlannedModulePage page={p} />,
    handle: { section, title: p.title } satisfies RouteHandle,
  })),
}
