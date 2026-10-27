import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApiClient } from './api'
import { loadConfig } from './config'
import { SignIn, type Providers } from './SignIn'

describe('config', () => {
  it('needs a server and at least one way to sign in, over https except on localhost', () => {
    expect(loadConfig({})).toBeNull()
    expect(loadConfig({ VITE_API_BASE_URL: 'https://api.example.com' })).toBeNull()
    expect(loadConfig({ VITE_API_BASE_URL: 'http://api.example.com', VITE_GOOGLE_CLIENT_ID: 'g' })).toBeNull()
    const c = loadConfig({ VITE_API_BASE_URL: 'https://api.example.com', VITE_GOOGLE_CLIENT_ID: 'g', VITE_APPLE_CLIENT_ID: 'ke.bahati.backoffice' }, 'https://bo.example.com')
    expect(c).toEqual({ apiBaseUrl: 'https://api.example.com', google: { clientId: 'g' }, apple: { clientId: 'ke.bahati.backoffice', redirectUri: 'https://bo.example.com/' } })
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
  it('offers Google and Apple and exchanges the ID token for a back-office session', async () => {
    const session = { token: 'stf_x', expires_at: '2026-10-26T21:00:00Z', member: { id: 'm1', name: 'Owner', email: 'owner@example.com', roles: ['admin'] } }
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
