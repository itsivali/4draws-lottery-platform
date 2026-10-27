import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { renderSignedIn } from './test/render'
import type { FinanceSummary, StaffMember } from './types'

const zero = { deposits_kes: 0, stakes_kes: 0, prizes_awarded_kes: 0, prizes_paid_kes: 0, withdrawals_kes: 0, reversals_kes: 0, excise_kes: 0, tax_withheld_kes: 0, tax_remitted_kes: 0, ggr_kes: 0 }
const summary: FinanceSummary = {
  from: '2026-10-25',
  to: '2026-10-26',
  totals: { ...zero, deposits_kes: 15000, stakes_kes: 4200, prizes_awarded_kes: 1260, prizes_paid_kes: 900, withdrawals_kes: 2500, excise_kes: 750, tax_withheld_kes: 180, ggr_kes: 2940 },
  days: [
    { day: '2026-10-25', ...zero, deposits_kes: 10000, stakes_kes: 3000, ggr_kes: 2100 },
    { day: '2026-10-26', ...zero, deposits_kes: 5000, stakes_kes: 1200, prizes_paid_kes: 900, withdrawals_kes: 2500, ggr_kes: 840 },
  ],
  balances: { owed_to_players_kes: 9800, player_debt_kes: 60, excise_owed_kes: 750, tax_owed_kes: 180, prize_pools_kes: 360, suspense_kes: 0, mpesa_clearing_kes: 15000, b2c_float_kes: -3400 },
  activity: { signups: 12, plays: 42, players: 9 },
}

describe('finance', () => {
  it('shows the range’s figures, what is owed, and every day in a table', async () => {
    const { fetch } = renderSignedIn(<App />, {
      roles: ['finance'],
      path: '/finance',
      route: (_m, url) => (url.pathname === '/v1/staff/finance' ? { status: 200, body: summary } : { status: 404 }),
    })
    expect(await screen.findByRole('heading', { name: 'Finance' })).toBeInTheDocument()
    const deposits = screen.getByRole('group', { name: 'Deposits' })
    expect(deposits).toHaveTextContent('KES 15,000')
    expect(screen.getByRole('group', { name: 'Gross gaming revenue' })).toHaveTextContent('KES 2,940')
    expect(screen.getByRole('group', { name: 'Owed to players' })).toHaveTextContent('KES 9,800')
    expect(screen.getByRole('group', { name: 'Excise owed to KRA' })).toHaveTextContent('KES 750')
    expect(screen.getByRole('img', { name: /Deposits, stakes, prizes paid and withdrawals per day/ })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: /Per day/ })
    expect(within(table).getAllByRole('row')).toHaveLength(3)
    expect(within(table).getByRole('row', { name: /26 Oct/ })).toHaveTextContent('2,500')
    // another range asks the server again
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Last 7 days' }))
    expect(fetch.mock.calls.some(([u]) => (u as string).includes('from='))).toBe(true)
  })
})

const members: StaffMember[] = [
  { id: 's1', email: 'alice@staff.local', name: 'Alice', roles: ['admin'], active: true, invited_by: 'bootstrap', providers: ['google'], last_sign_in: '2026-10-26T09:00:00Z', created_at: '2026-10-26T08:00:00Z' },
  { id: 's2', email: 'kim@staff.local', name: 'Kim', roles: ['kyc_reviewer'], active: true, invited_by: 'staff:s1', providers: [], last_sign_in: null, created_at: '2026-10-26T08:00:00Z' },
]

describe('staff', () => {
  it('an admin invites a member with roles, and changes another member’s roles', async () => {
    let list = [...members]
    const { fetch } = renderSignedIn(<App />, {
      roles: ['admin'],
      path: '/staff',
      route: (method, url, body) => {
        if (method === 'GET' && url.pathname === '/v1/staff/members') return { status: 200, body: { members: list } }
        if (method === 'POST' && url.pathname === '/v1/staff/members') {
          const b = body as { email: string; name: string; roles: StaffMember['roles'] }
          const m = { ...(members[1] as StaffMember), id: 's3', email: b.email, name: b.name, roles: b.roles }
          list = [...list, m]
          return { status: 201, body: m }
        }
        if (method === 'PUT' && url.pathname === '/v1/staff/members/s2/roles') {
          list = list.map((m) => (m.id === 's2' ? { ...m, roles: (body as { roles: StaffMember['roles'] }).roles } : m))
          return { status: 200, body: list[1] }
        }
        return { status: 404 }
      },
    })
    const user = userEvent.setup()
    expect(await screen.findByRole('row', { name: /kim@staff.local/ })).toHaveTextContent('Never signed in')
    await user.click(screen.getByRole('button', { name: 'Invite a member' }))
    const dialog = screen.getByRole('dialog', { name: 'Invite a member' })
    await user.type(within(dialog).getByLabelText(/^Email address/), 'wanjiru@example.com')
    await user.type(within(dialog).getByLabelText(/^Name/), 'Wanjiru')
    await user.click(within(dialog).getByRole('checkbox', { name: 'Finance' }))
    await user.click(within(dialog).getByRole('button', { name: 'Invite' }))
    expect(await screen.findByRole('row', { name: /wanjiru@example.com/ })).toBeInTheDocument()
    const invite = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(invite?.[1]?.body as string)).toEqual({ email: 'wanjiru@example.com', name: 'Wanjiru', roles: ['finance'] })

    await user.click(within(screen.getByRole('row', { name: /kim@staff.local/ })).getByRole('button', { name: 'Change roles' }))
    const roles = screen.getByRole('dialog', { name: 'Roles of kim@staff.local' })
    await user.click(within(roles).getByRole('checkbox', { name: 'Auditor' }))
    await user.click(within(roles).getByRole('button', { name: 'Save roles' }))
    expect(await screen.findByRole('row', { name: /kim@staff.local/ })).toHaveTextContent('Auditor')
    // nobody changes their own roles
    expect(within(screen.getByRole('row', { name: /alice@staff.local/ })).queryByRole('button', { name: 'Change roles' })).not.toBeInTheDocument()
  })
})
