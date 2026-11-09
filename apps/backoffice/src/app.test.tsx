import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { visibleSections } from './roles'
import { problem, renderSignedIn } from './test/render'
import type { Draw } from './types'

const submission = { user_id: 'u1', msisdn: '254712345678', national_id: '12345678', full_name: 'Amina Otieno', date_of_birth: '1992-03-04', submitted_at: '2026-10-26T09:00:00Z' }

describe('roles', () => {
  it('a member sees only the sections their roles open', () => {
    expect(visibleSections(['staff']).map((s) => s.path)).toEqual(['/finance', '/players', '/plays', '/topups', '/kyc', '/draws', '/payouts', '/payments', '/tax', '/alerts', '/compliance', '/system', '/closures'])
    expect(visibleSections(['admin']).map((s) => s.path)).toEqual(['/finance', '/metrics', '/players', '/plays', '/topups', '/kyc', '/draws', '/payouts', '/payments', '/tax', '/alerts', '/compliance', '/system', '/activity', '/closures', '/staff'])
  })

  it('the navigation and the routes follow the roles', async () => {
    renderSignedIn(<App />, { roles: ['staff'], route: () => ({ status: 200, body: { submissions: [] } }), path: '/staff' })
    const nav = screen.getByRole('navigation', { name: 'Back office' })
    expect(within(nav).getByRole('link', { name: 'Identity review' })).toBeInTheDocument()
    expect(within(nav).getByRole('link', { name: 'Payouts' })).toBeInTheDocument()
    expect(within(nav).queryByRole('link', { name: 'Staff' })).not.toBeInTheDocument()
    expect(within(nav).queryByRole('link', { name: 'App metrics' })).not.toBeInTheDocument()
    // a forbidden address (the admin-only Staff page) goes back to the overview
    expect(await screen.findByRole('heading', { name: /Signed in as Alice/ })).toBeInTheDocument()
  })
})

describe('identity review', () => {
  it('approving needs a reason, sends it, and reloads the queue', async () => {
    let pending = [submission]
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/kyc',
      route: (method, url) => {
        if (method === 'GET' && url.pathname === '/v1/staff/kyc/pending') return { status: 200, body: { submissions: pending } }
        if (method === 'POST' && url.pathname === '/v1/staff/kyc/u1/approve') {
          pending = []
          return { status: 200, body: { status: 'verified' } }
        }
        return { status: 404 }
      },
    })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    const dialog = screen.getByRole('dialog', { name: 'Approve Amina Otieno?' })
    const confirm = within(dialog).getByRole('button', { name: 'Approve' })
    expect(confirm).toBeDisabled()
    await user.type(within(dialog).getByLabelText(/What you checked/), 'ID card seen; details match')
    await user.click(confirm)
    expect(await screen.findByRole('status')).toHaveTextContent('Amina Otieno is verified.')
    await waitFor(() => { expect(screen.getByText('No submission is waiting for review.')).toBeInTheDocument() })
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(post?.[1]?.body).toBe(JSON.stringify({ reason: 'ID card seen; details match' }))
  })

  it('marks the submission an email linked to, and says when it is no longer waiting', async () => {
    const route = (method: string, url: URL) =>
      method === 'GET' && url.pathname === '/v1/staff/kyc/pending' ? { status: 200, body: { submissions: [submission] } } : { status: 404 }
    const first = renderSignedIn(<App />, { roles: ['staff'], path: '/kyc?user=u1', route })
    expect(await screen.findByText('From your email')).toBeInTheDocument()
    expect(screen.queryByText(/no longer waiting/)).not.toBeInTheDocument()
    first.unmount()
    renderSignedIn(<App />, { roles: ['staff'], path: '/kyc?user=u9', route })
    expect(await screen.findByText('The submission from your email is no longer waiting.')).toBeInTheDocument()
    expect(screen.queryByText('From your email')).not.toBeInTheDocument()
  })
})

const frozen: Draw = {
  id: 'd1', draw_type: 'daily', period_start: '2026-10-25', period_end: '2026-10-25', basis: 'calendar', status: 'frozen', freeze_at: '2026-10-26T00:05:00+03:00',
  entries: 42, drand_round_at: '2026-10-26T09:00:00+03:00', voided_reason: null, results: [],
  signoffs: [{ step: 'sign', attestation: 'a', signed_at: '2026-10-26T00:10:00Z' }],
}

describe('draws', () => {
  it('shows how many officials have signed, and the two-person refusal in the server’s words', async () => {
    renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/draws',
      route: (method, url) => {
        if (url.pathname === '/v1/staff/draws') return { status: 200, body: { draws: [frozen] } }
        if (url.pathname === '/v1/staff/draws/overdue') return { status: 200, body: { overdue: [] } }
        if (method === 'POST') return problem(409, 'already-signed', 'You have already signed this step; a second official must')
        return { status: 404 }
      },
    })
    expect(await screen.findByText('1 of 2 signatures')).toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Sign' }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Sign' }))
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('You have already signed this step; a second official must')
    expect(alert).toHaveTextContent('Nothing was changed.')
  })

  it("a personal draw says whose period it holds", async () => {
    const personal: Draw = { ...frozen, id: 'd2', draw_type: 'weekly', period_start: '2026-10-28', period_end: '2026-11-03', basis: 'personal' }
    renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/draws',
      route: (_m, url) => (url.pathname === '/v1/staff/draws' ? { status: 200, body: { draws: [personal] } } : { status: 200, body: { overdue: [] } }),
    })
    expect(await screen.findByText('players whose own week ends 2026-11-03')).toBeInTheDocument()
  })
})

describe('payouts', () => {
  it('a lost answer is shown as unknown and checked again with the same key', async () => {
    let calls = 0
    const held = { id: 'p1', user_id: 'u', kind: 'withdrawal', status: 'processing', state: 'held', gross_kes: 120000, tax_kes: 0, net_kes: 120000, debt_repaid_kes: 0, created_at: '2026-10-26T09:00:00Z' }
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/payouts',
      route: (method) => {
        if (method === 'GET') return { status: 200, body: { payouts: [held] } }
        calls++
        if (calls <= 2) return Promise.reject(new TypeError('network'))
        return { status: 200, body: { ...held, state: 'pending' } }
      },
    })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Approve' }))
    expect(await screen.findByText(/No answer from the server/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Check again' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Confirmed by the server.')
    const keys = fetch.mock.calls.filter(([, i]) => i?.method === 'POST').map(([, i]) => new Headers(i?.headers).get('Idempotency-Key'))
    expect(keys).toHaveLength(3)
    expect(new Set(keys).size).toBe(1)
  })
})

describe('closures', () => {
  it('a refused closure explains why (money still in the account)', async () => {
    renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/closures',
      route: (method) =>
        method === 'GET'
          ? { status: 200, body: { closures: [{ user_id: 'u1', msisdn: '254712345678', status: 'active', requested_at: '2026-10-26T09:00:00Z' }] } }
          : { status: 409, body: { type: 'https://lottery.example/problems/closure-blocked', title: 'The account cannot be closed yet', detail: 'the wallet still holds KES 950; pay it out first', status: 409 } },
    })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Close account' }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close account' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('the wallet still holds KES 950')
  })
})

