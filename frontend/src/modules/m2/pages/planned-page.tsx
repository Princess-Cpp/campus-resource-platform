import { ConstructionIcon } from "lucide-react"
import type { ReactNode } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Page } from "@/modules/common/page"

export type PlannedPage = {
  title: string
  description: string
  icon: ReactNode
  requirements: string[]
  capabilities: string[]
}

// M2 页面预留位：外壳已挂好路由与导航，M2 组实现时用真实页面替换 element，不改外壳。
export function PlannedModulePage({ page }: { page: PlannedPage }) {
  return (
    <Page title={page.title} description={page.description}>
      <div className="flex flex-col gap-4 px-4 lg:px-6">
        <Alert>
          <ConstructionIcon />
          <AlertTitle>M2 教室申请审批 · 开发中</AlertTitle>
          <AlertDescription>此页面为前端外壳预留位置，尚未连接任何接口，也不展示示例数据。</AlertDescription>
        </Alert>
        <div className="grid gap-4 @4xl/main:grid-cols-[2fr_1fr]">
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">{page.icon}</EmptyMedia>
              <EmptyTitle>{page.title}</EmptyTitle>
              <EmptyDescription>{page.description}</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <div className="flex flex-wrap justify-center gap-2">
                {page.requirements.map((id) => (
                  <Badge key={id} variant="outline" className="font-mono">
                    {id}
                  </Badge>
                ))}
              </div>
            </EmptyContent>
          </Empty>
          <Card>
            <CardHeader>
              <CardTitle>计划能力</CardTitle>
              <CardDescription>来自 docs/requirements/m2.md，具体规则待评审</CardDescription>
            </CardHeader>
            <CardContent>
              <ul className="flex list-disc flex-col gap-2 pl-4 text-sm text-muted-foreground">
                {page.capabilities.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
            </CardContent>
          </Card>
        </div>
      </div>
    </Page>
  )
}
