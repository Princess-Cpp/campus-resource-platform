import assert from "node:assert/strict"
import test from "node:test"
import { normalizeReport, queryDates, ReportSubmission, statusText, validateReport } from "./reporting.ts"
import { createPreviewGateway } from "./dev/gateway.ts"
import type { ReportInput, Room } from "./types.ts"

const rooms: Room[] = [{ id: "room-a", name: "示例自习室", area: "A楼", status: "ACTIVE", seats: [{ id: "seat-a", label: "01" }] }]
const input: ReportInput = { roomId: "room-a", seatId: "seat-a", category: "LIGHTING", description: "教室前排照明灯持续闪烁无法正常使用", severity: "NORMAL" }

test("report form trims description and omits an empty optional seat", () => {
  assert.deepEqual(normalizeReport({ ...input, description: `  ${input.description}\n`, seatId: "" }), { roomId: "room-a", category: "LIGHTING", description: input.description, severity: "NORMAL" })
})
test("calendar filters use Shanghai day boundaries and reject reversed or impossible dates", () => {
  assert.deepEqual(queryDates("2026-10-22", "2026-10-22"), { from: "2026-10-21T16:00:00.000Z", to: "2026-10-22T16:00:00.000Z" })
  assert.deepEqual(queryDates("", ""), {})
  assert.throws(() => queryDates("2026-10-23", "2026-10-22"), /开始日期/)
  assert.throws(() => queryDates("2026-02-30", ""), /有效日期/)
})

test("report form counts Unicode code points and accepts the proposed description boundaries", () => {
  assert.equal(validateReport({ ...input, description: "灯".repeat(9) }, rooms).description, "请填写 10–1000 字的故障描述")
  assert.deepEqual(validateReport({ ...input, description: "💡".repeat(1000) }, rooms), {})
  assert.ok(validateReport({ ...input, description: "💡".repeat(1001) }, rooms).description)
})

test("report form rejects stale room, retired room, wrong seat and unknown options", () => {
  assert.ok(validateReport({ ...input, roomId: "missing" }, rooms).roomId)
  assert.ok(validateReport(input, [{ ...rooms[0], status: "RETIRED" }]).roomId)
  assert.ok(validateReport({ ...input, seatId: "other-room-seat" }, rooms).seatId)
  assert.ok(validateReport({ ...input, category: "UNKNOWN" as ReportInput["category"] }, rooms).category)
  assert.ok(validateReport({ ...input, severity: "UNKNOWN" as ReportInput["severity"] }, rooms).severity)
  assert.deepEqual(validateReport(input, [{ ...rooms[0], status: "DISABLED" }]), {})
})

test("submission freezes its request and reuses the same key after a lost reply", async () => {
  const gateway = createPreviewGateway({ rooms, seed: false })
  gateway.loseNextCreateReply()
  let generatedKeys = 0
  const submission = new ReportSubmission(gateway, () => `retry-key-${++generatedKeys}`)
  await assert.rejects(submission.send(input), /响应丢失/)
  assert.equal(submission.locked, true)
  const listed = await gateway.list({ page: 1, pageSize: 20 })
  assert.equal(listed.total, 1)
  const order = await submission.send(input)
  assert.equal(order.seatLabel, "01")
  assert.equal(order.id, listed.items[0].id)
  assert.equal(submission.locked, false)
  assert.equal(generatedKeys, 1)
  assert.equal((await gateway.list({ page: 1, pageSize: 20 })).total, 1)
})

test("submission refuses an edited draft while an earlier request is unresolved", async () => {
  const gateway = createPreviewGateway({ rooms, seed: false })
  gateway.loseNextCreateReply()
  const submission = new ReportSubmission(gateway, () => "original-key")
  await assert.rejects(submission.send(input))
  await assert.rejects(submission.send({ ...input, description: "更换后的另一份描述不应该被发送" }), /原报修尚待核对/)
  assert.equal((await gateway.list({ page: 1, pageSize: 20 })).total, 1)
  assert.equal((await submission.send(input)).description, input.description)
})

test("double click shares the in-flight request rather than calling create twice", async () => {
  const gateway = createPreviewGateway({ rooms, seed: false })
  const original = gateway.create
  let calls = 0
  gateway.create = async (...args) => { calls++; return original(...args) }
  const submission = new ReportSubmission(gateway, () => "one-key")
  const [a, b] = await Promise.all([submission.send(input), submission.send(input)])
  assert.equal(calls, 1)
  assert.equal(a.id, b.id)
})

test("preview gateway rejects same-key changed content and allows separate reports", async () => {
  const gateway = createPreviewGateway({ rooms, seed: false })
  const a = await gateway.create(input, "key-a")
  await assert.rejects(gateway.create({ ...input, description: "另一份真实的故障描述不应该覆盖原单" }, "key-a"), /同一请求标识/)
  const b = await gateway.create(input, "key-b")
  assert.notEqual(a.id, b.id)
  assert.equal((await gateway.list({ page: 1, pageSize: 20 })).total, 2)
})

test("preview query uses stable pages, combined filters and exclusive end time", async () => {
  const gateway = createPreviewGateway({ rooms, seed: false, now: () => new Date("2026-10-22T02:00:00Z") })
  for (let i = 0; i < 3; i++) await gateway.create(input, `key-${i}`)
  const first = await gateway.list({ page: 1, pageSize: 2 })
  const second = await gateway.list({ page: 2, pageSize: 2 })
  assert.equal(first.total, 3)
  assert.equal(second.items.length, 1)
  assert.equal(new Set([...first.items, ...second.items].map(x => x.id)).size, 3)
  assert.equal((await gateway.list({ page: 1, pageSize: 20, category: "POWER" })).total, 0)
  assert.equal((await gateway.list({ page: 1, pageSize: 20, from: "2026-10-22T02:00:00Z", to: "2026-10-22T02:00:00Z" })).total, 0)
  await assert.rejects(gateway.list({ page: 0, pageSize: 20 }), /分页/)
  await assert.rejects(gateway.list({ page: 1, pageSize: 101 }), /分页/)
})

test("preview data is isolated between instances and callers cannot mutate stored records", async () => {
  const a = createPreviewGateway({ rooms, seed: false })
  const b = createPreviewGateway({ rooms, seed: false })
  const order = await a.create(input, "a")
  order.description = "修改返回值"
  assert.equal((await a.detail(order.id)).description, input.description)
  assert.equal((await b.list({ page: 1, pageSize: 20 })).total, 0)
  await assert.rejects(b.detail(order.id), /不存在/)
})

test("pending resource state never appears as completed or fully restored", () => {
  assert.equal(statusText({ state: "ASSIGNED", syncState: "PENDING" }), "维修准备中")
  assert.equal(statusText({ state: "AWAITING_ACCEPTANCE", syncState: "PENDING" }), "恢复处理中")
  assert.equal(statusText({ state: "COMPLETED", syncState: "UNKNOWN" }), "状态待核对")
  assert.equal(statusText({ state: "COMPLETED", syncState: "IDLE" }), "已完成")
})
