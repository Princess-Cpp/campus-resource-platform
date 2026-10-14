import { useMemo, useState, type FormEvent } from "react"
import { useNavigate } from "react-router"
import { ArmchairIcon, PlugZapIcon, SearchIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { describeError } from "@/modules/common/api"
import { Page } from "@/modules/common/page"
import { campusHour, campusInstant, campusToday, formatRange } from "@/modules/common/time"
import { searchSeats, type AvailableSeat } from "@/modules/m1/api"
import { BookSeatDialog } from "@/modules/m1/components/book-seat-dialog"

// 整点预约，单次最长 4 小时（M1-01.2 提案默认值，以服务端校验为准）。
const MAX_HOURS = 4
const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, "0")}:00` }))
const DURATIONS = Array.from({ length: MAX_HOURS }, (_, i) => ({ value: String(i + 1), label: `${i + 1} 小时` }))

type Query = { startAt: Date; endAt: Date; area: string; outlet: boolean }

export function SearchPage() {
  const navigate = useNavigate()
  const [date, setDate] = useState(campusToday())
  const [hour, setHour] = useState(String(Math.min(campusHour() + 1, 23)))
  const [duration, setDuration] = useState("2")
  const [area, setArea] = useState("")
  const [outlet, setOutlet] = useState(false)
  const [query, setQuery] = useState<Query>()
  const [results, setResults] = useState<AvailableSeat[]>()
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  const run = async (q: Query) => {
    setLoading(true)
    setError("")
    try {
      setResults(await searchSeats(q))
      setQuery(q)
    } catch (reason) {
      setError(describeError(reason))
      setResults(undefined)
    } finally {
      setLoading(false)
    }
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const startAt = campusInstant(date, Number(hour))
    const endAt = new Date(startAt.getTime() + Number(duration) * 3_600_000)
    run({ startAt, endAt, area: area.trim(), outlet })
  }

  const rooms = useMemo(() => new Set(results?.map((r) => r.room.id)).size, [results])

  return (
    <Page title="查找座位" description="选择日期与整点时段，查看你有权预约且当前空闲的座位。">
      <div className="px-4 lg:px-6">
        <Card>
          <CardContent>
            <form onSubmit={submit}>
              <FieldGroup className="grid grid-cols-1 gap-4 @3xl/main:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.2fr)_auto_auto] @3xl/main:items-end">
                <Field>
                  <FieldLabel htmlFor="search-date">日期</FieldLabel>
                  <Input id="search-date" type="date" value={date} min={campusToday()} onChange={(e) => setDate(e.target.value)} required />
                </Field>
                <Field>
                  <FieldLabel>开始时间</FieldLabel>
                  <Select items={HOURS} value={hour} onValueChange={(v) => v && setHour(v)}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {HOURS.map((h) => (
                        <SelectItem key={h.value} value={h.value}>
                          {h.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel>时长</FieldLabel>
                  <Select items={DURATIONS} value={duration} onValueChange={(v) => v && setDuration(v)}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {DURATIONS.map((d) => (
                        <SelectItem key={d.value} value={d.value}>
                          {d.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="search-area">区域</FieldLabel>
                  <Input id="search-area" value={area} onChange={(e) => setArea(e.target.value)} placeholder="例如 A楼，可留空" />
                </Field>
                <Field orientation="horizontal" className="h-9 w-auto">
                  <Switch id="search-outlet" checked={outlet} onCheckedChange={setOutlet} />
                  <FieldLabel htmlFor="search-outlet" className="whitespace-nowrap">
                    需要插座
                  </FieldLabel>
                </Field>
                <Button type="submit" disabled={loading}>
                  {loading ? <Spinner /> : <SearchIcon />}
                  搜索
                </Button>
              </FieldGroup>
            </form>
          </CardContent>
        </Card>
      </div>

      <div className="px-4 lg:px-6">
        {error ? (
          <Alert variant="destructive">
            <AlertTitle>查询失败</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : loading && !results ? (
          <Skeleton className="h-64 rounded-xl" />
        ) : !results || !query ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <ArmchairIcon />
              </EmptyMedia>
              <EmptyTitle>选择时段开始查找</EmptyTitle>
              <EmptyDescription>院系专属自习室只对本院系同学显示；提交预约时会再次校验。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : results.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <SearchIcon />
              </EmptyMedia>
              <EmptyTitle>该时段没有可预约的座位</EmptyTitle>
              <EmptyDescription>试试更换时段、区域，或关闭“需要插座”筛选。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <Card>
            <CardHeader>
              <CardTitle>可预约座位</CardTitle>
              <CardDescription>
                {formatRange(query.startAt.toISOString(), query.endAt.toISOString())} · {rooms} 间自习室 · {results.length} 个座位
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>自习室</TableHead>
                    <TableHead>区域</TableHead>
                    <TableHead>座位</TableHead>
                    <TableHead>开放时间</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {results.map((item) => (
                    <TableRow key={`${item.room.id}/${item.seat.id}`}>
                      <TableCell className="font-medium">{item.room.name}</TableCell>
                      <TableCell>{item.room.area || "—"}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          {item.seat.label}
                          {item.seat.outlet && (
                            <Badge variant="outline">
                              <PlugZapIcon />
                              插座
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="tabular-nums text-muted-foreground">
                        {String(item.room.openHour).padStart(2, "0")}:00–{String(item.room.closeHour).padStart(2, "0")}:00
                      </TableCell>
                      <TableCell className="text-right">
                        <BookSeatDialog item={item} startAt={query.startAt} endAt={query.endAt} onBooked={() => navigate("/m1/bookings")} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}
      </div>
    </Page>
  )
}
