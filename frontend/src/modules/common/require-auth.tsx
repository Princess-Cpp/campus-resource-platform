import { useEffect } from "react"
import { Navigate, Outlet } from "react-router"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { ensureSession, useSession } from "@/modules/common/session"

export function RequireAuth() {
  const session = useSession()

  useEffect(() => {
    void ensureSession()
  }, [])

  if (session.loading) {
    return (
      <main className="grid min-h-dvh place-items-center">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner />
          正在验证登录状态…
        </div>
      </main>
    )
  }
  if (session.error) {
    return (
      <main className="grid min-h-dvh place-items-center px-4">
        <div className="space-y-3 text-center">
          <p className="text-sm text-destructive">{session.error}</p>
          <Button onClick={() => void ensureSession(true)}>重试</Button>
        </div>
      </main>
    )
  }
  if (!session.data) return <Navigate to="/login" replace />
  return <Outlet />
}
