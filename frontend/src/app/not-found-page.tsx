import { Link } from "react-router"
import { CompassIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"

export function NotFoundPage() {
  return (
    <Empty className="m-4 border lg:m-6">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <CompassIcon />
        </EmptyMedia>
        <EmptyTitle>页面不存在</EmptyTitle>
        <EmptyDescription>链接可能已失效，或该功能尚未上线。</EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button size="sm" render={<Link to="/" />}>
          返回工作台
        </Button>
      </EmptyContent>
    </Empty>
  )
}
