import * as React from "react"
import { Area, AreaChart, CartesianGrid, XAxis } from "recharts"

import { useIsMobile } from "@/hooks/use-mobile"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { campusDay, campusToday, hoursBetween } from "@/modules/common/time"
import type { Booking } from "@/modules/m1/api"
import { isUsed } from "@/modules/m1/format"

const chartConfig = {
  booked: { label: "预约时长", color: "var(--primary)" },
  used: { label: "签到使用", color: "var(--chart-2)" },
} satisfies ChartConfig

const RANGES = [
  { value: "30", label: "近 30 天" },
  { value: "14", label: "近 14 天" },
  { value: "7", label: "近 7 天" },
]

const DAY = 86_400_000

// 以校区日期为横轴，统计未取消预约的计划小时与签到后的计划使用小时；包含未来 7 天已排期的预约。
function dailyHours(bookings: Booking[], days: number) {
  const today = new Date(`${campusToday()}T00:00:00+08:00`).getTime()
  const rows = new Map<string, { date: string; booked: number; used: number }>()
  for (let t = today - (days - 1) * DAY; t <= today + 7 * DAY; t += DAY) {
    const date = campusDay(new Date(t))
    rows.set(date, { date, booked: 0, used: 0 })
  }
  for (const b of bookings) {
    const row = rows.get(campusDay(b.startAt))
    if (!row || b.status === "CANCELLED") continue
    const hours = hoursBetween(b.startAt, b.endAt)
    row.booked += hours
    if (isUsed(b)) row.used += hours
  }
  return [...rows.values()]
}

const dayLabel = (value: string) => new Date(`${value}T00:00:00+08:00`).toLocaleDateString("zh-CN", { month: "numeric", day: "numeric", timeZone: "Asia/Shanghai" })

export function BookingHoursChart({ bookings }: { bookings: Booking[] }) {
  const isMobile = useIsMobile()
  const [range, setRange] = React.useState("30")

  React.useEffect(() => {
    if (isMobile) setRange("7")
  }, [isMobile])

  const data = React.useMemo(() => dailyHours(bookings, Number(range)), [bookings, range])

  return (
    <Card className="@container/card">
      <CardHeader>
        <CardTitle>我的自习时长</CardTitle>
        <CardDescription>
          <span className="hidden @[540px]/card:block">按预约开始日期统计，包含未来 7 天已排期的预约</span>
          <span className="@[540px]/card:hidden">按日统计</span>
        </CardDescription>
        <CardAction>
          <ToggleGroup
            multiple={false}
            value={[range]}
            onValueChange={(value) => setRange(value[0] ?? "30")}
            variant="outline"
            className="hidden *:data-[slot=toggle-group-item]:px-4! @[767px]/card:flex"
          >
            {RANGES.map((r) => (
              <ToggleGroupItem key={r.value} value={r.value}>
                {r.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <Select items={RANGES} value={range} onValueChange={(value) => value && setRange(value)}>
            <SelectTrigger className="flex w-32 @[767px]/card:hidden" size="sm" aria-label="选择统计范围">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="rounded-xl">
              {RANGES.map((r) => (
                <SelectItem key={r.value} value={r.value} className="rounded-lg">
                  {r.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardAction>
      </CardHeader>
      <CardContent className="px-2 pt-4 sm:px-6 sm:pt-6">
        <ChartContainer config={chartConfig} className="aspect-auto h-[250px] w-full">
          <AreaChart data={data}>
            <defs>
              <linearGradient id="fillBooked" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-booked)" stopOpacity={0.8} />
                <stop offset="95%" stopColor="var(--color-booked)" stopOpacity={0.1} />
              </linearGradient>
              <linearGradient id="fillUsed" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-used)" stopOpacity={0.9} />
                <stop offset="95%" stopColor="var(--color-used)" stopOpacity={0.1} />
              </linearGradient>
            </defs>
            <CartesianGrid vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} tickMargin={8} minTickGap={32} tickFormatter={dayLabel} />
            <ChartTooltip
              cursor={false}
              content={<ChartTooltipContent labelFormatter={(value) => dayLabel(String(value))} indicator="dot" />}
            />
            <Area dataKey="booked" type="monotone" fill="url(#fillBooked)" stroke="var(--color-booked)" />
            <Area dataKey="used" type="monotone" fill="url(#fillUsed)" stroke="var(--color-used)" />
          </AreaChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
}
