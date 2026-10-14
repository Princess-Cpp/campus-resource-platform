import type { ModuleDefinition } from "@/modules/common/navigation"
import { m1Module } from "@/modules/m1/module"
import { m2Module } from "@/modules/m2/module"

// 新模块（如 M3）在此注册即可获得导航与路由。
export const modules: ModuleDefinition[] = [m1Module, m2Module]
