import { StrictMode, useCallback, useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ApiClient } from './api'
import { App } from './App'
import { loadConfig, type Config } from './config'
import { SessionContext, type Session } from './context'
import { completeDevPopup } from './providers'
import { SignIn, type StaffSession } from './SignIn'
import { applyTheme, readTheme } from './theme'
import type { Me } from './types'
import { TooltipProvider } from '@/components/ui/tooltip'
import './index.css'

// The session lives in this tab's session storage only: closing the tab signs out. The token is the API's own
// back-office session (ADR 0018); the Google or Apple ID token is used once, to open it.
const KEY = 'backoffice.session'

function stored(): StaffSession | null {
  try {
    const raw = sessionStorage.getItem(KEY)
    const s = raw ? (JSON.parse(raw) as StaffSession) : null
    return s && new Date(s.expires_at) > new Date() ? s : null
  } catch {
    return null
  }
}

function Root({ config }: { config: Config }) {
  const [current, setCurrent] = useState<StaffSession | null>(stored)
  const [notice, setNotice] = useState<string | undefined>()

  const end = useCallback((message?: string) => {
    try { sessionStorage.removeItem(KEY) } catch { /* nothing stored */ }
    setNotice(message)
    setCurrent(null)
  }, [])

  const api = useMemo(
    () => new ApiClient(config.apiBaseUrl, () => Promise.resolve(current?.token ?? null), undefined,
      current ? () => { end('Your session ended. Sign in again.') } : undefined),
    [config.apiBaseUrl, current, end],
  )

  // the session ends on time even with the tab open, not only when the next request meets a 401
  useEffect(() => {
    if (!current) return
    const left = new Date(current.expires_at).getTime() - Date.now()
    const timer = setTimeout(() => { end('Your session ended. Sign in again.') }, Math.max(0, Math.min(left, 2 ** 31 - 1)))
    return () => { clearTimeout(timer) }
  }, [current, end])

  // roles can change while signed in (an admin edits them): the member is re-read on load and when the tab returns
  useEffect(() => {
    if (!current) return
    let last = 0
    const refresh = () => {
      if (Date.now() - last < 60_000) return
      last = Date.now()
      api.get<Me>('/v1/staff/me').then(
        (me) => {
          setCurrent((c) => {
            if (!c || c.token !== current.token || JSON.stringify(c.member) === JSON.stringify(me)) return c
            const next = { ...c, member: me }
            try { sessionStorage.setItem(KEY, JSON.stringify(next)) } catch { /* until reload */ }
            return next
          })
        },
        () => undefined, // a 401 already ended the session; anything else waits for the next look
      )
    }
    refresh()
    const onVisible = () => { if (document.visibilityState === 'visible') refresh() }
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [api, current])

  const session: Session | null = useMemo(() => current && {
    me: current.member,
    api,
    signOut: () => { void api.delete('/v1/staff/session').catch(() => undefined).finally(() => { end() }) },
  }, [current, api, end])

  if (!session) {
    return (
      <SignIn config={config} api={api} notice={notice} onSignedIn={(s) => {
        try { sessionStorage.setItem(KEY, JSON.stringify(s)) } catch { /* the session lasts until reload */ }
        setNotice(undefined)
        setCurrent(s)
      }} />
    )
  }
  return (
    <SessionContext.Provider value={session}>
      <App />
    </SessionContext.Provider>
  )
}

applyTheme(readTheme())
const config = loadConfig()
const root = document.getElementById('root')
if (config?.dev && location.pathname === '/callback') {
  void completeDevPopup(config.dev) // the local staff realm's popup hands its result back and closes
} else if (root) {
  createRoot(root).render(
    <StrictMode>
      <TooltipProvider>
        {config ? (
          <BrowserRouter><Root config={config} /></BrowserRouter>
        ) : (
          <div className="grid min-h-svh place-items-center p-6 text-center">
            <p role="alert" className="max-w-md text-muted-foreground">This build has no server or sign-in configured (VITE_API_BASE_URL, and VITE_GOOGLE_CLIENT_ID or VITE_APPLE_CLIENT_ID).</p>
          </div>
        )}
      </TooltipProvider>
    </StrictMode>,
  )
}
