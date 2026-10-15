import { useState } from "react"
import { Link } from "react-router"
import { CalendarXIcon, ChevronLeftIcon, ChevronRightIcon, PlusIcon, RefreshCwIcon } from "lucide-react"

import { Alert, AlertAction, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { describeError } from "@/modules/common/api"
import { Page } from "@/modules/common/page"
import { formatRange } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { HISTORY_PAGE_SIZE, listBookings, type Booking } from "@/modules/m1/api"
import { BookingStatusBadge } from "@/modules/m1/components/booking-status-badge"
import { CancelBookingButton } from "@/modules/m1/components/cancel-booking-button"
import { CheckInDialog } from "@/modules/m1/components/check-in-dialog"
import { DeleteBookingButton } from "@/modules/m1/components/delete-booking-button"
import { canDelete } from "@/modules/m1/format"

const FILTERS: { value: string; label: string; match: (b: Booking) => boolean }[] = [
  { value: "all", label: "全部", match: () => true },
  { value: "reserved", label: "待签到", match: (b) => b.status === "RESERVED" },
  { value: "used", label: "使用中/已完成", match: (b) => b.status === "CHECKED_IN" || b.status === "FINISHED" },
  { value: "cancelled", label: "已取消", match: (b) => b.status === "CANCELLED" },
]

export function BookingsPage() {
  const [page, setPage] = useState(1)
  const [filter, setFilter] = useState("all")
  const { data, error, loading, reload } = useRequest(() => listBookings(page), [page])
  const match = FILTERS.find((f) => f.value === filter)!.match
  const rows = data?.filter(match) ?? []

  return (
    <Page
      title="我的预约"
      description="只显示你本人的预约记录；签到与取消规则由服务端校验。"
      actions={
        <>
          <Button variant="outline" size="sm" onClick={reload} disabled={loading}>
            <RefreshCwIcon className={loading ? "animate-spin" : undefined} />
            刷新
          </Button>
          <Button size="sm" render={<Link to="/m1/search" />}>
            <PlusIcon />
            预约座位
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4 px-4 lg:px-6">
        <Tabs value={filter} onValueChange={(v) => setFilter(String(v))}>
          <TabsList>
            {FILTERS.map((f) => (
              <TabsTrigger key={f.value} value={f.value}>
                {f.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        {error ? (
          <Alert variant="destructive">
            <AlertTitle>加载失败</AlertTitle>
            <AlertDescription>{describeError(error)}</AlertDescription>
            <AlertAction>
              <Button size="sm" variant="outline" onClick={reload}>
                重试
              </Button>
            </AlertAction>
          </Alert>
        ) : loading && !data ? (
          <Skeleton className="h-64 rounded-xl" />
        ) : rows.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <CalendarXIcon />
              </EmptyMedia>
              <EmptyTitle>{data?.length ? "没有符合筛选条件的预约" : "还没有预约记录"}</EmptyTitle>
              <EmptyDescription>预约成功后，可在这里签到、取消或查看历史。</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button size="sm" render={<Link to="/m1/search" />}>
                去查找座位
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <Card className="py-0">
            <CardContent className="px-0">
              <Table>
                <TableHeader className="bg-muted">
                  <TableRow>
                    <TableHead className="pl-4">自习室</TableHead>
                    <TableHead>座位</TableHead>
                    <TableHead>时段</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="pr-4 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((b) => (
                    <TableRow key={b.id}>
                      <TableCell className="pl-4 font-medium">{b.roomName}</TableCell>
                      <TableCell>{b.seatLabel}</TableCell>
                      <TableCell className="tabular-nums">{formatRange(b.startAt, b.endAt)}</TableCell>
                      <TableCell>
                        <BookingStatusBadge booking={b} />
                      </TableCell>
                      <TableCell className="pr-4">
                        <div className="flex justify-end gap-2">
                          {b.status === "RESERVED" && (
                            <>
                              <CheckInDialog booking={b} onDone={reload} />
                              <CancelBookingButton booking={b} onDone={reload} />
                            </>
                          )}
                          {canDelete(b) && <DeleteBookingButton booking={b} onDone={reload} />}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        <div className="flex items-center justify-between text-sm text-muted-foreground">
          <span>第 {page} 页{filter !== "all" && "（筛选仅作用于当前页）"}</span>
          <div className="flex gap-2">
            <Button variant="outline" size="icon-sm" aria-label="上一页" disabled={page === 1 || loading} onClick={() => setPage(page - 1)}>
              <ChevronLeftIcon />
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label="下一页"
              disabled={(data?.length ?? 0) < HISTORY_PAGE_SIZE || loading}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRightIcon />
            </Button>
          </div>
        </div>
      </div>
    </Page>
  )
}
