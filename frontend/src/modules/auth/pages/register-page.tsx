import { useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router"
import { UserPlusIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ensureSession, register, useSession } from "@/modules/common/session"

export function RegisterPage() {
  const navigate = useNavigate()
  const session = useSession()
  const [schoolNumber, setSchoolNumber] = useState("")
  const [name, setName] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
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
    if (password !== confirmPassword) {
      setError("两次输入的密码不一致")
      return
    }
    setPending(true)
    setError("")
    try {
      const data = await register(schoolNumber, name, password, confirmPassword)
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
          <CardTitle className="text-2xl">注册学生账号</CardTitle>
          <CardDescription>填写学工号、姓名和密码即可注册；角色由系统确定为学生。</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-5">
            {error && (
              <Alert variant="destructive">
                <AlertTitle>注册失败</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="register-school-number">学工号</FieldLabel>
                <Input
                  id="register-school-number"
                  autoComplete="username"
                  maxLength={64}
                  value={schoolNumber}
                  onChange={(event) => setSchoolNumber(event.target.value)}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="register-name">姓名</FieldLabel>
                <Input
                  id="register-name"
                  autoComplete="name"
                  maxLength={128}
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  required
                />
                <FieldDescription>将作为账号显示名称。</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="register-password">密码</FieldLabel>
                <Input
                  id="register-password"
                  type="password"
                  autoComplete="new-password"
                  minLength={10}
                  maxLength={72}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
                <FieldDescription>请输入 10–72 个字符。</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="register-confirm-password">确认密码</FieldLabel>
                <Input
                  id="register-confirm-password"
                  type="password"
                  autoComplete="new-password"
                  minLength={10}
                  maxLength={72}
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  required
                />
              </Field>
            </FieldGroup>
            <Button className="w-full" type="submit" disabled={pending || session.loading}>
              {pending ? <Spinner /> : <UserPlusIcon />}
              注册并登录
            </Button>
            <p className="text-center text-sm text-muted-foreground">
              已有账号？{" "}
              <Link className="font-medium text-foreground underline underline-offset-4" to="/login">
                返回登录
              </Link>
            </p>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}
