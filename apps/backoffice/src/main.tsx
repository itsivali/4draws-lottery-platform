import { StrictMode, useCallback, useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ApiClient } from './api'
import { App } from './App'
import { loadConfig, type Config } from './config'
import { SessionContext, useSession, type Session } from './context'
import { completeDevPopup } from './providers'
import { NameForm } from './components/NameDialog'
import { PasswordForm } from './components/PasswordDialog'
import { SignIn, type StaffSession } from './SignIn'
import type { Me } from './types'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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

  const updateMe = useCallback((me: Me) => {
    setCurrent((c) => {
      if (!c) return c
      const next = { ...c, member: me }
      try { sessionStorage.setItem(KEY, JSON.stringify(next)) } catch { /* until reload */ }
      return next
    })
  }, [])

  const session: Session | null = useMemo(() => current && {
    me: current.member,
    api,
    signOut: () => { void api.delete('/v1/staff/session').catch(() => undefined).finally(() => { end() }) },
    updateMe,
  }, [current, api, end, updateMe])

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
      {session.me.must_change_password ? (
        <FirstPassword onDone={() => { updateMe({ ...session.me, must_change_password: false }) }} />
      ) : !session.me.name.trim() ? (
        <FirstName onDone={updateMe} />
      ) : <App />}
    </SessionContext.Provider>
  )
}

/** A member signed in with a temporary password sets their own before anything else (ADR 0019). */
function FirstPassword({ onDone }: { onDone: () => void }) {
  const { signOut } = useSession()
  return (
    <main id="main" className="grid min-h-svh place-items-center bg-background p-6">
      <Card className="w-full max-w-[460px] shadow-float">
        <CardHeader>
          <CardTitle className="text-xl">Choose your password</CardTitle>
          <CardDescription>You signed in with a temporary password. Set your own to continue.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <PasswordForm forced onDone={onDone} />
          <button type="button" className="justify-self-start text-sm text-muted-foreground underline-offset-4 hover:underline" onClick={signOut}>Sign out instead</button>
        </CardContent>
      </Card>
    </main>
  )
}

/** A member without a name gives one before anything else: the back office shows it in place of their email address. */
function FirstName({ onDone }: { onDone: (me: Me) => void }) {
  const { signOut } = useSession()
  return (
    <main id="main" className="grid min-h-svh place-items-center bg-background p-6">
      <Card className="w-full max-w-[460px] shadow-float">
        <CardHeader>
          <CardTitle className="text-xl">What should we call you?</CardTitle>
          <CardDescription>The back office shows your name, not your email address, wherever you appear.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <NameForm onDone={onDone} label="Continue" />
          <button type="button" className="justify-self-start text-sm text-muted-foreground underline-offset-4 hover:underline" onClick={signOut}>Sign out instead</button>
        </CardContent>
      </Card>
    </main>
  )
}

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
            <p role="alert" className="max-w-md text-muted-foreground">This build has no server configured (VITE_API_BASE_URL must be an https address, or http on localhost).</p>
          </div>
        )}
      </TooltipProvider>
    </StrictMode>,
  )
}
