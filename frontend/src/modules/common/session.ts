import { useSyncExternalStore } from "react"

export type Principal = {
  id: string
  schoolNumber: string
  name: string
  department: string
  roles: string[]
  permissions: string[]
  areas: string[]
  contactRefs: string[]
}

export type SessionData = {
  actor: Principal
  csrfToken: string
  expiresAt: string
  landingPath: string
}

type SessionState = {
  loading: boolean
  data?: SessionData
  error?: string
}

let state: SessionState = { loading: true }
let loadingPromise: Promise<SessionData | undefined> | undefined
const listeners = new Set<() => void>()

function publish(next: SessionState) {
  state = next
  listeners.forEach((notify) => notify())
}

function subscribe(notify: () => void) {
  listeners.add(notify)
  return () => listeners.delete(notify)
}

function snapshot() {
  return state
}

async function readBody(response: Response) {
  return response.json().catch(() => null)
}

function authMessage(code: string) {
  const messages: Record<string, string> = {
    INVALID_INPUT: "请检查输入内容，密码长度需为 10–72 个字符",
    INVALID_CREDENTIALS: "学工号或密码错误",
    REGISTRATION_UNAVAILABLE: "该学工号已注册，请直接登录",
    RATE_LIMITED: "尝试次数过多，请 15 分钟后再试",
    CSRF_INVALID: "无法验证请求来源，请从本站页面刷新后重试",
    NETWORK_ERROR: "无法连接服务器",
  }
  return messages[code] ?? `请求失败（${code}）`
}

async function authRequest(path: string, body?: object): Promise<SessionData> {
  const response = await fetch("/api/v1/auth/" + path, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(state.data?.csrfToken ? { "X-CSRF-Token": state.data.csrfToken } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  const payload = await readBody(response)
  if (!response.ok) throw new Error(authMessage(payload?.error?.code ?? `HTTP_${response.status}`))
  const data = payload.data as SessionData
  publish({ loading: false, data })
  return data
}

export function ensureSession(force = false) {
  if (!force && !state.loading) return Promise.resolve(state.data)
  if (loadingPromise) return loadingPromise
  loadingPromise = fetch("/api/v1/auth/session", { credentials: "same-origin" })
    .then(async (response) => {
      const payload = await readBody(response)
      if (response.status === 401) {
        publish({ loading: false })
        return undefined
      }
      if (!response.ok) {
        publish({ loading: false, error: "暂时无法验证登录状态" })
        return undefined
      }
      const data = payload.data as SessionData
      publish({ loading: false, data })
      return data
    })
    .catch(() => {
      publish({ loading: false, error: "无法连接服务器" })
      return undefined
    })
    .finally(() => {
      loadingPromise = undefined
    })
  return loadingPromise
}

export function login(schoolNumber: string, password: string) {
  return authRequest("login", { schoolNumber, password })
}

export function register(schoolNumber: string, name: string, password: string, confirmPassword: string) {
  return authRequest("register", { schoolNumber, name, password, confirmPassword })
}

export async function logout() {
  let response: Response
  try {
    response = await fetch("/api/v1/auth/logout", {
      method: "POST",
      credentials: "same-origin",
      headers: state.data?.csrfToken ? { "X-CSRF-Token": state.data.csrfToken } : {},
    })
  } catch {
    throw new Error(authMessage("NETWORK_ERROR"))
  }
  // 服务端会话已过期或已失效：本地同样结束会话，调用方可进入登录页。
  if (response.status === 401) {
    publish({ loading: false })
    return
  }
  if (!response.ok) {
    const payload = await readBody(response)
    throw new Error(authMessage(payload?.error?.code ?? `HTTP_${response.status}`))
  }
  publish({ loading: false })
}

export function clearSession() {
  publish({ loading: false })
}

export function getCurrentSession() {
  return state.data
}

export function getCSRFToken() {
  return state.data?.csrfToken
}

export function useSession() {
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}
