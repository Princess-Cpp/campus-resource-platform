import { useState } from "react"
import { toast } from "sonner"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { describeError } from "@/modules/common/api"
import { formatRange } from "@/modules/common/time"
import { cancelBooking, type Booking } from "@/modules/m1/api"

export function CancelBookingButton({ booking, onDone }: { booking: Booking; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)

  const cancel = async () => {
    setPending(true)
    try {
      await cancelBooking(booking.id)
      toast.success("已取消预约")
      setOpen(false)
      onDone()
    } catch (error) {
      toast.error("取消失败", { description: describeError(error) })
    } finally {
      setPending(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
      <AlertDialogTrigger render={<Button size="sm" variant="outline" />}>取消</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>取消这条预约？</AlertDialogTitle>
          <AlertDialogDescription>
            {booking.roomName} · {booking.seatLabel}，{formatRange(booking.startAt, booking.endAt)}。取消后座位会释放给其他同学。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>保留</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={cancel} disabled={pending}>
            {pending && <Spinner />}
            确认取消
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
