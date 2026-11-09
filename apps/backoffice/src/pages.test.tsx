import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { DrawsPage } from './pages/Draws'
import { PayoutsPage } from './pages/Payouts'
import { PlayersPage } from './pages/Players'
import { problem, renderSignedIn } from './test/render'
import type { Draw, FinanceSummary, Player, PlayerTimeline, ReconciliationRun, StaffMember } from './types'

const zero = { deposits_kes: 0, stakes_kes: 0, prizes_awarded_kes: 0, prizes_paid_kes: 0, withdrawals_kes: 0, reversals_kes: 0, excise_kes: 0, tax_withheld_kes: 0, tax_remitted_kes: 0, ggr_kes: 0 }
const none = { count: 0, kes: 0 }
const dayA = { start: '2026-10-25', end: '2026-10-25', ...zero, deposits_kes: 10000, stakes_kes: 3000, ggr_kes: 2100, plays: 30, players: 7, signups: 8 }
const dayB = { start: '2026-10-26', end: '2026-10-26', ...zero, deposits_kes: 5000, stakes_kes: 1200, prizes_paid_kes: 900, withdrawals_kes: 2500, ggr_kes: 840, plays: 12, players: 5, signups: 4 }
const summary: FinanceSummary = {
  from: '2026-10-25',
  to: '2026-10-26',
  group: 'day',
  totals: { ...zero, deposits_kes: 15000, stakes_kes: 4200, prizes_awarded_kes: 1260, prizes_paid_kes: 900, withdrawals_kes: 2500, excise_kes: 750, tax_withheld_kes: 180, ggr_kes: 2940 },
  periods: [dayA, dayB],
  balances: { owed_to_players_kes: 9800, player_debt_kes: 60, excise_owed_kes: 750, tax_owed_kes: 180, prize_pools_kes: 360, suspense_kes: 0, mpesa_clearing_kes: 15000, b2c_float_kes: -3400 },
  activity: { signups: 12, plays: 42, players: 9 },
  by_kind: [{ kind: 'deposit', journals: 30, debit_kes: 15750, credit_kes: 15750 }, { kind: 'stake', journals: 42, debit_kes: 4200, credit_kes: 4200 }],
  trial_balance: {
    as_at: '2026-10-26',
    accounts: [
      { code: 'MPESA_CLEARING', name: 'M-Pesa paybill clearing', type: 'asset', debits_kes: 15000, credits_kes: 0, balance_kes: 15000 },
      { code: 'WALLET_LIABILITY', name: 'Player wallet liability', type: 'liability', debits_kes: 6700, credits_kes: 16500, balance_kes: 9800 },
    ],
    debits_kes: 21700,
    credits_kes: 21700,
    balanced: true,
  },
  top_ups: [
    { start: '2026-10-25', end: '2026-10-25', started: { count: 25, kes: 12000 }, credited: { count: 20, kes: 10000 }, failed: { count: 2, kes: 1000 }, cancelled: { count: 2, kes: 800 }, expired: none, unconfirmed: { count: 1, kes: 200 }, reversed: none, pending_now: none, median_seconds_to_credit: 42 },
    { start: '2026-10-26', end: '2026-10-26', started: { count: 10, kes: 5000 }, credited: { count: 10, kes: 5000 }, failed: none, cancelled: none, expired: none, unconfirmed: none, reversed: none, pending_now: none, median_seconds_to_credit: null },
  ],
  payouts: [
    { start: '2026-10-25', end: '2026-10-25', requested: none, paid: none, failed: none, held: none, manual: none, tax_withheld_kes: 0 },
    { start: '2026-10-26', end: '2026-10-26', requested: { count: 3, kes: 3400 }, paid: { count: 2, kes: 2320 }, failed: none, held: { count: 1, kes: 900 }, manual: none, tax_withheld_kes: 180 },
  ],
  prize_pools: [{ type: 'daily', balance_kes: 360, funded_kes: 1260, paid_kes: 900 }],
  tax: { withheld_kes: 180, remitted_kes: 0, awaiting_remittance: { batches: 1, kes: 120 }, unbatched: { events: 2, kes: 60 }, oldest_unremitted_age_seconds: 3 * 86400 },
}

const runs: ReconciliationRun[] = [
  { id: 'r2', area: 'payments', ran_at: '2026-10-26T06:00:00Z', ok: false, mismatches: { credited_without_journal: 2 }, attention: { c2b_overdue: 1 }, totals: { payments: 120 } },
  { id: 'r1', area: 'payments', ran_at: '2026-10-25T06:00:00Z', ok: true, mismatches: {}, attention: {}, totals: { payments: 100 } },
]

/** The finance requests made so far, as URLs (the shell polls other things). */
const financeCalls = (fetch: ReturnType<typeof renderSignedIn>['fetch']) =>
  fetch.mock.calls.map(([u]) => new URL(u as string)).filter((u) => u.pathname === '/v1/staff/finance')

describe('finance', () => {
  it('shows the range’s figures, what is owed, and every period in a table', async () => {
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
    expect(screen.getByRole('img', { name: /Gross gaming revenue per day/ })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: /^Per day/ })
    expect(within(table).getAllByRole('row')).toHaveLength(4) // header, two days, total
    expect(within(table).getByRole('row', { name: /26 Oct/ })).toHaveTextContent('2,500')
    expect(within(table).getByRole('row', { name: /Total/ })).toHaveTextContent('42')
    // the first request asks for the last 30 days by day; another range asks the server again
    const first = financeCalls(fetch)[0]
    expect(first?.searchParams.get('group')).toBe('day')
    expect(first?.searchParams.get('from')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Last 7 days' }))
    await waitFor(() => { expect(financeCalls(fetch)).toHaveLength(2) })
    const second = financeCalls(fetch)[1]
    expect(second?.searchParams.get('to')).toBe(first?.searchParams.get('to'))
    expect(second?.searchParams.get('from')).not.toBe(first?.searchParams.get('from'))
  })

  it('groups by week or month on request and names the periods by their ends', async () => {
    const weekly: FinanceSummary = { ...summary, group: 'week', from: '2026-10-15', to: '2026-10-26', periods: [
      { ...dayA, start: '2026-10-15', end: '2026-10-18' },
      { ...dayB, start: '2026-10-19', end: '2026-10-25' },
      { ...dayB, start: '2026-10-26', end: '2026-10-26' },
    ] }
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/finance',
      route: (_m, url) => (url.pathname === '/v1/staff/finance' ? { status: 200, body: url.searchParams.get('group') === 'week' ? weekly : summary } : { status: 404 }),
    })
    await screen.findByRole('heading', { name: 'Finance' })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Week' }))
    const table = await screen.findByRole('table', { name: /^Per week/ })
    expect(within(table).getByRole('row', { name: /19 Oct – 25 Oct/ })).toBeInTheDocument()
    expect(financeCalls(fetch).at(-1)?.searchParams.get('group')).toBe('week')
    expect(screen.getByRole('button', { name: 'Week' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('a custom range is asked for only once it runs forward and fits in 366 days', async () => {
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/finance',
      route: (_m, url) => (url.pathname === '/v1/staff/finance' ? { status: 200, body: summary } : { status: 404 }),
    })
    await screen.findByRole('heading', { name: 'Finance' })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Custom' }))
    const before = financeCalls(fetch).length
    const from = screen.getByLabelText('From')
    const to = screen.getByLabelText('To')
    fireEvent.change(from, { target: { value: '2026-10-20' } })
    fireEvent.change(to, { target: { value: '2026-10-10' } })
    expect(screen.getByText('The range must run forward and cover at most 366 days.')).toHaveAttribute('role', 'alert')
    expect(financeCalls(fetch)).toHaveLength(before)
    fireEvent.change(to, { target: { value: '2028-10-10' } })
    expect(screen.getByText('The range must run forward and cover at most 366 days.')).toBeInTheDocument()
    expect(financeCalls(fetch)).toHaveLength(before)
    fireEvent.change(to, { target: { value: '2026-10-25' } })
    await waitFor(() => { expect(financeCalls(fetch)).toHaveLength(before + 1) })
    const url = financeCalls(fetch).at(-1)
    expect(url?.searchParams.get('from')).toBe('2026-10-20')
    expect(url?.searchParams.get('to')).toBe('2026-10-25')
    expect(screen.queryByText('The range must run forward and cover at most 366 days.')).not.toBeInTheDocument()
  })

  it('exports the figures and the ledger as files, and reports a refused ledger export in words', async () => {
    const created: string[] = []
    const objectURL = vi.fn(() => 'blob:finance')
    Object.defineProperty(URL, 'createObjectURL', { value: objectURL, configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: vi.fn(), configurable: true })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { created.push(this.download) })
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/finance',
      route: (_m, url) => {
        if (url.pathname === '/v1/staff/finance') return { status: 200, body: summary }
        if (url.pathname === '/v1/staff/finance/export') return { status: 200, body: 'period_start,period_end' }
        if (url.pathname === '/v1/staff/ledger/export') return problem(400, 'too-many-rows', 'Too many ledger entries for one file; narrow the range')
        return { status: 404 }
      },
    })
    await screen.findByRole('heading', { name: 'Finance' })
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Export figures (CSV)' }))
    expect(await screen.findByText(/^Saved finance-\d{4}-\d{2}-\d{2}-\d{4}-\d{2}-\d{2}\.csv/)).toHaveAttribute('role', 'status')
    const call = fetch.mock.calls.find(([u]) => (u as string).includes('/v1/staff/finance/export'))
    const url = new URL(call?.[0] as string)
    expect(url.searchParams.get('group')).toBe('day')
    expect(url.searchParams.get('from')).toBeTruthy()
    expect(created).toHaveLength(1)
    expect(created[0]).toMatch(/^finance-.*\.csv$/)
    expect(objectURL).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: 'Export ledger (CSV)' }))
    expect(await screen.findByText(/Too many ledger entries for one file; narrow the range/)).toBeInTheDocument()
    expect(created).toHaveLength(1)
    click.mockRestore()
  })

  it('warns when the trial balance does not balance, and shows the journal kinds', async () => {
    const broken: FinanceSummary = { ...summary, trial_balance: { ...summary.trial_balance, credits_kes: 21690, balanced: false } }
    renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/finance',
      route: (_m, url) => (url.pathname === '/v1/staff/finance' ? { status: 200, body: broken } : { status: 404 }),
    })
    await screen.findByRole('heading', { name: 'Finance' })
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: /Trial balance/ }))
    expect(await screen.findByText('Debits and credits differ')).toBeInTheDocument()
    expect(screen.getByText(/The ledger does not balance/)).toHaveAttribute('role', 'alert')
    const table = screen.getByRole('table', { name: /Trial balance/ })
    expect(within(table).getByRole('row', { name: /WALLET_LIABILITY/ })).toHaveTextContent('9,800')
    expect(within(table).getByRole('row', { name: /Total/ })).toHaveTextContent('off by 10')
    expect(within(screen.getByRole('table', { name: /by kind/ })).getByRole('row', { name: /stake/ })).toHaveTextContent('42')
  })

  it('lists the stored reconciliation runs of an area, newest first, with their findings counted', async () => {
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/finance',
      route: (_m, url) => {
        if (url.pathname === '/v1/staff/finance') return { status: 200, body: summary }
        if (url.pathname === '/v1/staff/reconciliations') return { status: 200, body: { runs: url.searchParams.get('area') === 'payments' ? runs : [], next_before: null } }
        return { status: 404 }
      },
    })
    await screen.findByRole('heading', { name: 'Finance' })
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Reconciliations' }))
    const table = await screen.findByRole('table', { name: /Payments reconciliation runs/ })
    const rows = within(table).getAllByRole('row')
    expect(rows).toHaveLength(3)
    expect(rows[1]).toHaveTextContent('Mismatches')
    expect(rows[1]).toHaveTextContent('credited_without_journal × 2')
    expect(rows[1]).toHaveTextContent('c2b_overdue × 1')
    expect(rows[2]).toHaveTextContent('Everything matched')
    const list = fetch.mock.calls.map(([u]) => new URL(u as string)).find((u) => u.pathname === '/v1/staff/reconciliations')
    expect(list?.searchParams.get('area')).toBe('payments')
    expect(screen.getByRole('button', { name: 'Run now' })).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Draws' }))
    expect(await screen.findByText('No draws reconciliation has run yet.')).toBeInTheDocument()
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
    const roles = screen.getByRole('dialog', { name: 'Role of Kim' })
    expect(within(roles).getByRole('radio', { name: /^Staff/ })).toBeChecked()
    await user.click(within(roles).getByRole('radio', { name: /^Administrator/ }))
    await user.click(within(roles).getByRole('button', { name: 'Save role' }))
    expect(await screen.findByRole('row', { name: /kim@staff.local/ })).toHaveTextContent('Administrator')
    const put = fetch.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({ roles: ['admin'] })

    await user.click(within(screen.getByRole('row', { name: /kim@staff.local/ })).getByRole('button', { name: 'Reset password' }))
    const reset = screen.getByRole('dialog', { name: 'Reset the password of Kim' })
    await user.type(within(reset).getByLabelText(/^Temporary password/), 'another temporary one')
    await user.click(within(reset).getByRole('button', { name: 'Set temporary password' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Kim has a temporary password.')
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
    await user.click(await screen.findByRole('button', { name: /Alice/ }))
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

  it('a member changes their name from the account menu, and it shows in place of their email address', async () => {
    const { fetch, session } = renderSignedIn(<App />, { roles: ['staff'], route: (method, url) => (method === 'PUT' && url.pathname === '/v1/staff/me/name' ? { status: 200, body: { id: 's1', name: 'Alice Njeri', email: 'alice@staff.local' } } : { status: 200, body: { waiting: 0, oldest_submitted_at: null } }) })
    const user = userEvent.setup()
    const account = await screen.findByRole('button', { name: /Alice/ })
    expect(account).not.toHaveTextContent('alice@staff.local')
    await user.click(account)
    await user.click(await screen.findByRole('menuitem', { name: 'Change name' }))
    const dialog = await screen.findByRole('dialog', { name: 'Change your name' })
    const name = within(dialog).getByLabelText('Your name')
    await user.clear(name)
    expect(within(dialog).getByRole('button', { name: 'Save name' })).toBeDisabled()
    await user.type(name, '  Alice Njeri ')
    await user.click(within(dialog).getByRole('button', { name: 'Save name' }))
    await waitFor(() => { expect(session.updateMe).toHaveBeenCalledWith(expect.objectContaining({ name: 'Alice Njeri', email: 'alice@staff.local' })) })
    const put = fetch.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({ name: 'Alice Njeri' })
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
    user_id: id, msisdn, email: id === 'u1' ? 'amina@example.com' : null, kyc_status: 'verified', status, phone_verified: true, balance_kes: 850, plays: 3,
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
    await user.type(screen.getByLabelText('Phone number, email or user id'), '992457')
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

describe('players: temporary passwords', () => {
  it('staff give a player a temporary password with a reason, after being told to check who is asking', async () => {
    const player: Player = {
      user_id: 'u1', msisdn: '254796992457', email: 'jane@example.com', kyc_status: 'verified', status: 'active', phone_verified: true, balance_kes: 100, plays: 3,
      created_at: '2026-10-26T08:00:00Z', last_sign_in_at: '2026-10-28T08:00:00Z', closure_requested_at: null, full_name: 'Amina Otieno', national_id: '12345678',
    }
    const { fetch } = renderSignedIn(<App />, { roles: ['staff'], path: '/players', route: (method, url) => {
      if (method === 'POST' && url.pathname === '/v1/staff/players/u1/password') return { status: 204 }
      if (url.pathname === '/v1/staff/players') return { status: 200, body: { players: [player], next_before: null } }
      return { status: 200, body: { waiting: 0, oldest_submitted_at: null } }
    } })
    const user = userEvent.setup()
    await user.click(within(await screen.findByRole('row', { name: /0796 992 457/ })).getByRole('button', { name: 'Temporary password' }))
    const dialog = screen.getByRole('dialog', { name: 'Temporary password for 0796 992 457' })
    expect(within(dialog).getByText(/check the caller against the name and ID number/)).toBeInTheDocument()
    const confirm = within(dialog).getByRole('button', { name: 'Set temporary password' })
    await user.type(within(dialog).getByLabelText(/^Reason/), 'caller verified by ID over the phone')
    await user.type(within(dialog).getByLabelText(/^Temporary password/), 'short')
    expect(confirm).toBeDisabled()
    await user.type(within(dialog).getByLabelText(/^Temporary password/), ' but long enough now')
    await user.click(confirm)
    expect(await screen.findByRole('status')).toHaveTextContent('0796 992 457 has a temporary password.')
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({ password: 'short but long enough now', reason: 'caller verified by ID over the phone' })
  })
})

describe('players: the timeline (owner, 2026-10-31)', () => {
  const amina: Player = {
    user_id: 'u1', msisdn: '254796992457', email: 'amina@example.com', kyc_status: 'verified', status: 'active', phone_verified: true, balance_kes: 850, plays: 3,
    created_at: '2026-10-27T18:04:26Z', last_sign_in_at: '2026-10-28T09:00:00Z', closure_requested_at: null, full_name: 'Amina Otieno', national_id: '12345678',
  }
  const timeline: PlayerTimeline = {
    player: amina,
    wallet: { balance_kes: 850, debt_kes: 0, transactions: [
      { id: 'w2', kind: 'stake', amount_kes: -100, journal_id: 'j2', reference: 'ticket:t-1', created_at: '2026-10-28T09:05:00Z' },
      { id: 'w1', kind: 'deposit', amount_kes: 950, journal_id: 'j1', reference: 'payment:p-1', created_at: '2026-10-28T09:00:00Z' },
    ] },
    top_ups: [{ id: 'p-1', channel: 'stk', status: 'credited', amount_requested_kes: 1000, amount_confirmed_kes: 1000, receipt: 'SGH1234567', failure_reason: null, result_code: 0, created_at: '2026-10-28T08:59:00Z' }],
    plays: { count: 3, last: [{ ticket_id: 't-1', play_date: '2026-10-28', seq: 1, price_kes: 100, status: 'paid', slot: 4, lucky: false, created_at: '2026-10-28T09:05:00Z' }] },
    payouts: [],
    identity: { kyc: [{ id: 'k1', status: 'verified', provider: 'manual', reviewed_by: 'staff:m1', reason: 'ID matches', created_at: '2026-10-27T19:00:00Z' }],
      sessions: [{ id: 's1', platform: 'android', created_at: '2026-10-28T09:00:00Z', revoked_at: null, revoked_reason: null, device_verdict: 'passed' }],
      status_history: [{ id: 2, from: 'active', to: 'suspended', actor_type: 'staff', actor: 'staff:ops-lead', reason: 'chargeback inquiry', at: '2026-10-29T09:00:00Z' }, { id: 1, from: null, to: 'active', actor_type: 'player', actor: 'u1', reason: 'signed up', at: '2026-10-27T09:00:00Z' }],
      locations: [{ id: 3, action: 'play', country: 'UG', refused: true, at: '2026-10-29T10:00:00Z' }, { id: 1, action: 'signup', country: 'KE', refused: false, at: '2026-10-27T09:00:00Z' }],
      closure_requested_at: null },
    limits: { deposit: [{ period: 'daily', amount_kes: 500, effective_from: '2026-10-27T19:00:00Z', pending: false }], restriction: null },
    events: [{ id: '7', at: '2026-10-28T09:05:00Z', actor_type: 'player', actor_id: 'u1', action: 'game.play', entity_type: 'ticket', entity_id: 't-1', details: { price_kes: 100 } }],
  }
  const server = (asked: URL[]) => (method: string, url: URL) => {
    asked.push(url)
    if (method === 'GET' && url.pathname === '/v1/staff/players') return { status: 200, body: { players: [amina], next_before: null } }
    if (method === 'GET' && url.pathname === '/v1/staff/players/u1') return { status: 200, body: timeline }
    if (method === 'GET' && url.pathname === '/v1/staff/players/u1/export') return { status: 200, body: 'section,id' }
    return { status: 404 }
  }

  it('opens from a row with every section, and downloads the player pack', async () => {
    const asked: URL[] = []
    const created: string[] = []
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:player'), configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: vi.fn(), configurable: true })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { created.push(this.download) })
    renderSignedIn(<PlayersPage />, { roles: ['staff'], route: server(asked) })
    const user = userEvent.setup()
    await user.click(within(await screen.findByRole('row', { name: /0796 992 457/ })).getByRole('button', { name: 'Open 0796 992 457' }))
    const sheet = await screen.findByRole('dialog', { name: 'Amina Otieno' })
    expect(within(sheet).getByText('KES 850')).toBeInTheDocument()
    expect(within(sheet).getByText('3 plays')).toBeInTheDocument()
    expect(within(sheet).getByText('Top-up')).toBeInTheDocument()
    expect(within(sheet).getByText('Play')).toBeInTheDocument()
    expect(within(sheet).getByText('−KES 100')).toBeInTheDocument()
    await user.click(within(sheet).getByRole('tab', { name: 'Top-ups' }))
    expect(within(sheet).getByText('SGH1234567')).toBeInTheDocument()
    expect(within(sheet).getByRole('link', { name: 'Open top-up' })).toHaveAttribute('href', '/topups?topup=p-1')
    await user.click(within(sheet).getByRole('tab', { name: 'Identity & sessions' }))
    expect(within(sheet).getByText('device passed')).toBeInTheDocument()
    expect(within(sheet).getByText(/ID matches/)).toBeInTheDocument()
    expect(within(sheet).getByText('active → suspended')).toBeInTheDocument()
    expect(within(sheet).getByText(/chargeback inquiry/)).toBeInTheDocument()
    expect(within(sheet).getByText('UG')).toBeInTheDocument()
    expect(within(sheet).getByText('refused')).toBeInTheDocument()
    await user.click(within(sheet).getByRole('tab', { name: 'Limits' }))
    expect(within(sheet).getByText('KES 500')).toBeInTheDocument()
    await user.click(within(sheet).getByRole('tab', { name: 'Activity' }))
    expect(within(sheet).getByText('game.play')).toBeInTheDocument()
    expect(within(sheet).getByRole('link', { name: 'Everything, in the activity log' })).toHaveAttribute('href', '/activity?entity_id=u1')
    expect(within(sheet).getByRole('button', { name: 'Suspend' })).toBeInTheDocument()
    expect(within(sheet).queryByRole('button', { name: 'Refund to source' })).not.toBeInTheDocument() // admins only (ADR 0031)
    await user.click(within(sheet).getByRole('button', { name: 'Download player pack (CSV)' }))
    await waitFor(() => { expect(created).toEqual(['player-u1.csv']) })
    expect(asked.some((u) => u.pathname === '/v1/staff/players/u1/export')).toBe(true)
    expect(await within(sheet).findByRole('status')).toHaveTextContent(/Saved player-u1.csv/)
    click.mockRestore()
  })

  it('an admin asks for a refund to source with a reason (ADR 0031)', async () => {
    const asked: URL[] = []
    const base = server(asked)
    const { fetch } = renderSignedIn(<PlayersPage />, { roles: ['admin'], path: '/players?player=u1', route: (method, url) =>
      method === 'POST' && url.pathname === '/v1/staff/players/u1/refund' ? { status: 201, body: { id: 'po-1', state: 'held' } } : base(method, url) })
    const user = userEvent.setup()
    const sheet = await screen.findByRole('dialog', { name: 'Amina Otieno' })
    await user.click(within(sheet).getByRole('button', { name: 'Refund to source' }))
    const dialog = screen.getByRole('dialog', { name: 'Refund KES 850 to source?' })
    expect(within(dialog).getByText(/until someone else approves it/)).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText(/^Reason/), 'identity refused by the registry')
    await user.click(within(dialog).getByRole('button', { name: 'Ask for the refund' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Refund asked for.')
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({ reason: 'identity refused by the registry' })
  })

  it('staff record a self-exclusion the player asked for, with its term and how they asked (ADR 0032)', async () => {
    const asked: URL[] = []
    const base = server(asked)
    const { fetch } = renderSignedIn(<PlayersPage />, { roles: ['staff'], path: '/players?player=u1', route: (method, url) =>
      method === 'POST' && url.pathname === '/v1/staff/players/u1/self-exclusion' ? { status: 200, body: { user_id: 'u1', term: '1y' } } : base(method, url) })
    const user = userEvent.setup()
    const sheet = await screen.findByRole('dialog', { name: 'Amina Otieno' })
    await user.click(within(sheet).getByRole('button', { name: 'Record self-exclusion' }))
    const dialog = screen.getByRole('dialog', { name: 'Self-exclude 0796 992 457?' })
    expect(within(dialog).getByText(/nobody can lift or shorten it/)).toBeInTheDocument()
    await user.click(within(dialog).getByLabelText('1 year'))
    await user.type(within(dialog).getByLabelText(/^How they asked/), 'called from the registered number, ID checked')
    await user.click(within(dialog).getByRole('button', { name: 'Self-exclude' }))
    expect(await screen.findByRole('status')).toHaveTextContent('0796 992 457 is self-excluded.')
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({ term: '1y', reason: 'called from the registered number, ID checked' })
  })

  it('opens from a ?player= link, and closing it keeps the list', async () => {
    const asked: URL[] = []
    renderSignedIn(<PlayersPage />, { roles: ['staff'], route: server(asked), path: '/players?player=u1' })
    const sheet = await screen.findByRole('dialog', { name: 'Amina Otieno' })
    expect(within(sheet).getByText('Balance')).toBeInTheDocument()
    expect(asked.some((u) => u.pathname === '/v1/staff/players/u1')).toBe(true)
    await userEvent.click(within(sheet).getByRole('button', { name: 'Close' }))
    await waitFor(() => { expect(screen.queryByRole('dialog')).not.toBeInTheDocument() })
    expect(screen.getByRole('row', { name: /0796 992 457/ })).toBeInTheDocument()
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

  it('two people approve a held payout and settle a manual one (ADR 0032)', async () => {
    const base = { user_id: 'u1', kind: 'withdrawal', status: 'processing', gross_kes: 120000, tax_kes: 0, net_kes: 120000, debt_repaid_kes: 0, created_at: '2026-10-26T09:00:00Z' }
    const { fetch } = renderSignedIn(<PayoutsPage />, {
      roles: ['staff'],
      route: (method, url) => {
        if (method === 'POST') return { status: 200, body: { ...base, id: 'po-new', state: 'held', approvals: ['staff:s1'] } }
        if (url.searchParams.get('state') === 'manual') return { status: 200, body: { payouts: [{ ...base, id: 'po-m', state: 'manual', proposal: { paid: true, receipt: 'SGH0000001', by: 'staff:s2', at: '2026-10-26T10:00:00Z' } }], next_after: null } }
        return { status: 200, body: { payouts: [
          { ...base, id: 'po-new', state: 'held' },
          { ...base, id: 'po-mine', state: 'held', approvals: ['staff:s1'] },
        ], next_after: null } }
      },
    })
    const user = userEvent.setup()
    const fresh = await screen.findByRole('row', { name: /po-new/ })
    await user.click(within(fresh).getByRole('button', { name: 'Approve' }))
    const dialog = screen.getByRole('dialog', { name: 'Approve KES 120,000?' })
    expect(within(dialog).getByText(/first of two approvals/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Approve' }))
    expect(await screen.findByRole('status')).toHaveTextContent('A second person approves it before it is sent.')
    expect(fetch.mock.calls.some(([u, init]) => init?.method === 'POST' && (typeof u === 'string' ? u : u instanceof URL ? u.href : u.url).endsWith('/v1/staff/payouts/po-new/approve'))).toBe(true)
    const mine = screen.getByRole('row', { name: /po-mine/ })
    expect(within(mine).getByRole('button', { name: 'You approved' })).toBeDisabled()
    expect(within(mine).getByText(/one more person approves it/)).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Needs manual settlement' }))
    expect(await screen.findByText(/a second person confirms it/)).toBeInTheDocument()
    expect(screen.getByText('staff:s2')).toBeInTheDocument()
  })

  it('a held refund to source says why, who asked and where it goes (ADR 0031)', async () => {
    renderSignedIn(<PayoutsPage />, {
      roles: ['staff'],
      route: () => ({ status: 200, body: { payouts: [{ id: 'po-1', user_id: 'u1', kind: 'withdrawal', status: 'processing', state: 'held', gross_kes: 950, tax_kes: 0,
        net_kes: 950, debt_repaid_kes: 0, refund: { reason: 'identity refused', requested_by: 'staff:alice', msisdn: '254708374149' }, created_at: '2026-10-26T09:00:00Z' }], next_after: null } }),
    })
    expect(await screen.findByText('Refund to source')).toBeInTheDocument()
    expect(screen.getByText(/identity refused · asked by/)).toBeInTheDocument()
    expect(screen.getByText('staff:alice')).toBeInTheDocument()
    expect(screen.getByText(/to 0708 374 149/)).toBeInTheDocument()
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
