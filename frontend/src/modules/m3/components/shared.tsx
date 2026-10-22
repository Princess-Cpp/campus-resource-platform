import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { statusText } from "../reporting"
import type { Order } from "../types"

export const selectClass = "h-9 w-full min-w-0 rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50"

export function Status({ order }: { order: Order }) {
  return <Badge variant={order.syncState === "NEEDS_ATTENTION" ? "destructive" : "secondary"}>{statusText(order)}</Badge>
}
export function ErrorBox({ error }: { error: unknown }) {
  return <Alert variant="destructive"><AlertTitle>操作未完成</AlertTitle><AlertDescription>{error instanceof Error ? error.message : "请稍后重试"}</AlertDescription></Alert>
}
export function FormError({ id, message }: { id: string; message?: string }) {
  return message ? <p id={id} className="text-sm text-destructive" role="alert">{message}</p> : null
}
