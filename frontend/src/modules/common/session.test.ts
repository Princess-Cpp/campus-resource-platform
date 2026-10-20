import assert from "node:assert/strict"
import { afterEach, test } from "node:test"

import { ensureSession, getCurrentSession, login, logout } from "./session.ts"

const sessionBody = {
  data: {
    actor: {
      id: "student-1",
      schoolNumber: "20260001",
      name: "演示学生",
      department: "CS",
      roles: ["student"],
      permissions: ["booking.create"],
      areas: [],
      contactRefs: ["campus-email"],
    },
    csrfToken: "csrf-token",
    expiresAt: "2030-01-10T12:00:00.000Z",
    landingPath: "/m1/search",
  },
}

function jsonResponse(status: number, body: unknown = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

afterEach(() => {
  globalThis.fetch = originalFetch
})

const originalFetch = globalThis.fetch

async function establishSession() {
  globalThis.fetch = async (input) => {
    assert.equal(String(input), "/api/v1/auth/login")
    return jsonResponse(200, sessionBody)
  }
  await login("20260001", "student-pass-1")
  assert.equal(getCurrentSession()?.actor.id, "student-1")
}

test("logout 401 clears cached session so ensureSession does not reuse it", async () => {
  await establishSession()
  const calls: string[] = []
  globalThis.fetch = async (input) => {
    const url = String(input)
    calls.push(url)
    if (url.endsWith("/auth/logout")) return jsonResponse(401, { error: { code: "UNAUTHENTICATED" } })
    throw new Error("ensureSession must not refetch after a completed logout")
  }
  await logout()
  assert.equal(getCurrentSession(), undefined)
  assert.equal(await ensureSession(), undefined)
  assert.deepEqual(calls, ["/api/v1/auth/logout"])
})

test("logout network and non-401 errors keep the local session", async () => {
  await establishSession()
  globalThis.fetch = async () => {
    throw new TypeError("failed to fetch")
  }
  await assert.rejects(logout)
  assert.equal(getCurrentSession()?.actor.id, "student-1")

  globalThis.fetch = async () => jsonResponse(503, { error: { code: "INTERNAL_ERROR" } })
  await assert.rejects(logout)
  assert.equal(getCurrentSession()?.actor.id, "student-1")
})
