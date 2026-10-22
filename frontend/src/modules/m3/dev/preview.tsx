/// <reference types="vite/client" />
import { StrictMode, useState } from "react"
import { createRoot } from "react-dom/client"
import { createMemoryRouter, Link, Outlet, RouterProvider } from "react-router"
import { ThemeProvider } from "next-themes"
import { WrenchIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { TooltipProvider } from "@/components/ui/tooltip"
import { createM3Module } from "../module"
import { ReportSubmission } from "../reporting"
import { createPreviewGateway } from "./gateway"
import "@/index.css"

if (!import.meta.env.DEV) throw new Error("M3 interaction preview is development-only")
const gateway = createPreviewGateway({ seed: !new URLSearchParams(location.search).has("empty") })
const submission = new ReportSubmission(gateway)
const module = createM3Module(gateway, submission)

function PreviewLayout() {
  const [readFailure, setReadFailure] = useState(false)
  const [notice, setNotice] = useState("")
  return <div className="min-h-dvh bg-background text-foreground">
    <div className="border-b border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-100"><div className="mx-auto max-w-6xl"><strong>界面预览 · 示例数据</strong><span className="ml-2">不连接业务接口，不会发送报修。刷新页面后示例数据会重置。</span></div></div>
    <header className="border-b"><div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-4"><div className="flex items-center gap-2 font-semibold"><WrenchIcon className="size-5" />物业报修</div><nav aria-label="报修导航" className="flex gap-5 text-sm">{module.nav.map(item => <Link key={item.url} to={item.url} className="rounded-sm underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-4">{item.title}</Link>)}</nav></div></header>
    <main className="mx-auto flex max-w-6xl flex-col"><Outlet /></main>
    <footer className="mx-auto mt-8 max-w-6xl space-y-3 border-t px-4 py-5 text-xs text-muted-foreground"><p>预览检查：表单校验、筛选分页、详情展示及请求重试。此页面不提供登录、权限、数据库或资源联动验收。</p><div className="flex flex-wrap items-center gap-3"><Button size="sm" variant="outline" onClick={() => { gateway.loseNextCreateReply(); setNotice("下一笔示例报修将模拟响应丢失，可在表单中重试原请求。") }}>下次提交模拟响应丢失</Button><label className="flex items-center gap-2"><input type="checkbox" checked={readFailure} onChange={e => { gateway.setReadFailure(e.target.checked); setReadFailure(e.target.checked); setNotice("请刷新列表或重新进入页面查看读取结果。") }} />模拟读取失败</label><a className="underline underline-offset-4" href="/dev/m3-preview.html?empty=1">重新打开空列表示例</a></div>{notice && <p role="status">{notice}</p>}</footer>
  </div>
}
const router = createMemoryRouter([{ element: <PreviewLayout />, children: module.routes }], { initialEntries: ["/m3/report"] })
createRoot(document.getElementById("root")!).render(<StrictMode><ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange><TooltipProvider><RouterProvider router={router} /></TooltipProvider></ThemeProvider></StrictMode>)
