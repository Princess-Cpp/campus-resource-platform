import assert from "node:assert/strict"
import test from "node:test"
import { canOpenMaintenancePage, visibleMaintenanceItems } from "./navigation.ts"
import type { MaintenanceSession } from "./transport.ts"

const urls = ["notifications", "escalations", "stats", "dispatch", "acceptance", "assigned", "report", "workorders"]
const items = urls.map(name => ({ url: `/m3/${name}`, title: name }))
const session = (permissions: string[], id = "current"): MaintenanceSession => ({ actor: { id, permissions }, development: true })
const visible = (permissions: string[]) => visibleMaintenanceItems(items, "current", session(permissions), false, undefined).map(item => item.title)

test("direct workbench URLs require their own permission, without administrator inheritance", () => {
  for (const page of ["dispatch", "assigned", "acceptance", "stats", "escalations"] as const) {
    assert.equal(canOpenMaintenancePage(page, ["workorder.report"]), false)
    assert.equal(canOpenMaintenancePage(page, ["account.manage"]), false)
  }
  assert.equal(canOpenMaintenancePage("dispatch", ["workorder.dispatch"]), true)
  assert.equal(canOpenMaintenancePage("acceptance", ["workorder.dispatch"]), false)
  assert.equal(canOpenMaintenancePage("report", []), false)
  assert.equal(canOpenMaintenancePage("detail", []), true)
})

test("student sees reporting and own history/messages, without staff entries", () => {
  assert.deepEqual(visible(["workorder.report"]), ["notifications", "report", "workorders"])
})
test("each staff permission reveals only its corresponding workbench", () => {
  for (const [permission, title] of [["dispatch", "dispatch"], ["repair", "assigned"], ["accept", "acceptance"], ["stats", "stats"], ["supervise", "escalations"], ["escalation.read", "escalations"]]) {
    assert.deepEqual(visible([`workorder.${permission}`]), ["notifications", title, "workorders"])
  }
})
test("combined permissions compose and unknown entries stay hidden", () => {
  assert.deepEqual(visible(["workorder.dispatch", "workorder.accept"]), ["notifications", "dispatch", "acceptance", "workorders"])
  assert.deepEqual(visibleMaintenanceItems([{ url: "/m3/new-admin" }], "current", session(["*"]), false, undefined), [])
})
test("loading, failure, logout and a stale account response hide all M3 links", () => {
  const old = session(["workorder.dispatch"], "old")
  for (const [actorId, current, loading, error] of [
    ["current", session(["workorder.dispatch"]), true, undefined],
    ["current", session(["workorder.dispatch"]), false, new Error("offline")],
    [undefined, old, false, undefined],
    ["current", old, false, undefined],
    ["current", undefined, false, undefined],
  ] as const) assert.deepEqual(visibleMaintenanceItems(items, actorId, current, loading, error), [])
})
test("revoked permission disappears and filtering preserves the original navigation", () => {
  const before = JSON.stringify(items)
  assert.ok(visible(["workorder.dispatch"]).includes("dispatch"))
  assert.deepEqual(visible([]), ["notifications", "workorders"])
  assert.equal(JSON.stringify(items), before)
})
