import { BellOffIcon, RefreshCwIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { describeError } from "@/modules/common/api"
import { Page } from "@/modules/common/page"
import { formatDateTime } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { listNotifications } from "@/modules/m1/api"
import { DELIVERY_STATUS, NOTIFICATION_KINDS } from "@/modules/m1/format"

export function NotificationsPage() {
  const { data, error, loading, reload } = useRequest(listNotifications, [])

  return (
    <Page
      title="通知记录"
      description="预约相关的提醒事件（最近 100 条）。邮件投递依赖服务端通知渠道配置。"
      actions={
        <Button variant="outline" size="sm" onClick={reload} disabled={loading}>
          <RefreshCwIcon className={loading ? "animate-spin" : undefined} />
          刷新
        </Button>
      }
    >
      <div className="px-4 lg:px-6">
        {error ? (
          <Alert variant="destructive">
            <AlertTitle>加载失败</AlertTitle>
            <AlertDescription>{describeError(error)}</AlertDescription>
          </Alert>
        ) : loading && !data ? (
          <Skeleton className="h-64 rounded-xl" />
        ) : !data?.length ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <BellOffIcon />
              </EmptyMedia>
              <EmptyTitle>暂无通知</EmptyTitle>
              <EmptyDescription>预约成功、开始前和迟到时会生成提醒。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <Card className="py-0">
            <CardContent className="px-0">
              <Table>
                <TableHeader className="bg-muted">
                  <TableRow>
                    <TableHead className="pl-4">时间</TableHead>
                    <TableHead>类型</TableHead>
                    <TableHead>关联预约</TableHead>
                    <TableHead className="pr-4">投递状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.map((n) => {
                    const delivery = DELIVERY_STATUS[n.status] ?? { label: n.status, tone: "outline" as const }
                    return (
                      <TableRow key={n.eventId}>
                        <TableCell className="pl-4 tabular-nums">{formatDateTime(n.createdAt)}</TableCell>
                        <TableCell className="font-medium">{NOTIFICATION_KINDS[n.kind] ?? n.kind}</TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">{n.bookingId}</TableCell>
                        <TableCell className="pr-4">
                          <Badge variant={delivery.tone}>{delivery.label}</Badge>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}
      </div>
    </Page>
  )
}
