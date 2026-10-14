import { CalendarClockIcon, CircleAlertIcon, CircleCheckIcon, HourglassIcon } from "lucide-react"
import type { ReactNode } from "react"

import { Badge } from "@/components/ui/badge"
import { Card, CardAction, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { campusDay, campusToday, formatDateTime, hoursBetween } from "@/modules/common/time"
import type { Booking } from "@/modules/m1/api"
import { isNoShow, isUsed } from "@/modules/m1/format"

// 指标只基于当前用户最近的预约记录（本人历史接口的第一页），不是全校运营统计（M1-04.2 尚未实现）。
export function summarize(bookings: Booking[], now = new Date()) {
  const month = campusToday(now).slice(0, 7)
  const upcoming = bookings
    .filter((b) => b.status === "RESERVED" && new Date(b.endAt) > now)
    .sort((a, b) => a.startAt.localeCompare(b.startAt))
  const thisMonth = bookings.filter((b) => campusDay(b.startAt).startsWith(month))
  return {
    upcoming,
    monthHours: thisMonth.filter((b) => b.status !== "CANCELLED").reduce((sum, b) => sum + hoursBetween(b.startAt, b.endAt), 0),
    monthCount: thisMonth.filter((b) => b.status !== "CANCELLED").length,
    used: bookings.filter(isUsed).length,
    noShow: bookings.filter(isNoShow).length,
  }
}

function StatCard({ label, value, icon, footer, hint }: { label: string; value: ReactNode; icon: ReactNode; footer: ReactNode; hint: string }) {
  return (
    <Card className="@container/card">
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">{value}</CardTitle>
        <CardAction>
          <Badge variant="outline">{icon}</Badge>
        </CardAction>
      </CardHeader>
      <CardFooter className="flex-col items-start gap-1.5 text-sm">
        <div className="line-clamp-1 font-medium">{footer}</div>
        <div className="text-muted-foreground">{hint}</div>
      </CardFooter>
    </Card>
  )
}

export function BookingSummaryCards({ bookings, loading }: { bookings?: Booking[]; loading: boolean }) {
  const grid =
    "grid grid-cols-1 gap-4 px-4 *:data-[slot=card]:bg-linear-to-t *:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card *:data-[slot=card]:shadow-xs lg:px-6 @xl/main:grid-cols-2 @5xl/main:grid-cols-4 dark:*:data-[slot=card]:bg-card"
  if (loading && !bookings) {
    return (
      <div className={grid}>
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-36 rounded-xl" />
        ))}
      </div>
    )
  }
  const s = summarize(bookings ?? [])
  const next = s.upcoming[0]
  return (
    <div className={grid}>
      <StatCard
        label="待签到预约"
        value={s.upcoming.length}
        icon={<CalendarClockIcon />}
        footer={next ? `下一场 ${formatDateTime(next.startAt)}` : "暂无即将开始的预约"}
        hint={next ? `${next.roomName} · ${next.seatLabel}` : "可在“查找座位”中预约"}
      />
      <StatCard
        label="本月预约时长"
        value={`${s.monthHours} 小时`}
        icon={<HourglassIcon />}
        footer={`${s.monthCount} 条有效预约`}
        hint="不含已取消的预约"
      />
      <StatCard label="已签到使用" value={s.used} icon={<CircleCheckIcon />} footer="使用中与已完成" hint="以签到为准，不含离座时间" />
      <StatCard
        label="未签到违约"
        value={s.noShow}
        icon={<CircleAlertIcon />}
        footer={s.noShow ? "开始后 15 分钟未签到会自动取消" : "保持良好记录"}
        hint="统计范围为最近的预约记录"
      />
    </div>
  )
}
