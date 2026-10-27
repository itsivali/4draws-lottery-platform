// The sign-in providers' own scripts: Google Identity Services and Sign in with Apple JS. Each returns an ID token and
// the nonce it was bound to; the API checks both (ADR 0018). Nothing here decides who may sign in: the API does.
import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import type { Config } from './config'

export interface IDToken { idToken: string; nonce: string }

function randomNonce(): string {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

async function sha256Hex(s: string): Promise<string> {
  const d = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(s))
  return Array.from(new Uint8Array(d), (x) => x.toString(16).padStart(2, '0')).join('')
}

const loaded = new Map<string, Promise<void>>()

function loadScript(src: string): Promise<void> {
  let p = loaded.get(src)
  if (!p) {
    p = new Promise((resolve, reject) => {
      const s = document.createElement('script')
      s.src = src
      s.async = true
      s.onload = () => { resolve() }
      s.onerror = () => { reject(new Error(`could not load ${src}`)) }
      document.head.append(s)
    })
    loaded.set(src, p)
  }
  return p
}

interface GoogleAccounts {
  id: {
    initialize(o: { client_id: string; nonce: string; callback: (r: { credential: string }) => void; ux_mode: 'popup'; auto_select: boolean }): void
    renderButton(el: HTMLElement, o: { type: 'standard'; theme: 'outline'; size: 'large'; text: 'signin_with'; width: number; logo_alignment: 'left' }): void
  }
}

interface AppleIDAuth {
  init(o: { clientId: string; scope: string; redirectURI: string; usePopup: boolean; nonce: string }): void
  signIn(): Promise<{ authorization: { id_token: string } }>
}

declare global {
  interface Window { google?: { accounts: GoogleAccounts }; AppleID?: { auth: AppleIDAuth } }
}

/** Google renders its own button into el and calls back with each ID token. */
export async function renderGoogle(clientId: string, el: HTMLElement, onToken: (t: IDToken) => void): Promise<void> {
  await loadScript('https://accounts.google.com/gsi/client')
  const g = window.google?.accounts
  if (!g) throw new Error('Google sign-in did not load')
  const nonce = randomNonce()
  g.id.initialize({ client_id: clientId, nonce, ux_mode: 'popup', auto_select: false, callback: (r) => { onToken({ idToken: r.credential, nonce }) } })
  g.id.renderButton(el, { type: 'standard', theme: 'outline', size: 'large', text: 'signin_with', width: 280, logo_alignment: 'left' })
}

/** Apple opens its own window; Apple puts the SHA-256 of the nonce in the token (the API hashes what it is given). */
export async function signInWithApple(clientId: string, redirectUri: string): Promise<IDToken> {
  await loadScript('https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js')
  const a = window.AppleID?.auth
  if (!a) throw new Error('Sign in with Apple did not load')
  const nonce = randomNonce()
  a.init({ clientId, scope: 'email', redirectURI: redirectUri, usePopup: true, nonce: await sha256Hex(nonce) })
  const r = await a.signIn()
  return { idToken: r.authorization.id_token, nonce }
}

/** The local staff realm, in development only: a popup to Keycloak, whose ID token carries no nonce we check. */
export async function signInWithDevRealm(dev: NonNullable<Config['dev']>): Promise<IDToken> {
  const users = new UserManager({
    authority: dev.authority, client_id: dev.clientId, redirect_uri: `${location.origin}/callback`, response_type: 'code',
    scope: 'openid email profile', userStore: new WebStorageStateStore({ store: sessionStorage }),
  })
  const user = await users.signinPopup()
  if (!user.id_token) throw new Error('the local staff realm returned no ID token')
  return { idToken: user.id_token, nonce: '' }
}

/** The popup's own page: hands the result back to the window that opened it. */
export async function completeDevPopup(dev: NonNullable<Config['dev']>): Promise<void> {
  const users = new UserManager({ authority: dev.authority, client_id: dev.clientId, redirect_uri: `${location.origin}/callback`,
    response_type: 'code', userStore: new WebStorageStateStore({ store: sessionStorage }) })
  await users.signinPopupCallback()
}

/** Errors that mean the person closed the provider's window: not a failure to report. */
export function cancelled(e: unknown): boolean {
  const s = e instanceof Error ? e.message : typeof e === 'object' && e && 'error' in e ? String((e as { error: unknown }).error) : ''
  return /popup_closed|closed by user|user_cancelled|user_trigger_new_signin_flow/i.test(s)
}
