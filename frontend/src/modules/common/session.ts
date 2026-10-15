import { useSyncExternalStore } from "react"

// 开发期身份：后端 M1_DEV_MODE 用 fixture 令牌认证。M2 交付 D-01 登录后替换本文件，
// 页面只依赖 useDevToken/getDevToken，不直接读写 localStorage。
const STORAGE_KEY = "campus.devToken"
export const DEFAULT_DEV_TOKEN = "student-demo"
export const KNOWN_DEV_TOKENS = ["student-demo", "department-demo", "venue-demo", "operator-demo"]

const listeners = new Set<() => void>()

export function getDevToken(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) || DEFAULT_DEV_TOKEN
  } catch {
    return DEFAULT_DEV_TOKEN
  }
}

export function setDevToken(token: string) {
  try {
    localStorage.setItem(STORAGE_KEY, token)
  } catch {
    // 隐私模式下无法持久化，仍通知订阅者以便本次会话生效。
  }
  listeners.forEach((notify) => notify())
}

function subscribe(notify: () => void) {
  listeners.add(notify)
  return () => listeners.delete(notify)
}

export function useDevToken() {
  return useSyncExternalStore(subscribe, getDevToken)
}
