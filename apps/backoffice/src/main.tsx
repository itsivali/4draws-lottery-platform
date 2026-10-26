import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import { StrictMode, useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ApiClient, ProblemError } from './api'
import { App } from './App'
import { loadConfig, type Config } from './config'
import { SessionContext, type Session } from './context'
import './styles/app.css'
import type { Me } from './types'

let bootstrapping: Promise<Session | null> | null = null

async function bootstrap(users: UserManager, config: Config): Promise<Session | null> {
  if (location.pathname === '/callback') {
    await users.signinRedirectCallback()
    history.replaceState(null, '', '/')
  }
  const user = await users.getUser()
  if (!user || user.expired) {
    await users.signinRedirect()
    return null
  }
  const api = new ApiClient(config.apiBaseUrl, async () => (await users.getUser())?.access_token ?? null)
  // roles come from the server, which checks the token, not from decoding it here
  const me = await api.get<Me>('/v1/staff/me')
  return { me, api, signOut: () => void users.signoutRedirect() }
}

function SignedIn({ config }: { config: Config }) {
  const users = useMemo(
    () =>
      new UserManager({
        authority: config.authority,
        client_id: config.clientId,
        redirect_uri: `${location.origin}/callback`,
        post_logout_redirect_uri: location.origin,
        response_type: 'code',
        scope: 'openid profile email',
        // tokens live in this tab's session storage only; closing the tab signs out
        userStore: new WebStorageStateStore({ store: sessionStorage }),
        automaticSilentRenew: true,
      }),
    [config],
  )
  const [session, setSession] = useState<Session | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    // once per page load: a second run (React's development double effects, or a remount) would exchange the
    // authorization code twice, and the second exchange is refused
    bootstrapping ??= bootstrap(users, config)
    bootstrapping.then(
      (s) => { if (s) setSession(s) },
      (e: unknown) => {
        setError(e instanceof ProblemError ? e.title : 'Sign-in failed. Try again or contact an administrator.')
      },
    )
  }, [users, config])

  if (error) return <div className="center"><p role="alert">{error}</p><button type="button" onClick={() => void users.signinRedirect()}>Sign in again</button></div>
  if (!session) return <div className="center"><p aria-live="polite">Signing in…</p></div>
  return (
    <SessionContext.Provider value={session}>
      <App />
    </SessionContext.Provider>
  )
}

const config = loadConfig()
const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      {config ? (
        <BrowserRouter><SignedIn config={config} /></BrowserRouter>
      ) : (
        <div className="center"><p role="alert">This build has no server or sign-in configured (VITE_API_BASE_URL, VITE_OIDC_AUTHORITY, VITE_OIDC_CLIENT_ID).</p></div>
      )}
    </StrictMode>,
  )
}
