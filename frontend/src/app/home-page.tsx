import { Link } from "react-router"
import { ArrowRightIcon, CalendarCheckIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemTitle } from "@/components/ui/item"
import { modules } from "@/app/modules"
import { describeError } from "@/modules/common/api"
import { Page } from "@/modules/common/page"
import { formatRange } from "@/modules/common/time"
import { useRequest } from "@/modules/common/use-request"
import { listBookings } from "@/modules/m1/api"
import { BookingHoursChart } from "@/modules/m1/components/booking-hours-chart"
import { BookingSummaryCards, summarize } from "@/modules/m1/components/booking-summary-cards"
import { CheckInDialog } from "@/modules/m1/components/check-in-dialog"

// 工作台统计只取本人最近 100 条预约（历史接口单页上限）。
const RECENT_LIMIT = 100

export function HomePage() {
  const { data, error, loading, reload } = useRequest(() => listBookings(1, RECENT_LIMIT), [])
  const upcoming = data ? summarize(data).upcoming.slice(0, 5) : []
  const planned = modules.filter((m) => m.status === "planned")

  return (
    <Page title="工作台" description="自习座位预约概览；统计基于你最近 100 条预约记录。">
      {error !== undefined && (
        <div className="px-4 lg:px-6">
          <Alert variant="destructive">
            <AlertTitle>预约数据加载失败</AlertTitle>
            <AlertDescription>{describeError(error)}</AlertDescription>
          </Alert>
        </div>
      )}
      <BookingSummaryCards bookings={data} loading={loading} />
      <div className="px-4 lg:px-6">
        <BookingHoursChart bookings={data ?? []} />
      </div>
      <div className="grid grid-cols-1 gap-4 px-4 lg:px-6 @4xl/main:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <Card>
          <CardHeader>
            <CardTitle>即将开始</CardTitle>
            <CardDescription>开始后 15 分钟内需要签到</CardDescription>
            <CardAction>
              <Button variant="ghost" size="sm" render={<Link to="/m1/bookings" />}>
                全部预约
                <ArrowRightIcon />
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            {upcoming.length === 0 ? (
              <Empty className="py-6">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <CalendarCheckIcon />
                  </EmptyMedia>
                  <EmptyTitle>暂无待签到的预约</EmptyTitle>
                  <EmptyDescription>
                    <Link to="/m1/search">查找座位</Link> 开始预约。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <ItemGroup className="gap-2">
                {upcoming.map((b) => (
                  <Item key={b.id} variant="outline" size="sm">
                    <ItemContent>
                      <ItemTitle>
                        {b.roomName} · {b.seatLabel}
                      </ItemTitle>
                      <ItemDescription className="tabular-nums">{formatRange(b.startAt, b.endAt)}</ItemDescription>
                    </ItemContent>
                    <ItemActions>
                      <CheckInDialog booking={b} onDone={reload} />
                    </ItemActions>
                  </Item>
                ))}
              </ItemGroup>
            )}
          </CardContent>
        </Card>
        {planned.map((m) => (
          <Card key={m.id}>
            <CardHeader>
              <CardTitle>{m.label}</CardTitle>
              <CardDescription>整间教室的申请与多级审批</CardDescription>
              <CardAction>
                <Badge variant="outline">开发中</Badge>
              </CardAction>
            </CardHeader>
            <CardContent>
              <ItemGroup className="gap-1">
                {m.nav.map((item) => (
                  <Item key={item.url} size="sm" render={<Link to={item.url} />}>
                    <ItemContent>
                      <ItemTitle className="[&_svg]:size-4">
                        {item.icon}
                        {item.title}
                      </ItemTitle>
                    </ItemContent>
                    <ItemActions>
                      <ArrowRightIcon className="size-4 text-muted-foreground" />
                    </ItemActions>
                  </Item>
                ))}
              </ItemGroup>
            </CardContent>
            <CardFooter className="text-xs text-muted-foreground">页面位置已预留，由 M2 组实现后接入。</CardFooter>
          </Card>
        ))}
      </div>
    </Page>
  )
}
