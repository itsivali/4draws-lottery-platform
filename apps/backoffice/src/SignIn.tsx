import { CircleX, KeyRound, LockKeyhole, Mail, MailCheck, ShieldCheck } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { ProblemError, type ApiClient } from './api'
import { BrandGlyph } from './components/BrandGlyph'
import { rememberCredential } from './credentials'
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
  // a forgotten password: the member asks for an emailed link (answered the same whether or not the address is known)
  const [forgot, setForgot] = useState(false)
  const [forgotSent, setForgotSent] = useState(false)
  const askForLink = async () => {
    if (busy || !email.trim()) return
    setBusy(true)
    setProblem(null)
    try {
      await api.post('/v1/staff/password/forgot', { email: email.trim() })
      setForgotSent(true)
    } catch (e) {
      if (e instanceof ProblemError && e.status === 503) setProblem({ title: 'Password reset by email is not set up.', detail: 'An administrator can give you a temporary password.' })
      else setProblem(e instanceof ProblemError ? { title: e.title, detail: e.detail } : { title: 'The server could not be reached. Try again.' })
    } finally {
      setBusy(false)
    }
  }

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
        const s = await api.post<StaffSession>('/v1/staff/session', { provider, ...b })
        if (provider === 'password') rememberCredential(b.email ?? '', b.password ?? '', s.member.name || undefined)
        onSignedIn(s)
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
        <div className="absolute -bottom-40 -left-24 size-[460px] rounded-full bg-white/10 blur-3xl" />
        <p className="relative flex items-center gap-3 font-display text-3xl font-extrabold tracking-tight"><span className="flex size-12 items-center justify-center rounded-2xl bg-white/15 shadow-float backdrop-blur"><BrandGlyph className="size-9" /></span> Tajirika</p>
        <div className="relative max-w-lg animate-in fade-in-0 slide-in-from-bottom-4 duration-700 motion-reduce:animate-none">
          <p className="font-display text-5xl font-extrabold leading-[1.05] tracking-tight">Every action, under your name.</p>
          <p className="mt-5 text-lg text-white/85">Identity review, draws, payouts, tax and the ledger, with two-person rules where the money moves.</p>
        </div>
        <p className="relative text-xs text-white/60">Staff back office · Nairobi time</p>
      </aside>
      <div className="relative flex items-center justify-center overflow-hidden bg-muted/40 p-6 md:p-10">
        <div aria-hidden="true" className="pointer-events-none absolute -right-28 -top-28 size-[380px] rounded-full bg-primary-container/70" />
        <div aria-hidden="true" className="pointer-events-none absolute -bottom-36 -left-28 size-[360px] rounded-full bg-violet-container/80" />
        <section className="relative w-full max-w-[440px] rounded-3xl border bg-card p-7 shadow-float animate-in fade-in-0 slide-in-from-bottom-4 duration-500 motion-reduce:animate-none md:p-9" aria-labelledby="signin-title">
          <div className="mb-7 flex items-center gap-3">
            <span className="flex size-11 items-center justify-center rounded-2xl bg-hero text-white shadow-card"><BrandGlyph className="size-8" /></span>
            <div className="leading-tight">
              <p className="font-display text-lg font-extrabold tracking-tight">Tajirika</p>
              <p className="text-xs text-muted-foreground">Staff back office</p>
            </div>
          </div>
          <h1 id="signin-title" className="font-display text-4xl font-extrabold tracking-tight">Sign in</h1>
          <p className="mt-2 text-sm text-muted-foreground">With the email address an administrator invited and your password. Every action you take is recorded under your name.</p>
          {notice ? (
            <Alert role="status" className="mt-6 border-pending/40 bg-pending/10 [&>svg]:text-pending">
              <ShieldCheck />
              <AlertTitle>{notice}</AlertTitle>
            </Alert>
          ) : null}
          <form
            className="mt-7 grid gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              if (!email.trim() || !password) return
              void exchange('password', () => Promise.resolve({ email: email.trim(), password }))
            }}
          >
            <div className="grid gap-1.5">
              <Label htmlFor={`${id}-email`}>Email address</Label>
              <div className="relative">
                <Mail aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input id={`${id}-email`} name="username" type="email" inputMode="email" autoComplete="username" required value={email} onChange={(e) => { setEmail(e.target.value) }} className="h-11 rounded-xl bg-muted/40 pl-10 shadow-none" />
              </div>
            </div>
            <div className="grid gap-1.5">
              <div className="flex items-baseline justify-between">
                <Label htmlFor={`${id}-password`}>Password</Label>
                {forgot ? null : (
                  <button type="button" className="text-xs font-medium text-primary underline-offset-4 hover:underline" onClick={() => { setForgot(true); setProblem(null) }}>Forgotten it?</button>
                )}
              </div>
              <div className="relative">
                <LockKeyhole aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input id={`${id}-password`} name="password" type="password" autoComplete="current-password" required value={password} onChange={(e) => { setPassword(e.target.value) }} className="h-11 rounded-xl bg-muted/40 pl-10 shadow-none" />
              </div>
            </div>
            {forgot ? (
              forgotSent ? (
                <Alert role="status" className="border-success/40 bg-success/10 [&>svg]:text-success">
                  <MailCheck />
                  <AlertTitle>If that address belongs to a staff member, a reset link is on its way.</AlertTitle>
                  <AlertDescription>It works for 30 minutes. Check the junk folder if nothing arrives.</AlertDescription>
                </Alert>
              ) : (
                <div className="grid gap-2 rounded-xl border bg-muted/40 p-3">
                  <p className="text-sm">Enter the email address above and we will email you a link to choose a new password. An administrator can also give you a temporary one.</p>
                  <div className="flex gap-2">
                    <Button type="button" size="sm" variant="outline" disabled={busy || !email.trim()} onClick={() => { void askForLink() }}>Email me a reset link</Button>
                    <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => { setForgot(false) }}>Cancel</Button>
                  </div>
                </div>
              )
            ) : null}
            <Button type="submit" size="lg" className="mt-1 h-12 w-full rounded-xl text-base shadow-card" disabled={busy || !email.trim() || !password}>
              <KeyRound aria-hidden="true" /> {busy ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
          {others ? (
            <div className="mt-6 grid gap-3">
              <p className="flex items-center gap-3 text-xs uppercase tracking-wide text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">or continue with</p>
              {config.google ? <div ref={google} className="min-h-10" /> : null}
              {apple ? (
                <Button type="button" size="lg" variant="outline" disabled={busy} className="h-11 w-full rounded-xl border-foreground/80 bg-foreground text-background hover:bg-foreground/90 hover:text-background"
                  onClick={() => { void exchange('apple', () => providers.apple(apple.clientId, apple.redirectUri).then(withToken)) }}>
                  <svg aria-hidden="true" viewBox="0 0 14 17" width="14" height="17"><path fill="currentColor" d="M11.6 9c0-2 1.7-3 1.8-3-1-1.4-2.5-1.6-3-1.6-1.3-.1-2.5.8-3.2.8-.7 0-1.7-.8-2.8-.7C3 3.5 1.7 4.4 1 5.7c-1.5 2.6-.4 6.4 1 8.5.7 1 1.5 2.2 2.6 2.1 1 0 1.4-.7 2.7-.7 1.2 0 1.6.7 2.7.7s1.8-1 2.5-2.1c.8-1.2 1.1-2.3 1.1-2.4 0 0-2.1-.8-2.1-3.2zM9.6 3c.6-.7 1-1.7.9-2.7-.8 0-1.9.6-2.5 1.3-.5.6-1 1.6-.9 2.6.9.1 1.9-.5 2.5-1.2z" /></svg>
                  Sign in with Apple
                </Button>
              ) : null}
              {dev ? (
                <Button type="button" variant="outline" size="lg" disabled={busy} className="h-11 w-full rounded-xl" onClick={() => { void exchange('dev', () => providers.dev(dev).then(withToken)) }}>
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
          <p className="mt-7 text-center text-xs text-muted-foreground">Staff back office · Nairobi time</p>
        </section>
      </div>
    </main>
  )
}
