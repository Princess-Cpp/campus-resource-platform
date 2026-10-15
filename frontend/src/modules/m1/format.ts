import type { Booking } from "@/modules/m1/api"

type Tone = "default" | "secondary" | "outline" | "destructive"

const STATUS: Record<string, { label: string; tone: Tone }> = {
  RESERVED: { label: "待签到", tone: "default" },
  CHECKED_IN: { label: "使用中", tone: "secondary" },
  FINISHED: { label: "已完成", tone: "outline" },
  CANCELLED: { label: "已取消", tone: "outline" },
}

const REASONS: Record<string, string> = {
  USER_CANCELLED: "本人取消",
  NO_SHOW: "未签到自动取消",
  MAINTENANCE: "维修下架",
}

export function bookingStatus(booking: Booking) {
  const base = STATUS[booking.status] ?? { label: booking.status, tone: "outline" as Tone }
  if (booking.status === "CANCELLED" && booking.reason === "NO_SHOW") return { ...base, label: "违约", tone: "destructive" as Tone }
  return base
}

export function bookingReason(booking: Booking) {
  return booking.reason ? (REASONS[booking.reason] ?? booking.reason) : ""
}

export const NOTIFICATION_KINDS: Record<string, string> = {
  CREATED: "预约成功",
  CANCELLED: "预约取消",
  PRE_REMINDER: "开始前提醒",
  LATE_REMINDER: "迟到提醒",
}

// SUPPRESSED：提醒发出前预约已签到或取消，后端不再投递。
export const DELIVERY_STATUS: Record<string, { label: string; tone: "default" | "secondary" | "outline" }> = {
  PENDING: { label: "待投递", tone: "secondary" },
  DELIVERED: { label: "已投递", tone: "default" },
  SUPPRESSED: { label: "已忽略", tone: "outline" },
}

export function isUsed(booking: Booking) {
  return booking.status === "CHECKED_IN" || booking.status === "FINISHED"
}

export function isNoShow(booking: Booking) {
  return booking.status === "CANCELLED" && booking.reason === "NO_SHOW"
}

// 违约记录需保留以便申诉，与服务端规则一致。
export function canDelete(booking: Booking) {
  return booking.status === "FINISHED" || (booking.status === "CANCELLED" && !isNoShow(booking))
}
