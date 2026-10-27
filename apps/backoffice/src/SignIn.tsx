import { useEffect, useRef, useState } from 'react'
import { ProblemError, type ApiClient } from './api'
import type { Config } from './config'
import { cancelled, renderGoogle, signInWithApple, signInWithDevRealm, type IDToken } from './providers'
import type { Me } from './types'

/** What POST /v1/staff/session returns. */
export interface StaffSession { token: string; expires_at: string; member: Me }

export interface Providers {
  google: (clientId: string, el: HTMLElement, onToken: (t: IDToken) => void) => Promise<void> | void
  apple: (clientId: string, redirectUri: string) => Promise<IDToken>
  dev: (dev: NonNullable<Config['dev']>) => Promise<IDToken>
}

const realProviders: Providers = { google: renderGoogle, apple: signInWithApple, dev: signInWithDevRealm }

/** The sign-in page: Google and Apple, and the local staff realm in development. The API decides who is staff. */
export function SignIn({ config, api, onSignedIn, providers = realProviders, notice }: {
  config: Config
  api: ApiClient
  onSignedIn: (s: StaffSession) => void
  providers?: Providers
  notice?: string | undefined
}) {
  const google = useRef<HTMLDivElement>(null)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<{ title: string; detail?: string | undefined } | null>(null)

  const exchange = async (provider: string, get: () => Promise<IDToken>) => {
    setProblem(null)
    let t: IDToken
    try {
      t = await get()
    } catch (e) {
      if (!cancelled(e)) setProblem({ title: `${provider === 'apple' ? 'Apple' : 'The sign-in window'} could not sign you in. Try again.` })
      return
    }
    setBusy(true)
    try {
      onSignedIn(await api.post<StaffSession>('/v1/staff/session', { provider, id_token: t.idToken, nonce: t.nonce }))
    } catch (e) {
      setProblem(e instanceof ProblemError ? { title: e.title, detail: e.detail } : { title: 'The server could not be reached. Try again.' })
    } finally {
      setBusy(false)
    }
  }
  const exchangeRef = useRef(exchange)
  exchangeRef.current = exchange

  useEffect(() => {
    const el = google.current
    if (!config.google || !el) return
    void Promise.resolve(providers.google(config.google.clientId, el, (t) => { void exchangeRef.current('google', () => Promise.resolve(t)) }))
      .catch(() => { setProblem({ title: 'Google sign-in could not load. Check the connection and reload.' }) })
  }, [config.google, providers])

  const { apple, dev } = config
  return (
    <main className="signin" id="main">
      <section className="signin-card" aria-labelledby="signin-title">
        <p className="brand">Back office</p>
        <h1 id="signin-title">Sign in</h1>
        <p className="muted">Use the Google or Apple account an admin invited. Every action you take is recorded under your name.</p>
        {notice ? <p className="notice notice-pending" role="status">{notice}</p> : null}
        <div className="signin-buttons">
          {config.google ? <div ref={google} className="google-button" /> : null}
          {apple ? (
            <button type="button" className="apple-button" disabled={busy}
              onClick={() => { void exchange('apple', () => providers.apple(apple.clientId, apple.redirectUri)) }}>
              <svg aria-hidden="true" viewBox="0 0 14 17" width="14" height="17"><path fill="currentColor" d="M11.6 9c0-2 1.7-3 1.8-3-1-1.4-2.5-1.6-3-1.6-1.3-.1-2.5.8-3.2.8-.7 0-1.7-.8-2.8-.7C3 3.5 1.7 4.4 1 5.7c-1.5 2.6-.4 6.4 1 8.5.7 1 1.5 2.2 2.6 2.1 1 0 1.4-.7 2.7-.7 1.2 0 1.6.7 2.7.7s1.8-1 2.5-2.1c.8-1.2 1.1-2.3 1.1-2.4 0 0-2.1-.8-2.1-3.2zM9.6 3c.6-.7 1-1.7.9-2.7-.8 0-1.9.6-2.5 1.3-.5.6-1 1.6-.9 2.6.9.1 1.9-.5 2.5-1.2z" /></svg>
              Sign in with Apple
            </button>
          ) : null}
          {dev ? (
            <button type="button" className="secondary" disabled={busy} onClick={() => { void exchange('dev', () => providers.dev(dev)) }}>
              Sign in with the local staff realm (development)
            </button>
          ) : null}
        </div>
        {busy ? <p className="muted" aria-live="polite">Signing in…</p> : null}
        {problem ? (
          <div className="notice notice-error" role="alert">
            <p><strong>{problem.title}</strong></p>
            {problem.detail ? <p>{problem.detail}</p> : null}
          </div>
        ) : null}
      </section>
    </main>
  )
}
