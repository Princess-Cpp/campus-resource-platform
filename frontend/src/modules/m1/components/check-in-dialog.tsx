import { useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { describeError } from "@/modules/common/api"
import { checkIn, type Booking } from "@/modules/m1/api"

export function CheckInDialog({ booking, onDone }: { booking: Booking; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [code, setCode] = useState("")
  const [error, setError] = useState("")
  const [submitting, setSubmitting] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError("")
    try {
      await checkIn(booking.id, code)
      toast.success("签到成功", { description: `${booking.roomName} · ${booking.seatLabel}` })
      setOpen(false)
      onDone()
    } catch (reason) {
      setError(describeError(reason))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) {
          setCode("")
          setError("")
        }
      }}
    >
      <DialogTrigger render={<Button size="sm" />}>签到</DialogTrigger>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>座位签到</DialogTitle>
            <DialogDescription>
              {booking.roomName} · {booking.seatLabel}
            </DialogDescription>
          </DialogHeader>
          <Field data-invalid={!!error || undefined}>
            <FieldLabel htmlFor="check-in-code">教室签到码</FieldLabel>
            <Input
              id="check-in-code"
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder="见教室门口的当日编码"
              autoComplete="off"
              autoFocus
              className="font-mono uppercase"
              aria-invalid={!!error || undefined}
            />
            {error ? <FieldError>{error}</FieldError> : <FieldDescription>签到码每天更新，只在预约时段的签到窗口内有效。</FieldDescription>}
          </Field>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>取消</DialogClose>
            <Button type="submit" disabled={submitting || !code.trim()}>
              {submitting && <Spinner />}
              确认签到
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
