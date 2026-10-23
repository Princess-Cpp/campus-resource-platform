import assert from "node:assert/strict"
import { test } from "node:test"

import { hasAnyPermission, visibleModules, type ModuleDefinition } from "./navigation.ts"

const modules: ModuleDefinition[] = [
  {
    id: "m1",
    label: "座位预约",
    nav: [
      { title: "查找座位", url: "/m1/search", icon: null, permissions: ["booking.create"] },
      { title: "我的预约", url: "/m1/bookings", icon: null, permissions: ["booking.create"] },
    ],
    routes: [],
  },
  {
    id: "m2",
    label: "教室申请",
    nav: [
      { title: "申请教室", url: "/m2/apply", icon: null, permissions: ["classroom.apply"] },
      { title: "我的申请", url: "/m2/applications", icon: null, permissions: ["classroom.apply"] },
      { title: "审批待办", url: "/m2/approvals", icon: null, permissions: ["approval.department", "approval.final"] },
      { title: "申请统计", url: "/m2/stats", icon: null, permissions: ["classroom.stats"] },
    ],
    routes: [],
  },
]

test("hasAnyPermission allows unrestricted items and any matching permission", () => {
  assert.equal(hasAnyPermission(["booking.create"]), true)
  assert.equal(hasAnyPermission(["booking.create"], []), true)
  assert.equal(hasAnyPermission(["booking.create"], ["classroom.stats"]), false)
  assert.equal(hasAnyPermission(["approval.final"], ["approval.department", "approval.final"]), true)
})

test("students only see booking and classroom apply entries", () => {
  const visible = visibleModules(modules, ["booking.create", "classroom.apply"])
  assert.deepEqual(
    visible.map((module) => ({ id: module.id, titles: module.nav.map((item) => item.title) })),
    [
      { id: "m1", titles: ["查找座位", "我的预约"] },
      { id: "m2", titles: ["申请教室", "我的申请"] },
    ],
  )
})

test("operators only see stats and approvers only see inbox", () => {
  assert.deepEqual(
    visibleModules(modules, ["classroom.stats"]).map((module) => module.nav.map((item) => item.title)),
    [["申请统计"]],
  )
  assert.deepEqual(
    visibleModules(modules, ["approval.department"]).map((module) => module.nav.map((item) => item.title)),
    [["审批待办"]],
  )
  assert.deepEqual(visibleModules(modules, []), [])
})
