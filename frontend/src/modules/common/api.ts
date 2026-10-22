import { clearSession, getCSRFToken } from "@/modules/common/session"

// 统一 API 客户端：后端成功返回 {data, correlationId}，失败返回 {error: {code}, correlationId}。
export class ApiError extends Error {
  readonly code: string
  readonly status: number
  readonly correlationId?: string

  constructor(code: string, status: number, correlationId?: string) {
    super(errorMessage(code))
    this.code = code
    this.status = status
    this.correlationId = correlationId
  }
}

const MESSAGES: Record<string, string> = {
  UNAUTHENTICATED: "登录已失效，请重新登录",
  FORBIDDEN: "没有权限执行该操作",
  INVALID_INPUT: "输入不符合要求，请检查时间和参数",
  NOT_FOUND: "记录不存在或无权查看",
  RESOURCE_UNAVAILABLE: "座位当前不可用（未开放、已下架或无权预约）",
  RESOURCE_CONFLICT: "该时段已被占用，请换一个座位或时段",
  CLOSED: "所选时段不在自习室开放时间内",
  IDEMPOTENCY_CONFLICT: "重复的请求，请刷新后重试",
  INVALID_IDEMPOTENCY_KEY: "请求标识无效，请重试",
  INVALID_STATE: "当前状态或时间不允许该操作",
  INVALID_CODE: "签到码不正确",
  CSRF_INVALID: "页面会话已失效，请刷新后重试",
  RATE_LIMITED: "尝试次数过多，请稍后再试",
  VERSION_CONFLICT: "数据已变化，请刷新后重试",
  DEPENDENCY_UNAVAILABLE: "场地服务暂不可用，请稍后核对或重试原操作",
  OPERATION_PENDING: "场地操作尚未确认，请刷新后查看进度",
  DEADLINE_EXPIRED: "审批期限已到，不能继续批准",
  APPROVERS_UNAVAILABLE: "缺少符合条件的审批人，请联系管理员配置",
  INTERNAL_ERROR: "服务器内部错误，请稍后重试",
  NETWORK_ERROR: "无法连接服务器",
}

export function errorMessage(code: string) {
  return MESSAGES[code] ?? `请求失败（${code}）`
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  let response: Response
  try {
    response = await fetch("/api/v1" + path, {
      ...init,
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        ...(!["GET", "HEAD", "OPTIONS"].includes((init.method ?? "GET").toUpperCase()) && getCSRFToken()
          ? { "X-CSRF-Token": getCSRFToken()! }
          : {}),
        ...init.headers,
      },
    })
  } catch {
    throw new ApiError("NETWORK_ERROR", 0)
  }
  const body = await response.json().catch(() => null)
  if (!response.ok) {
    if (response.status === 401) clearSession()
    throw new ApiError(body?.error?.code ?? `HTTP_${response.status}`, response.status, body?.correlationId)
  }
  return body?.data as T
}

export function describeError(error: unknown) {
  if (error instanceof ApiError) return error.message
  return error instanceof Error ? error.message : String(error)
}
