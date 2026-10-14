// 业务规则按校区时区计算（后端 M1 使用 Asia/Shanghai），前端显式带 +08:00，不依赖浏览器时区。
export const CAMPUS_TIME_ZONE = "Asia/Shanghai"
const OFFSET = "+08:00"

export function campusToday(now = new Date()) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: CAMPUS_TIME_ZONE }).format(now)
}

export function campusHour(now = new Date()) {
  return Number(new Intl.DateTimeFormat("en-GB", { timeZone: CAMPUS_TIME_ZONE, hour: "2-digit", hourCycle: "h23" }).format(now))
}

export function campusInstant(date: string, hour: number) {
  return new Date(`${date}T${String(hour).padStart(2, "0")}:00:00${OFFSET}`)
}

export function campusDay(value: string | Date) {
  return campusToday(new Date(value))
}

const dateTime = new Intl.DateTimeFormat("zh-CN", {
  timeZone: CAMPUS_TIME_ZONE,
  month: "2-digit",
  day: "2-digit",
  weekday: "short",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
})
const clock = new Intl.DateTimeFormat("zh-CN", { timeZone: CAMPUS_TIME_ZONE, hour: "2-digit", minute: "2-digit", hourCycle: "h23" })

export function formatDateTime(value: string | Date) {
  return dateTime.format(new Date(value))
}

export function formatRange(start: string, end: string) {
  return `${dateTime.format(new Date(start))} – ${clock.format(new Date(end))}`
}

export function hoursBetween(start: string, end: string) {
  return (new Date(end).getTime() - new Date(start).getTime()) / 3_600_000
}
