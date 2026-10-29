import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApiClient } from './api'
import { loadConfig } from './config'
import { SignIn, type Providers } from './SignIn'

describe('config', () => {
  it('needs a server over https (http on localhost); Google, Apple and the dev realm are optional', () => {
    expect(loadConfig({})).toBeNull()
    expect(loadConfig({ VITE_API_BASE_URL: 'https://api.example.com' })).toEqual({ apiBaseUrl: 'https://api.example.com' })
    expect(loadConfig({ VITE_API_BASE_URL: 'http://api.example.com', VITE_GOOGLE_CLIENT_ID: 'g' })).toBeNull()
    const c = loadConfig({ VITE_API_BASE_URL: 'https://api.example.com', VITE_GOOGLE_CLIENT_ID: 'g', VITE_APPLE_CLIENT_ID: 'ke.tajirika.backoffice' }, 'https://bo.example.com')
    expect(c).toEqual({ apiBaseUrl: 'https://api.example.com', google: { clientId: 'g' }, apple: { clientId: 'ke.tajirika.backoffice', redirectUri: 'https://bo.example.com/' } })
    expect(loadConfig({ VITE_API_BASE_URL: 'http://localhost:8080', VITE_DEV_OIDC_AUTHORITY: 'http://localhost:8081/realms/lottery-staff', VITE_DEV_OIDC_CLIENT_ID: 'lottery-backoffice' })?.dev)
      .toEqual({ authority: 'http://localhost:8081/realms/lottery-staff', clientId: 'lottery-backoffice' })
  })
})

function setup(route: (body: unknown) => { status: number; body?: unknown }, providers: Partial<Providers> = {}) {
  const fetch = vi.fn<typeof globalThis.fetch>((_input, init) => {
    const r = route(typeof init?.body === 'string' ? JSON.parse(init.body) : undefined)
    return Promise.resolve(new Response(r.body === undefined ? null : JSON.stringify(r.body), {
      status: r.status, headers: { 'content-type': r.status >= 400 ? 'application/problem+json' : 'application/json' },
    }))
  })
  const api = new ApiClient('https://api.test', () => Promise.resolve(null), fetch)
  const onSignedIn = vi.fn()
  const all: Providers = {
    google: vi.fn(),
    apple: vi.fn(() => Promise.resolve({ idToken: 'apple-id-token', nonce: 'raw-nonce' })),
    dev: vi.fn(() => Promise.resolve({ idToken: 'dev-id-token', nonce: '' })),
    ...providers,
  }
  render(<SignIn config={{ apiBaseUrl: 'https://api.test', google: { clientId: 'g' }, apple: { clientId: 'a', redirectUri: 'https://bo.test/' } }}
    api={api} providers={all} onSignedIn={onSignedIn} />)
  return { fetch, onSignedIn, all }
}

describe('sign in', () => {
  it('signs in with an email address and password, and says so in the server’s words when refused', async () => {
    const session = { token: 'stf_x', expires_at: '2026-10-26T21:00:00Z', member: { id: 'm1', name: 'Owner', email: 'owner@example.com', roles: ['admin'], must_change_password: true } }
    let attempts = 0
    const { fetch, onSignedIn } = setup((body) => {
      attempts++
      const b = body as { provider: string; email: string; password: string }
      if (b.provider !== 'password') return { status: 400 }
      return b.password === 'the right one here'
        ? { status: 201, body: session }
        : { status: 401, body: { type: 'https://lottery.example/problems/invalid-credentials', title: 'Wrong email address or password', status: 401 } }
    })
    const store = vi.fn(() => Promise.resolve())
    class PasswordCredential { constructor(public init: { id: string; password: string; name?: string }) {} }
    Object.assign(window, { PasswordCredential })
    Object.defineProperty(navigator, 'credentials', { value: { store }, configurable: true })
    const user = userEvent.setup()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled()
    await user.type(screen.getByLabelText('Email address'), ' Owner@Example.com ')
    await user.type(screen.getByLabelText('Password'), 'not it')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Wrong email address or password')
    expect(onSignedIn).not.toHaveBeenCalled()
    await user.clear(screen.getByLabelText('Password'))
    await user.type(screen.getByLabelText('Password'), 'the right one here')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(onSignedIn).toHaveBeenCalledWith(session)
    expect(attempts).toBe(2)
    // the browser's password manager is offered the credential once the server accepted it (not after the refusal)
    expect(store).toHaveBeenCalledTimes(1)
    expect((store.mock.calls[0] as unknown as [PasswordCredential])[0].init).toEqual({ id: 'Owner@Example.com', password: 'the right one here', name: 'Owner' })
    const [, init] = fetch.mock.calls[1] ?? []
    expect(JSON.parse(init?.body as string)).toEqual({ provider: 'password', email: 'Owner@Example.com', password: 'the right one here' })
  })

  it('emails a reset link to a member who forgot their password, and never says whether the address is known', async () => {
    const { fetch } = setup(() => ({ status: 202 }))
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Email address'), 'owner@example.com')
    await user.click(screen.getByRole('button', { name: 'Forgotten it?' }))
    await user.click(screen.getByRole('button', { name: 'Email me a reset link' }))
    expect(await screen.findByRole('status')).toHaveTextContent('If that address belongs to a staff member, a reset link is on its way.')
    const [url, init] = fetch.mock.calls[0] ?? []
    expect(url).toBe('https://api.test/v1/staff/password/forgot')
    expect(JSON.parse(init?.body as string)).toEqual({ email: 'owner@example.com' })
  })

  it('offers Google and Apple and exchanges the ID token for a back-office session', async () => {
    const session = { token: 'stf_x', expires_at: '2026-10-26T21:00:00Z', member: { id: 'm1', name: 'Owner', email: 'owner@example.com', roles: ['admin'], must_change_password: false } }
    const { fetch, onSignedIn, all } = setup(() => ({ status: 201, body: session }))
    expect(all.google).toHaveBeenCalled() // Google renders its own button into the page
    await userEvent.setup().click(screen.getByRole('button', { name: 'Sign in with Apple' }))
    expect(onSignedIn).toHaveBeenCalledWith(session)
    const [, init] = fetch.mock.calls[0] ?? []
    expect(JSON.parse(init?.body as string)).toEqual({ provider: 'apple', id_token: 'apple-id-token', nonce: 'raw-nonce' })
    expect(screen.queryByRole('button', { name: /local staff realm/ })).not.toBeInTheDocument()
  })

  it('explains a refusal in the server’s words', async () => {
    const { onSignedIn } = setup(() => ({
      status: 403,
      body: { type: 'https://lottery.example/problems/not-invited', title: 'This account is not a staff member', detail: 'ask an admin to invite the email address', status: 403 },
    }))
    await userEvent.setup().click(screen.getByRole('button', { name: 'Sign in with Apple' }))
    const alert = await screen.findByRole('alert')
    expect(within(alert).getByText('This account is not a staff member')).toBeInTheDocument()
    expect(alert).toHaveTextContent('ask an admin to invite the email address')
    expect(onSignedIn).not.toHaveBeenCalled()
  })

  it('a cancelled Apple window is not an error', async () => {
    const { fetch } = setup(() => ({ status: 201 }), { apple: vi.fn(() => Promise.reject(new Error('popup_closed_by_user'))) })
    await userEvent.setup().click(screen.getByRole('button', { name: 'Sign in with Apple' }))
    expect(fetch).not.toHaveBeenCalled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
