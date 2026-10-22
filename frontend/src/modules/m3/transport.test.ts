import assert from "node:assert/strict"
import test from "node:test"
import { createHTTPGateway } from "./transport.ts"
import { pendingStore } from "./pending-store.ts"
import { ReportSubmission } from "./reporting.ts"
import { createPreviewGateway } from "./dev/gateway.ts"
import type { Order, ReportInput } from "./types.ts"

const input: ReportInput = { roomId: "r", category: "LIGHTING", description: "前排照明灯持续闪烁影响正常阅读", severity: "NORMAL" }
function storage() {
  const values = new Map<string, string>()
  return { values, getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) }, removeItem: (key: string) => { values.delete(key) } }
}
test("HTTP adapter uses documented paths, encoded identifiers and the original idempotency key", async () => {
  const calls: { path: string; init?: RequestInit }[] = []
  const gateway = createHTTPGateway(async <T>(path: string, init?: RequestInit): Promise<T> => { calls.push({ path, init }); return {} as T })
  await gateway.create(input, "original")
  assert.equal(calls[0].path, "/m3/workorders")
  assert.equal(calls[0].init?.method, "POST")
  assert.deepEqual(calls[0].init?.headers, { "Idempotency-Key": "original" })
  assert.deepEqual(JSON.parse(calls[0].init!.body as string), input)
  await gateway.list({ page: 2, pageSize: 20, category: "LIGHTING", from: "2026-10-22T00:00:00+08:00" })
  assert.equal(new URLSearchParams(calls[1].path.split("?")[1]).get("from"), "2026-10-22T00:00:00+08:00")
  await gateway.detail("id/with?reserved")
  assert.equal(calls[2].path, "/m3/workorders/id%2Fwith%3Freserved")
})
test("pending commands survive controller recreation and stay isolated by actor", async () => {
  const s = storage()
  const gateway = createPreviewGateway({ seed: false, rooms: [{ id: "r", name: "测试教室", area: "A", status: "ACTIVE", seats: [] }] })
  gateway.loseNextCreateReply()
  const first = new ReportSubmission(gateway, () => "persisted-key", pendingStore("actor-a", s))
  await assert.rejects(first.send(input))
  assert.equal(new ReportSubmission(gateway, () => "other-key", pendingStore("actor-b", s)).locked, false)
  const restored = new ReportSubmission(gateway, () => { throw new Error("must not create a new key") }, pendingStore("actor-a", s))
  assert.deepEqual(restored.draft, input)
  await restored.send(input)
  assert.equal((await gateway.list({ page: 1, pageSize: 20 })).total, 1)
  assert.equal(pendingStore("actor-a", s).load(), undefined)
})
test("corrupt or unavailable storage fails before any network write", async () => {
  const s = storage(); s.setItem("campus.m3.pending.a", "invalid json")
  let calls = 0
  const gateway = createHTTPGateway(async <T>(): Promise<T> => { calls++; return {} as T })
  const broken = new ReportSubmission(gateway, () => "key", pendingStore("a", s))
  await assert.rejects(broken.send(input), /未确认报修/)
  const unavailable = new ReportSubmission(gateway, () => "key", { load: () => undefined, save: () => { throw new Error("denied") }, clear() {} })
  await assert.rejects(unavailable.send(input), /报修尚未发送/)
  assert.equal(calls, 0)
})
test("definite rejection unlocks a new draft but never discards an earlier unknown result", async () => {
  const gateway = createHTTPGateway(async <T>(): Promise<T> => { throw Object.assign(new Error("forbidden"), { status: 403 }) })
  const fresh = new ReportSubmission(gateway, () => "key")
  await assert.rejects(fresh.send(input)); assert.equal(fresh.locked, false)
  let attempt = 0
  gateway.create = async (): Promise<Order> => { if (attempt++ === 0) throw new Error("network"); throw Object.assign(new Error("forbidden"), { status: 403 }) }
  const uncertain = new ReportSubmission(gateway, () => "key")
  await assert.rejects(uncertain.send(input)); await assert.rejects(uncertain.send(input))
  assert.equal(uncertain.locked, true)
})
