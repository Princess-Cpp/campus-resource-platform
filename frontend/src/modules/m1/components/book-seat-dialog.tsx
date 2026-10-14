import { useState } from "react"
import { toast } from "sonner"
import { PlugZapIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
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
import { Spinner } from "@/components/ui/spinner"
import { describeError } from "@/modules/common/api"
import { formatRange } from "@/modules/common/time"
import { createBooking, type AvailableSeat } from "@/modules/m1/api"

export function BookSeatDialog({ item, startAt, endAt, onBooked }: { item: AvailableSeat; startAt: Date; endAt: Date; onBooked: () => void }) {
  const [open, setOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [key, setKey] = useState("")

  const submit = async () => {
    setSubmitting(true)
    try {
      await createBooking({ roomId: item.room.id, seatId: item.seat.id, startAt, endAt }, key)
      toast.success("预约成功", { description: `${item.room.name} · ${item.seat.label}，请在开始后 15 分钟内签到` })
      setOpen(false)
      onBooked()
    } catch (error) {
      toast.error("预约失败", { description: describeError(error) })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        // 每次打开对话框生成新的幂等键；同一次确认中的重试复用该键。
        if (next) setKey(crypto.randomUUID())
        setOpen(next)
      }}
    >
      <DialogTrigger render={<Button size="sm" />}>预约</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>确认预约</DialogTitle>
          <DialogDescription>提交时服务端会重新校验开放时间、访问权限和占用情况。</DialogDescription>
        </DialogHeader>
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
          <dt className="text-muted-foreground">自习室</dt>
          <dd>
            {item.room.name}
            {item.room.area && <span className="text-muted-foreground"> · {item.room.area}</span>}
          </dd>
          <dt className="text-muted-foreground">座位</dt>
          <dd className="flex items-center gap-2">
            {item.seat.label}
            {item.seat.outlet && (
              <Badge variant="outline">
                <PlugZapIcon />
                插座
              </Badge>
            )}
          </dd>
          <dt className="text-muted-foreground">时段</dt>
          <dd>{formatRange(startAt.toISOString(), endAt.toISOString())}</dd>
        </dl>
        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>取消</DialogClose>
          <Button onClick={submit} disabled={submitting}>
            {submitting && <Spinner />}
            确认预约
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
