import { describe, expect, it, vi } from 'vitest'
import { ApiClient, ProblemError, UncertainError } from './api'

function respond(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { 'content-type': status >= 400 ? 'application/problem+json' : 'application/json', ...headers },
  })
}

describe('ApiClient', () => {
  it('sends the staff token and an idempotency key on every write', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(respond(200, { status: 'verified' }))
    const api = new ApiClient('https://api.test', () => Promise.resolve('tok'), fetch)
    await api.post('/v1/staff/kyc/u1/approve', { reason: 'matches' })
    const [url, init] = fetch.mock.calls[0] ?? []
    expect(url).toBe('https://api.test/v1/staff/kyc/u1/approve')
    const headers = new Headers(init?.headers)
    expect(headers.get('Authorization')).toBe('Bearer tok')
    expect(headers.get('Idempotency-Key')).toMatch(/^bo-[0-9a-f]{32}$/)
    expect(init?.body).toBe(JSON.stringify({ reason: 'matches' }))
  })

  it('turns problem+json into a ProblemError with the server title', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(
      respond(409, { type: 'https://lottery.example/problems/two-person-rule', title: 'Someone else must record it', status: 409 }),
    )
    const api = new ApiClient('https://api.test', () => Promise.resolve('tok'), fetch)
    const err = await api.post('/v1/staff/tax/batches/1/remit', {}).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ProblemError)
    expect((err as ProblemError).slug).toBe('two-person-rule')
    expect((err as ProblemError).title).toBe('Someone else must record it')
  })

  it('retries a lost write once with the same key, then reports it as uncertain', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockRejectedValue(new TypeError('network'))
    const api = new ApiClient('https://api.test', () => Promise.resolve('tok'), fetch)
    const err = await api.post('/v1/staff/payouts/p1/approve', {}).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(UncertainError)
    const keys = fetch.mock.calls.map(([, init]) => new Headers(init?.headers).get('Idempotency-Key'))
    expect(keys).toHaveLength(2)
    expect(new Set(keys).size).toBe(1)
  })

  it('treats a 5xx on a write as uncertain, not as failed', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(respond(502, undefined))
    const api = new ApiClient('https://api.test', () => Promise.resolve('tok'), fetch)
    await expect(api.post('/v1/staff/draws/d1/sign', {})).rejects.toBeInstanceOf(UncertainError)
  })

  it('reads lists without a key', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(respond(200, { alerts: [] }))
    const api = new ApiClient('https://api.test', () => Promise.resolve('tok'), fetch)
    expect(await api.get<{ alerts: unknown[] }>('/v1/staff/alerts', { all: 'true' })).toEqual({ alerts: [] })
    const [url, init] = fetch.mock.calls[0] ?? []
    expect(url).toBe('https://api.test/v1/staff/alerts?all=true')
    expect(new Headers(init?.headers).has('Idempotency-Key')).toBe(false)
  })

  it('reports an expired session so the app can sign in again', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(
      respond(401, { type: 'https://lottery.example/problems/unauthenticated', title: 'Staff authentication required', status: 401 }),
    )
    const api = new ApiClient('https://api.test', () => Promise.resolve('tok'), fetch)
    const err = await api.get('/v1/staff/me').catch((e: unknown) => e)
    expect((err as ProblemError).status).toBe(401)
  })
})
