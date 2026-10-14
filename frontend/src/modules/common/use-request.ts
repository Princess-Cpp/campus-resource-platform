import { useCallback, useEffect, useState } from "react"

// 读取类请求的状态封装；写操作在各页面内显式处理，避免隐式重试。
export function useRequest<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<unknown>()
  const [loading, setLoading] = useState(true)
  const [version, setVersion] = useState(0)

  useEffect(() => {
    let active = true
    setLoading(true)
    load()
      .then((value) => {
        if (!active) return
        setData(value)
        setError(undefined)
      })
      .catch((reason) => active && setError(reason))
      .finally(() => active && setLoading(false))
    return () => {
      active = false
    }
  }, [...deps, version])

  const reload = useCallback(() => setVersion((v) => v + 1), [])
  return { data, error, loading, reload }
}
