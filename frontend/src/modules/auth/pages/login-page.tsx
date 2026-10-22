import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router"
import { LogInIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ensureSession, login, useSession } from "@/modules/common/session"

export function LoginPage() {
  const navigate = useNavigate()
  const session = useSession()
  const [schoolNumber, setSchoolNumber] = useState("")
  const [password, setPassword] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    void ensureSession()
  }, [])
  useEffect(() => {
    if (session.data) navigate(session.data.landingPath, { replace: true })
  }, [navigate, session.data])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setPending(true)
    setError("")
    try {
      const data = await login(schoolNumber, password)
      navigate(data.landingPath, { replace: true })
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-muted/30 px-4 py-10">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="text-2xl">登录校园资源服务平台</CardTitle>
          <CardDescription>使用已注册的学工号和密码登录。</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-5">
            {error && (
              <Alert variant="destructive">
                <AlertTitle>登录失败</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="login-school-number">学工号</FieldLabel>
                <Input
                  id="login-school-number"
                  autoComplete="username"
                  maxLength={64}
                  value={schoolNumber}
                  onChange={(event) => setSchoolNumber(event.target.value)}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="login-password">密码</FieldLabel>
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="current-password"
                  maxLength={72}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
              </Field>
            </FieldGroup>
            <Button className="w-full" type="submit" disabled={pending || session.loading}>
              {pending ? <Spinner /> : <LogInIcon />}
              登录
            </Button>
            <p className="text-center text-sm text-muted-foreground">
              首次使用？{" "}
              <Link className="font-medium text-foreground underline underline-offset-4" to="/register">
                注册学生账号
              </Link>
            </p>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}
