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
import { deleteApplication, type Application } from "../api"

export function canDeleteApplication(application: Application) {
  return (
    ["REJECTED", "CANCELLED", "EXPIRED", "TERMINATED"].includes(application.status) ||
    (application.status === "APPROVED" && Date.now() >= Date.parse(application.endAt))
  )
}

export function DeleteApplicationButton({ application, onDone }: { application: Application; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)

  const remove = async () => {
    setPending(true)
    try {
      await deleteApplication(application.id)
      toast.success("已删除申请记录")
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
      <AlertDialogTrigger render={<Button variant="ghost" size="sm" />}>删除</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>删除这条申请记录？</AlertDialogTitle>
          <AlertDialogDescription>
            {application.roomName}，{formatRange(application.startAt, application.endAt)}。删除后不再显示在“我的申请”中，审批轨迹和审计数据仍会保留。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>保留</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={pending} onClick={remove}>
            {pending && <Spinner />}
            确认删除
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
