import type { ReactNode } from "react"
import type { RouteObject } from "react-router"

// 各模块通过 ModuleDefinition 挂载到外壳：外壳（M2 维护）只负责布局与导航，业务页面由各模块组维护。
export type NavItem = { title: string; url: string; icon: ReactNode }

export type ModuleDefinition = {
  id: string
  label: string
  status?: "planned"
  nav: NavItem[]
  routes: RouteObject[]
}

export type RouteHandle = { section?: string; title: string }
