import { useCallback, useEffect, useRef, useState } from 'react'
import { apiUrl } from '../lib/api'

/** How long a revealed value stays visible. The dashboard is screen-shared. */
export const REVEAL_TTL_MS = 30_000

export function useSecretReveal(resourceId: string) {
  const [values, setValues] = useState<Record<string, string>>({})
  const timers = useRef<Record<string, ReturnType<typeof setTimeout>>>({})

  const clearAll = useCallback(() => {
    Object.values(timers.current).forEach(clearTimeout)
    timers.current = {}
    setValues({})
  }, [])

  // Never let a revealed value outlive the pane it belongs to.
  useEffect(() => clearAll, [clearAll, resourceId])

  const hide = useCallback((field: string) => {
    clearTimeout(timers.current[field])
    delete timers.current[field]
    setValues((v) => {
      const next = { ...v }
      delete next[field]
      return next
    })
  }, [])

  const reveal = useCallback(
    async (field: string) => {
      const res = await fetch(apiUrl(`/resources/component/${encodeURIComponent(resourceId)}/secret-value`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ field }),
      })
      if (!res.ok) throw new Error(`could not reveal ${field}: ${res.status}`)
      const body = (await res.json()) as { value: string }
      setValues((v) => ({ ...v, [field]: body.value }))
      timers.current[field] = setTimeout(() => hide(field), REVEAL_TTL_MS)
    },
    [resourceId, hide],
  )

  return { values, reveal, hide }
}
