import type { PendingReport, PendingStore } from "./reporting.ts"
import { normalizeReport } from "./reporting.ts"

export function pendingStore(actorId: string, storage: Pick<Storage, "getItem" | "setItem" | "removeItem">): PendingStore {
  if (!actorId) throw new Error("无法识别当前登录用户，请重新登录")
  const name = "campus.m3.pending." + actorId
  return {
    load() {
      const raw = storage.getItem(name)
      if (!raw) return undefined
      const value: unknown = JSON.parse(raw)
      if (!value || typeof value !== "object" || !("key" in value) || typeof value.key !== "string" || !/^[A-Za-z0-9._:-]{1,128}$/.test(value.key) || !("input" in value) || !value.input || typeof value.input !== "object") throw new Error("未确认报修记录无法读取，请保留页面并联系维护人员核对")
      const input = value.input as Record<string, unknown>
      if (typeof input.roomId !== "string" || typeof input.description !== "string" || typeof input.category !== "string" || typeof input.severity !== "string" || (input.seatId !== undefined && typeof input.seatId !== "string")) throw new Error("未确认报修记录不完整，请保留页面并核对")
      return { key: value.key, input: normalizeReport(input as PendingReport["input"]) }
    },
    save(value) { storage.setItem(name, JSON.stringify(value)) },
    clear() { storage.removeItem(name) },
  }
}
