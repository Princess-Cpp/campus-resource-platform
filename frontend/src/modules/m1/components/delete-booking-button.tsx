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
import { deleteBooking, type Booking } from "@/modules/m1/api"

export function DeleteBookingButton({ booking, onDone }: { booking: Booking; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)

  const remove = async () => {
    setPending(true)
    try {
      await deleteBooking(booking.id)
      toast.success("已删除记录")
      setOpen(false)
      onDone()
    } catch (error) {
      toast.error("删除失败", { description: describeError(error) })
    } finally {
      setPending(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
      <AlertDialogTrigger render={<Button size="sm" variant="ghost" />}>删除</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>删除这条记录？</AlertDialogTitle>
          <AlertDialogDescription>
            {booking.roomName} · {booking.seatLabel}，{formatRange(booking.startAt, booking.endAt)}。删除后不再显示在“我的预约”中，系统仍会保留审计记录。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>保留</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={remove} disabled={pending}>
            {pending && <Spinner />}
            确认删除
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
