import { request } from "@/modules/common/api"

export type Seat = { id: string; label: string; outlet: boolean; active: boolean }
export type Room = { id: string; name: string; area: string; department: string; active: boolean; openHour: number; closeHour: number }
export type AvailableSeat = { room: Room; seat: Seat }

export type BookingStatus = "RESERVED" | "CHECKED_IN" | "FINISHED" | "CANCELLED"
export type Booking = {
  id: string
  roomId: string
  seatId: string
  roomName: string
  seatLabel: string
  startAt: string
  endAt: string
  status: BookingStatus | string
  reason: string
}

export type Notification = { eventId: string; bookingId: string; kind: string; status: string; createdAt: string }

export function searchSeats(params: { startAt: Date; endAt: Date; area?: string; outlet?: boolean }) {
  const query = new URLSearchParams({ startAt: params.startAt.toISOString(), endAt: params.endAt.toISOString() })
  if (params.area) query.set("area", params.area)
  if (params.outlet) query.set("outlet", "true")
  return request<AvailableSeat[] | null>(`/resources/availability?${query}`).then((rows) => rows ?? [])
}

// 幂等键由调用方在一次“确认预约”操作内保持不变，网络重试不会产生重复预约。
export function createBooking(input: { roomId: string; seatId: string; startAt: Date; endAt: Date }, idempotencyKey: string) {
  return request<Booking>("/m1/bookings", {
    method: "POST",
    headers: { "Idempotency-Key": idempotencyKey },
    body: JSON.stringify({ ...input, startAt: input.startAt.toISOString(), endAt: input.endAt.toISOString() }),
  })
}

export const HISTORY_PAGE_SIZE = 20

export function listBookings(page = 1, pageSize = HISTORY_PAGE_SIZE) {
  return request<Booking[] | null>(`/m1/bookings?page=${page}&pageSize=${pageSize}`).then((rows) => rows ?? [])
}

export function checkIn(bookingId: string, code: string) {
  return request<Booking>(`/m1/bookings/${encodeURIComponent(bookingId)}/check-in`, {
    method: "POST",
    body: JSON.stringify({ code: code.trim().toUpperCase() }),
  })
}

export function cancelBooking(bookingId: string) {
  return request<Booking>(`/m1/bookings/${encodeURIComponent(bookingId)}/cancel`, { method: "POST" })
}

export function listNotifications() {
  return request<Notification[] | null>("/m1/notifications").then((rows) => rows ?? [])
}
