import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { DrawsPage } from './pages/Draws'
import { PayoutsPage } from './pages/Payouts'
import { PlayersPage } from './pages/Players'
import { renderSignedIn } from './test/render'
import type { Draw, FinanceSummary, Player, StaffMember } from './types'

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
      roles: ['staff'],
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
  { id: 's1', email: 'alice@staff.local', name: 'Alice', roles: ['admin'], active: true, invited_by: 'bootstrap', providers: ['google'], last_sign_in: '2026-10-26T09:00:00Z', created_at: '2026-10-26T08:00:00Z', has_password: true, must_change_password: false },
  { id: 's2', email: 'kim@staff.local', name: 'Kim', roles: ['staff'], active: true, invited_by: 'staff:s1', providers: [], last_sign_in: null, created_at: '2026-10-26T08:00:00Z', has_password: true, must_change_password: true },
]

describe('staff', () => {
  it('an admin invites a member with a role and a temporary password, resets passwords, and changes another member’s role', async () => {
    let list = [...members]
    const { fetch } = renderSignedIn(<App />, {
      roles: ['admin'],
      path: '/staff',
      route: (method, url, body) => {
        if (method === 'GET' && url.pathname === '/v1/staff/members') return { status: 200, body: { members: list } }
        if (method === 'POST' && url.pathname === '/v1/staff/members') {
          const b = body as { email: string; name: string; roles: StaffMember['roles']; password?: string }
          const m = { ...(members[1] as StaffMember), id: 's3', email: b.email, name: b.name, roles: b.roles, has_password: b.password !== undefined }
          list = [...list, m]
          return { status: 201, body: m }
        }
        if (method === 'PUT' && url.pathname === '/v1/staff/members/s2/roles') {
          list = list.map((m) => (m.id === 's2' ? { ...m, roles: (body as { roles: StaffMember['roles'] }).roles } : m))
          return { status: 200, body: list[1] }
        }
        if (method === 'POST' && url.pathname === '/v1/staff/members/s2/password') return { status: 204 }
        return { status: 404 }
      },
    })
    const user = userEvent.setup()
    const kim = await screen.findByRole('row', { name: /kim@staff.local/ })
    expect(kim).toHaveTextContent('Never signed in')
    expect(kim).toHaveTextContent('Must change password')
    await user.click(screen.getByRole('button', { name: 'Invite a member' }))
    const dialog = screen.getByRole('dialog', { name: 'Invite a member' })
    await user.type(within(dialog).getByLabelText(/^Email address/), 'wanjiru@example.com')
    await user.type(within(dialog).getByLabelText(/^Name/), 'Wanjiru')
    await user.click(within(dialog).getByRole('radio', { name: /^Administrator/ }))
    await user.type(within(dialog).getByLabelText(/^Temporary password/), 'welcome wanjiru 2026')
    await user.click(within(dialog).getByRole('button', { name: 'Invite' }))
    expect(await screen.findByRole('row', { name: /wanjiru@example.com/ })).toHaveTextContent('Administrator')
    const invite = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(invite?.[1]?.body as string)).toEqual({ email: 'wanjiru@example.com', name: 'Wanjiru', roles: ['admin'], password: 'welcome wanjiru 2026' })

    await user.click(within(screen.getByRole('row', { name: /kim@staff.local/ })).getByRole('button', { name: 'Change role' }))
    const roles = screen.getByRole('dialog', { name: 'Role of kim@staff.local' })
    expect(within(roles).getByRole('radio', { name: /^Staff/ })).toBeChecked()
    await user.click(within(roles).getByRole('radio', { name: /^Administrator/ }))
    await user.click(within(roles).getByRole('button', { name: 'Save role' }))
    expect(await screen.findByRole('row', { name: /kim@staff.local/ })).toHaveTextContent('Administrator')
    const put = fetch.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({ roles: ['admin'] })

    await user.click(within(screen.getByRole('row', { name: /kim@staff.local/ })).getByRole('button', { name: 'Reset password' }))
    const reset = screen.getByRole('dialog', { name: 'Reset the password of kim@staff.local' })
    await user.type(within(reset).getByLabelText(/^Temporary password/), 'another temporary one')
    await user.click(within(reset).getByRole('button', { name: 'Set temporary password' }))
    expect(await screen.findByRole('status')).toHaveTextContent('kim@staff.local has a temporary password.')
    const resetCall = fetch.mock.calls.find(([u, init]) => init?.method === 'POST' && (u as string).endsWith('/s2/password'))
    expect(JSON.parse(resetCall?.[1]?.body as string)).toEqual({ password: 'another temporary one' })
    // nobody changes their own role or resets their own password here
    const alice = screen.getByRole('row', { name: /alice@staff.local/ })
    expect(within(alice).queryByRole('button', { name: 'Change role' })).not.toBeInTheDocument()
    expect(within(alice).queryByRole('button', { name: 'Reset password' })).not.toBeInTheDocument()
  })
})

describe('passwords and metrics', () => {
  it('a member changes their own password from the account menu; mismatches and short ones never leave the page', async () => {
    const { fetch } = renderSignedIn(<App />, { roles: ['staff'], route: (method, url) => (method === 'PUT' && url.pathname === '/v1/staff/password' ? { status: 204 } : { status: 200, body: { waiting: 0, oldest_submitted_at: null } }) })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /alice@staff.local/ }))
    await user.click(await screen.findByRole('menuitem', { name: 'Change password' }))
    const dialog = await screen.findByRole('dialog', { name: 'Change your password' })
    const save = within(dialog).getByRole('button', { name: 'Set password' })
    await user.type(within(dialog).getByLabelText('Current password'), 'old one here')
    await user.type(within(dialog).getByLabelText('New password'), 'short')
    expect(save).toBeDisabled()
    await user.clear(within(dialog).getByLabelText('New password'))
    await user.type(within(dialog).getByLabelText('New password'), 'a brand new long password')
    await user.type(within(dialog).getByLabelText('New password again'), 'a brand new long passwor')
    expect(within(dialog).getByText('The two do not match.')).toBeInTheDocument()
    expect(save).toBeDisabled()
    await user.type(within(dialog).getByLabelText('New password again'), 'd')
    await user.click(save)
    expect(await within(dialog).findByRole('status')).toHaveTextContent('Password changed')
    const put = fetch.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({ current_password: 'old one here', new_password: 'a brand new long password' })
  })

  it('app metrics show each platform’s figures, a chart of the chosen measure and the per-day table', async () => {
    const fig = (n: number) => ({ signups: n, sessions: n * 2, active_players: n * 3, plays: n * 4, stakes_kes: n * 400, top_ups: n, top_ups_kes: n * 1000, withdrawals: 0, withdrawals_kes: 0 })
    const summary = {
      from: '2026-10-27', to: '2026-10-28', platforms: ['web', 'android', 'ios'],
      days: [
        { day: '2026-10-27', platform: 'web', ...fig(1) }, { day: '2026-10-27', platform: 'android', ...fig(2) }, { day: '2026-10-27', platform: 'ios', ...fig(0) },
        { day: '2026-10-28', platform: 'web', ...fig(0) }, { day: '2026-10-28', platform: 'android', ...fig(3) }, { day: '2026-10-28', platform: 'ios', ...fig(1) },
      ],
      totals: [{ platform: 'web', ...fig(1) }, { platform: 'android', ...fig(5) }, { platform: 'ios', ...fig(1) }],
      all: fig(7),
    }
    const { fetch } = renderSignedIn(<App />, { roles: ['admin'], path: '/metrics', route: (_m, url) => (url.pathname === '/v1/staff/metrics' ? { status: 200, body: summary } : { status: 200, body: { waiting: 0, oldest_submitted_at: null } }) })
    const android = await screen.findByRole('group', { name: 'Android' })
    expect(android).toHaveTextContent('15') // players seen: 5 × 3
    expect(android).toHaveTextContent('71% of players seen') // 15 of 21
    expect(screen.getByRole('img', { name: /Players seen per day, by platform/ })).toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Top-ups (KES)' }))
    expect(screen.getByRole('img', { name: /Top-ups \(KES\) per day, by platform/ })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: 'Top-ups (KES) per day' })
    expect(within(table).getByRole('row', { name: /28 Oct/ })).toHaveTextContent('3,000')
    await user.click(screen.getByRole('button', { name: 'Last 7 days' }))
    expect(fetch.mock.calls.some(([u]) => (u as string).includes('/v1/staff/metrics?from='))).toBe(true)
  })
})

describe('players', () => {
  const player = (id: string, status: Player['status'], msisdn: string): Player => ({
    user_id: id, msisdn, kyc_status: 'verified', status, phone_verified: true, balance_kes: 850, plays: 3,
    created_at: '2026-10-27T18:04:26Z', last_sign_in_at: '2026-10-28T09:00:00Z', closure_requested_at: null,
    full_name: id === 'u1' ? 'Amina Otieno' : '', national_id: id === 'u1' ? '12345678' : '',
  })
  it('lists players, searches by phone on the server, filters by status, and suspends with a reason', async () => {
    const asked: string[] = []
    let amina = player('u1', 'active', '254796992457')
    const { fetch } = renderSignedIn(<PlayersPage />, {
      roles: ['staff'],
      route: (method, url, body) => {
        if (method === 'GET' && url.pathname === '/v1/staff/players') {
          asked.push(url.search)
          const all = [player('u2', 'closed', '254712000002'), amina]
          const q = url.searchParams.get('q'), status = url.searchParams.get('status')
          return { status: 200, body: { players: all.filter((p) => (!q || p.msisdn.includes(q)) && (!status || p.status === status)), next_before: null } }
        }
        if (method === 'POST' && url.pathname === '/v1/staff/players/u1/suspend') {
          amina = { ...amina, status: 'suspended' }
          return { status: 200, body: { user_id: 'u1', status: 'suspended', reason: (body as { reason: string }).reason } }
        }
        return { status: 404 }
      },
    })
    const row = await screen.findByRole('row', { name: /0796 992 457/ })
    expect(within(row).getByText('ID verified')).toBeInTheDocument()
    expect(within(row).getByText('Amina Otieno')).toBeInTheDocument()
    expect(within(row).getByText('12345678')).toBeInTheDocument()
    expect(within(screen.getByRole('row', { name: /0712 000 002/ })).getByText('No name yet')).toBeInTheDocument()
    expect(within(screen.getByRole('row', { name: /0712 000 002/ })).getByText('closed')).toBeInTheDocument()
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Phone number or user id'), '992457')
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() => { expect(asked.at(-1)).toContain('q=992457') })
    expect(screen.queryByRole('row', { name: /0712 000 002/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Clear' }))
    await user.click(screen.getByRole('tab', { name: 'Closed' }))
    await waitFor(() => { expect(asked.at(-1)).toContain('status=closed') })
    await user.click(screen.getByRole('tab', { name: 'All' }))
    await user.click(within(await screen.findByRole('row', { name: /0796 992 457/ })).getByRole('button', { name: 'Suspend' }))
    const dialog = screen.getByRole('dialog', { name: 'Suspend 0796 992 457?' })
    expect(within(dialog).getByRole('button', { name: 'Suspend' })).toBeDisabled()
    await user.type(within(dialog).getByLabelText(/^Reason/), 'chargeback dispute')
    await user.click(within(dialog).getByRole('button', { name: 'Suspend' }))
    expect(await screen.findByRole('status')).toHaveTextContent('0796 992 457 is suspended.')
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({ reason: 'chargeback dispute' })
    expect(await within(screen.getByRole('row', { name: /0796 992 457/ })).findByText('suspended')).toBeInTheDocument()
    expect(within(screen.getByRole('row', { name: /0796 992 457/ })).getByRole('button', { name: 'Reinstate' })).toBeInTheDocument()
  })
})

const draw = (id: string): Draw => ({
  id, draw_type: 'daily', period_start: '2026-10-25', period_end: '2026-10-25', basis: 'calendar', status: 'frozen', freeze_at: '2026-10-26T00:05:00+03:00',
  entries: 42, drand_round_at: '2026-10-26T09:00:00+03:00', voided_reason: null, results: [], signoffs: [],
})

describe('paging', () => {
  it('draws load a page at a time behind the server’s cursor, and a reload reads every page again in order', async () => {
    const pages: Record<string, { draws: Draw[]; next_before: string | null }> = {
      first: { draws: [draw('d1'), draw('d2')], next_before: 'd2' },
      d2: { draws: [draw('d3')], next_before: null },
    }
    const asked: string[] = []
    const { fetch } = renderSignedIn(<DrawsPage />, {
      roles: ['staff'],
      route: (method, url) => {
        if (url.pathname === '/v1/staff/draws/overdue') return { status: 200, body: { overdue: [] } }
        if (url.pathname === '/v1/staff/draws') {
          asked.push(url.searchParams.get('before') ?? 'first')
          return { status: 200, body: pages[url.searchParams.get('before') ?? 'first'] }
        }
        if (method === 'POST') return { status: 200, body: {} }
        return { status: 404 }
      },
    })
    const table = await screen.findByRole('table', { name: 'Draws, newest first' })
    expect(within(table).getAllByRole('row')).toHaveLength(3) // header + 2
    expect(asked).toEqual(['first'])
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Load more' }))
    await waitFor(() => { expect(within(table).getAllByRole('row')).toHaveLength(4) })
    expect(asked).toEqual(['first', 'd2'])
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument()
    // signing reloads the list: both pages, in order, and the operator keeps every row they had
    const [sign] = within(table).getAllByRole('button', { name: 'Sign' })
    if (!sign) throw new Error('no Sign button')
    await user.click(sign)
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Sign' }))
    await waitFor(() => { expect(asked).toEqual(['first', 'd2', 'first', 'd2']) })
    await waitFor(() => { expect(within(table).getAllByRole('row')).toHaveLength(4) })
    expect(fetch.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(1)
  })

  it('payouts page forward with `after`, and a change of state starts again from the first page', async () => {
    const asked: string[] = []
    renderSignedIn(<PayoutsPage />, {
      roles: ['staff'],
      route: (_m, url) => {
        asked.push(`${url.searchParams.get('state') ?? ''}:${url.searchParams.get('after') ?? ''}`)
        const state = url.searchParams.get('state') ?? 'held'
        const after = url.searchParams.get('after')
        const id = `${state}-${after ? 2 : 1}`
        const payout = { id, user_id: 'u1', kind: 'withdrawal', status: 'processing', state, gross_kes: 120000, tax_kes: 0, net_kes: 120000, debt_repaid_kes: 0, created_at: '2026-10-26T09:00:00Z' }
        return { status: 200, body: { payouts: [payout], next_after: after ? null : id } }
      },
    })
    expect(await screen.findByText('held-1')).toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByText('held-2')).toBeInTheDocument()
    expect(asked).toEqual(['held:', 'held:held-1'])
    await user.click(screen.getByRole('tab', { name: 'Failed' }))
    expect(await screen.findByText('failed-1')).toBeInTheDocument()
    expect(screen.queryByText('held-1')).not.toBeInTheDocument()
    expect(screen.queryByText('held-2')).not.toBeInTheDocument()
    expect(asked).toEqual(['held:', 'held:held-1', 'failed:'])
    expect(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument()
  })
})
