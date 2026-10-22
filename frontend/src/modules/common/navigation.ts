import type { ReactNode } from "react"
import type { RouteObject } from "react-router"

// 各模块通过 ModuleDefinition 挂载到外壳：外壳（M2 维护）只负责布局与导航，业务页面由各模块组维护。
export type NavItem = { title: string; url: string; icon: ReactNode; permissions?: string[] }

export type ModuleDefinition = {
  id: string
  label: string
  status?: "planned"
  nav: NavItem[]
  routes: RouteObject[]
}

export type RouteHandle = { section?: string; title: string }

export function hasAnyPermission(granted: readonly string[] | undefined, required?: readonly string[]) {
  if (!required || required.length === 0) return true
  return required.some((permission) => granted?.includes(permission))
}

export function visibleModules(modules: ModuleDefinition[], granted: readonly string[] | undefined): ModuleDefinition[] {
  return modules
    .map((module) => ({
      ...module,
      nav: module.nav.filter((item) => hasAnyPermission(granted, item.permissions)),
    }))
    .filter((module) => module.nav.length > 0)
}
