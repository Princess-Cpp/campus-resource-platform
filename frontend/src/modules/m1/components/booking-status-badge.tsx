import { Badge } from "@/components/ui/badge"
import type { Booking } from "@/modules/m1/api"
import { bookingReason, bookingStatus } from "@/modules/m1/format"

export function BookingStatusBadge({ booking }: { booking: Booking }) {
  const status = bookingStatus(booking)
  const reason = bookingReason(booking)
  return (
    <div className="flex flex-col items-start gap-1">
      <Badge variant={status.tone}>{status.label}</Badge>
      {reason && <span className="text-xs text-muted-foreground">{reason}</span>}
    </div>
  )
}
