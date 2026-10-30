import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useSession } from './context'
import { can } from './roles'
import type { Alert } from './types'

interface AlertWatch {
  /** open alerts nobody has acknowledged yet (null until first read, and for members who cannot see alerts) */
  open: Alert[] | null
  refresh: () => void
}

export const ALERT_POLL_MS = 60_000

const AlertWatchContext = createContext<AlertWatch>({ open: null, refresh: () => undefined })

export const useAlertWatch = () => useContext(AlertWatchContext)

/**
 * Keeps the open alerts in view on every page (owner, 2026-10-30: system alerts in the back office): polled every
 * minute while the tab is visible and whenever it regains focus, one request at a time.
 */
export function AlertWatchProvider({ children }: { children: ReactNode }) {
  const { me, api } = useSession()
  const allowed = can(me.roles, 'staff')
  const [open, setOpen] = useState<Alert[] | null>(null)
  const inflight = useRef(false)
  const live = useRef(true)

  const check = useCallback(() => {
    if (!allowed || inflight.current) return
    inflight.current = true
    api.get<{ alerts?: Alert[] } | undefined>('/v1/staff/alerts').then(
      (r) => {
        const alerts = r?.alerts
        if (live.current && Array.isArray(alerts)) setOpen(alerts.filter((a) => !a.acknowledged_at && !a.resolved_at))
      },
      () => undefined, // keep the last list; the next poll asks again
    ).finally(() => { inflight.current = false })
  }, [api, allowed])

  useEffect(() => {
    if (!allowed) return
    live.current = true
    const onFocus = () => { check() }
    const onVisible = () => { if (document.visibilityState === 'visible') check() }
    const tick = () => { if (document.visibilityState !== 'hidden') check() }
    check()
    const timer = setInterval(tick, ALERT_POLL_MS)
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      live.current = false
      clearInterval(timer)
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [check, allowed])

  return <AlertWatchContext.Provider value={{ open, refresh: check }}>{children}</AlertWatchContext.Provider>
}
