import { CircleX, KeyRound, ShieldCheck } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { ProblemError, type ApiClient } from './api'
import type { Config } from './config'
import { cancelled, renderGoogle, signInWithApple, signInWithDevRealm, type IDToken } from './providers'
import type { Me } from './types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/** What POST /v1/staff/session returns. */
export interface StaffSession { token: string; expires_at: string; member: Me }

export interface Providers {
  google: (clientId: string, el: HTMLElement, onToken: (t: IDToken) => void) => Promise<void> | void
  apple: (clientId: string, redirectUri: string) => Promise<IDToken>
  dev: (dev: NonNullable<Config['dev']>) => Promise<IDToken>
}

const realProviders: Providers = { google: renderGoogle, apple: signInWithApple, dev: signInWithDevRealm }

/**
 * The sign-in page: email address and password (ADR 0019), with Google, Apple and the local staff realm offered when
 * configured (ADR 0018). The API decides who is staff and what they may do.
 */
export function SignIn({ config, api, onSignedIn, providers = realProviders, notice }: {
  config: Config
  api: ApiClient
  onSignedIn: (s: StaffSession) => void
  providers?: Providers
  notice?: string | undefined
}) {
  const id = useId()
  const google = useRef<HTMLDivElement>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<{ title: string; detail?: string | undefined } | null>(null)

  // one sign-in at a time: a second click (or Google's callback during an Apple popup) is ignored, not run alongside
  const running = useRef(false)
  const exchange = async (provider: string, body: () => Promise<Record<string, string>>) => {
    if (running.current) return
    running.current = true
    setBusy(true)
    setProblem(null)
    try {
      let b: Record<string, string>
      try {
        b = await body()
      } catch (e) {
        if (!cancelled(e)) setProblem({ title: `${provider === 'apple' ? 'Apple' : 'The sign-in window'} could not sign you in. Try again.` })
        return
      }
      try {
        onSignedIn(await api.post<StaffSession>('/v1/staff/session', { provider, ...b }))
      } catch (e) {
        setProblem(e instanceof ProblemError ? { title: e.title, detail: e.detail } : { title: 'The server could not be reached. Try again.' })
      }
    } finally {
      running.current = false
      setBusy(false)
    }
  }
  const exchangeRef = useRef(exchange)
  exchangeRef.current = exchange
  const withToken = (t: IDToken) => Promise.resolve({ id_token: t.idToken, nonce: t.nonce })

  useEffect(() => {
    const el = google.current
    if (!config.google || !el) return
    void Promise.resolve(providers.google(config.google.clientId, el, (t) => { void exchangeRef.current('google', () => withToken(t)) }))
      .catch(() => { setProblem({ title: 'Google sign-in could not load. Check the connection and reload.' }) })
  }, [config.google, providers])

  const { apple, dev } = config
  const others = Boolean(config.google || apple || dev)
  return (
    <main id="main" className="grid min-h-svh lg:grid-cols-[1.1fr_1fr]">
      <aside aria-hidden="true" className="relative hidden overflow-hidden bg-hero text-white lg:flex lg:flex-col lg:justify-between lg:p-12">
        <div className="absolute inset-0 bg-[radial-gradient(90%_70%_at_0%_0%,rgb(255_255_255_/_0.28)_0%,transparent_60%),radial-gradient(80%_60%_at_100%_100%,rgb(43_18_84_/_0.55)_0%,transparent_55%)]" />
        <div className="absolute -right-32 -top-32 size-[520px] rounded-full border border-white/15" />
        <div className="absolute -right-16 -top-16 size-[380px] rounded-full border border-white/15" />
        <div className="absolute -bottom-40 -left-24 size-[460px] rounded-full bg-white/10 blur-3xl" />
        <p className="relative font-display text-2xl font-bold tracking-tight">Tajirika</p>
        <div className="relative max-w-md">
          <p className="font-display text-4xl font-bold leading-tight tracking-tight">Every action, under your name.</p>
          <p className="mt-4 text-white/80">Identity review, draws, payouts, tax and the ledger, with two-person rules where the money moves.</p>
        </div>
        <p className="relative text-xs text-white/60">Staff back office · Nairobi time</p>
      </aside>
      <div className="flex items-center justify-center p-6 md:p-10">
        <section className="w-full max-w-[400px]" aria-labelledby="signin-title">
          <p className="mb-6 flex items-center gap-2 text-sm font-medium text-muted-foreground lg:hidden"><ShieldCheck aria-hidden="true" className="size-4 text-primary" /> Tajirika back office</p>
          <h1 id="signin-title" className="text-3xl font-bold tracking-tight">Sign in</h1>
          <p className="mt-2 text-sm text-muted-foreground">With the email address an administrator invited and your password. Every action you take is recorded under your name.</p>
          {notice ? (
            <Alert role="status" className="mt-6 border-pending/40 bg-pending/10 [&>svg]:text-pending">
              <ShieldCheck />
              <AlertTitle>{notice}</AlertTitle>
            </Alert>
          ) : null}
          <form
            className="mt-8 grid gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              if (!email.trim() || !password) return
              void exchange('password', () => Promise.resolve({ email: email.trim(), password }))
            }}
          >
            <div className="grid gap-1.5">
              <Label htmlFor={`${id}-email`}>Email address</Label>
              <Input id={`${id}-email`} type="email" inputMode="email" autoComplete="username" required value={email} onChange={(e) => { setEmail(e.target.value) }} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor={`${id}-password`}>Password</Label>
              <Input id={`${id}-password`} type="password" autoComplete="current-password" required value={password} onChange={(e) => { setPassword(e.target.value) }} />
              <p className="text-xs text-muted-foreground">Forgotten it? An administrator can give you a temporary one.</p>
            </div>
            <Button type="submit" size="lg" className="h-11 w-full" disabled={busy || !email.trim() || !password}>
              <KeyRound aria-hidden="true" /> {busy ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
          {others ? (
            <div className="mt-6 grid gap-3">
              <p className="flex items-center gap-3 text-xs uppercase tracking-wide text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">or continue with</p>
              {config.google ? <div ref={google} className="min-h-10" /> : null}
              {apple ? (
                <Button type="button" size="lg" variant="outline" disabled={busy} className="h-11 w-full border-foreground/80 bg-foreground text-background hover:bg-foreground/90 hover:text-background"
                  onClick={() => { void exchange('apple', () => providers.apple(apple.clientId, apple.redirectUri).then(withToken)) }}>
                  <svg aria-hidden="true" viewBox="0 0 14 17" width="14" height="17"><path fill="currentColor" d="M11.6 9c0-2 1.7-3 1.8-3-1-1.4-2.5-1.6-3-1.6-1.3-.1-2.5.8-3.2.8-.7 0-1.7-.8-2.8-.7C3 3.5 1.7 4.4 1 5.7c-1.5 2.6-.4 6.4 1 8.5.7 1 1.5 2.2 2.6 2.1 1 0 1.4-.7 2.7-.7 1.2 0 1.6.7 2.7.7s1.8-1 2.5-2.1c.8-1.2 1.1-2.3 1.1-2.4 0 0-2.1-.8-2.1-3.2zM9.6 3c.6-.7 1-1.7.9-2.7-.8 0-1.9.6-2.5 1.3-.5.6-1 1.6-.9 2.6.9.1 1.9-.5 2.5-1.2z" /></svg>
                  Sign in with Apple
                </Button>
              ) : null}
              {dev ? (
                <Button type="button" variant="outline" size="lg" disabled={busy} className="h-11 w-full" onClick={() => { void exchange('dev', () => providers.dev(dev).then(withToken)) }}>
                  Sign in with the local staff realm (development)
                </Button>
              ) : null}
            </div>
          ) : null}
          {busy ? <p className="mt-4 text-sm text-muted-foreground" aria-live="polite">Signing in…</p> : null}
          {problem ? (
            <Alert variant="destructive" className="mt-6">
              <CircleX />
              <AlertTitle>{problem.title}</AlertTitle>
              {problem.detail ? <AlertDescription>{problem.detail}</AlertDescription> : null}
            </Alert>
          ) : null}
        </section>
      </div>
    </main>
  )
}
