import { render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import { ApiClient } from '../api'
import { SessionContext } from '../context'
import type { Role } from '../types'

export type Route = (method: string, url: URL, body: unknown) => { status: number; body?: unknown } | Promise<never>

/** Renders ui inside a signed-in session whose server answers with route. */
export function renderSignedIn(ui: ReactNode, { roles, route, path = '/' }: { roles: Role[]; route: Route; path?: string }) {
  const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) => {
    const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url)
    const body: unknown = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
    const r = await route(init?.method ?? 'GET', url, body)
    const problem = r.status >= 400
    return new Response(r.body === undefined ? null : JSON.stringify(r.body), {
      status: r.status,
      headers: { 'content-type': problem ? 'application/problem+json' : 'application/json' },
    })
  })
  const api = new ApiClient('https://api.test', () => Promise.resolve('staff-token'), fetch)
  const session = { me: { id: 's1', name: 'Alice', email: 'alice@staff.local', roles }, api, signOut: vi.fn() }
  const view = render(
    <MemoryRouter initialEntries={[path]}>
      <SessionContext.Provider value={session}>{ui}</SessionContext.Provider>
    </MemoryRouter>,
  )
  return { ...view, fetch, session }
}

export const problem = (status: number, slug: string, title: string) => ({
  status,
  body: { type: `https://lottery.example/problems/${slug}`, title, status },
})
