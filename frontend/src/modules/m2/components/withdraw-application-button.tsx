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
import { act, type Application } from "../api"

export function canWithdrawApplication(application: Application) {
  return (
    ["SUBMITTED", "DEPARTMENT_APPROVED"].includes(application.status) &&
    application.syncStatus === "IDLE" &&
    Date.now() < Date.parse(application.startAt)
  )
}

export function WithdrawApplicationButton({ application, onDone }: { application: Application; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [key] = useState(() => crypto.randomUUID())

  const withdraw = async () => {
    setPending(true)
    try {
      await act(application.id, { action: "CANCEL", expectedVersion: application.version, comment: "" }, key)
      toast.success("申请已撤回")
      setOpen(false)
      onDone()
    } catch (error) {
      toast.error("撤回失败", { description: describeError(error) })
    } finally {
      setPending(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
      <AlertDialogTrigger render={<Button variant="outline" size="sm" />}>撤回申请</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>撤回这条申请？</AlertDialogTitle>
          <AlertDialogDescription>
            {application.roomName}，{formatRange(application.startAt, application.endAt)}。撤回后不能继续审批，如需使用需重新提交。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>保留申请</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={pending} onClick={withdraw}>
            {pending && <Spinner />}
            确认撤回
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
