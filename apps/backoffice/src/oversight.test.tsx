import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { ActivityPage } from './pages/Activity'
import { AlertsPage } from './pages/Alerts'
import { PlaysPage } from './pages/Plays'
import { SystemPage } from './pages/System'
import { TopUpsPage } from './pages/TopUps'
import { renderSignedIn } from './test/render'
import type { Alert, AlertDetail, AreaSwitch, AuditEvent, OperatingState, PlayRow, StaffMember, SystemState, TopUpDetail, TopUpRow } from './types'

const play: PlayRow = {
  ticket_id: 't-1', user_id: 'u-1', msisdn: '254705369005', full_name: 'Willis Ivali', play_date: '2026-10-30', seq: 2,
  price_kes: 1, status: 'paid', slot: 0, lucky: true, created_at: '2026-10-30T09:23:04Z',
}

const topUp: TopUpRow = {
  id: 'p-1', user_id: 'u-1', msisdn: '254705369005', full_name: 'Willis Ivali', channel: 'stk', amount_requested_kes: 1,
  amount_confirmed_kes: 1, status: 'credited', failure_reason: null, result_code: 1037, receipt: 'SANDBOX-p-1', sandbox: true,
  claimed_receipt: null, claim_status: null, created_at: '2026-10-30T09:22:29Z', updated_at: '2026-10-30T09:22:58Z',
}

const system: SystemState = {
  now: '2026-10-30T10:00:00Z',
  settings: {
    daraja: 'sandbox', sandbox_settlement: true, top_up_min_kes: 1, top_up_cooldown_s: 120, buy_goods: false, paybill: true, payouts: false,
    email: true, google_sign_in: false, apple_sign_in: false, attestation: 'record', kyc_registry: false, alerts_delivery: 'log', heartbeat: false,
    workers: 'in-process', started_at: '2026-10-30T09:18:34Z', version: '2e6a991ceffc', play_price_kes: 1, wheel_slots: 12, wheel_lucky_slots: 1,
  },
  jobs: [{ kind: 'payments.stk_verify', state: 'completed', count: 3 }],
  failed_jobs: [{ id: 9, kind: 'payouts.send', state: 'retryable', attempt: 2, last_error: 'daraja: unavailable', at: '2026-10-30T09:50:00Z' }],
  reconciliations: [
    { area: 'payments', ran_at: '2026-10-30T09:18:40Z', ok: true }, { area: 'game', ran_at: null, ok: null },
    { area: 'draws', ran_at: '2026-10-30T09:18:41Z', ok: true }, { area: 'payouts', ran_at: '2026-10-30T09:18:41Z', ok: false },
  ],
  open_alerts: 1, players: 4, plays_today: 3, top_ups_pending: 0, top_ups_unconfirmed: 1, receipt_claims: 0,
}

describe('plays', () => {
  it('lists plays with the player’s name and time, and searches on the server', async () => {
    const asked: URL[] = []
    renderSignedIn(<PlaysPage />, { roles: ['staff'], route: (_m, url) => { asked.push(url); return { status: 200, body: { plays: [play], next_before: null } } } })
    const table = await screen.findByRole('table', { name: /Plays, newest first/ })
    expect(within(table).getByText('Willis Ivali')).toBeInTheDocument()
    expect(within(table).getByText('Slot 0')).toBeInTheDocument()
    expect(within(table).getByText('Lucky')).toBeInTheDocument()
    expect(within(table).getByText(/play 2 of 2026-10-30/)).toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: 'Phone number or player id' }), '0705')
    await userEvent.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() => { expect(asked.at(-1)?.searchParams.get('q')).toBe('0705') })
  })
})

describe('top-ups', () => {
  it('shows each top-up’s outcome in words and marks sandbox credits', async () => {
    renderSignedIn(<TopUpsPage />, { roles: ['staff'], route: () => ({ status: 200, body: { topups: [topUp], next_before: null } }) })
    const table = await screen.findByRole('table', { name: /Top-ups, newest first/ })
    expect(within(table).getByText('Credited')).toBeInTheDocument()
    expect(within(table).getByText('Sandbox')).toBeInTheDocument()
    expect(within(table).getByText(/credited without M-Pesa’s confirmation/)).toBeInTheDocument()
    expect(within(table).getByText('SANDBOX-p-1')).toBeInTheDocument()
  })
})

describe('top-up reconciliation', () => {
  const claimed: TopUpRow = {
    ...topUp, id: 'p-2', status: 'unconfirmed', amount_confirmed_kes: null, result_code: null, receipt: null, sandbox: false,
    failure_reason: 'paid_without_callback', claimed_receipt: 'QJ7XK2M9PL', claim_status: 'unverified',
  }
  const detail: TopUpDetail = {
    ...claimed, merchant_request_id: 'm-1', checkout_request_id: 'ws_CO_1', reversed_at: null, reversal_reason: null, callback: null,
    claim: { receipt: 'QJ7XK2M9PL', status: 'unverified', claimed_at: '2026-10-30T18:40:00Z', note: 'M-Pesa did not confirm the receipt: does not exist', originator: 'AG_1' },
    orphan_callbacks: [], jobs: [{ kind: 'payments.stk_verify', state: 'completed', attempt: 3, scheduled_at: '2026-10-30T18:36:00Z', finalized_at: '2026-10-30T18:49:00Z' }],
    events: [{ id: '7', at: '2026-10-30T18:40:00Z', actor_type: 'player', actor_id: 'u-1', action: 'payment.topup.receipt_claimed', entity_type: 'payment', entity_id: 'p-2', details: { receipt: 'QJ7XK2M9PL' } }],
    player: { status: 'active', kyc_status: 'verified', phone_verified: false, email: null, balance_kes: 0, top_ups: 2, credited_kes: 1, unconfirmed_count: 1 },
  }

  it('marks a player’s receipt in the list, filters on it, and opens the top-up with its claim and actions', async () => {
    const calls: { method: string; url: URL; body: unknown }[] = []
    renderSignedIn(<TopUpsPage />, {
      roles: ['staff'],
      route: (method, url, body) => {
        calls.push({ method, url, body })
        if (url.pathname === '/v1/staff/topups/p-2') return { status: 200, body: detail }
        if (method === 'POST') return { status: 204 }
        return { status: 200, body: { topups: [claimed], next_before: null } }
      },
    })
    const table = await screen.findByRole('table', { name: /Top-ups, newest first/ })
    expect(within(table).getByText('Receipt from player')).toBeInTheDocument()
    expect(within(table).getByText('QJ7XK2M9PL')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('checkbox', { name: /Receipt from player to check/ }))
    await waitFor(() => { expect(calls.at(-1)?.url.searchParams.get('claimed')).toBe('true') })

    // the filtered list is a fresh table
    const filtered = await screen.findByRole('table', { name: /Top-ups, newest first/ })
    await userEvent.click(await within(filtered).findByRole('button', { name: 'Open top-up p-2' }))
    const sheet = await screen.findByRole('dialog', { name: 'Top-up' })
    expect(await within(sheet).findByText('Receipt from player: not confirmed')).toBeInTheDocument()
    expect(within(sheet).getByText(/does not exist/)).toBeInTheDocument()
    expect(within(sheet).getByText(/receipt claimed/)).toBeInTheDocument()
    expect(within(sheet).getByText('Phone not proved')).toBeInTheDocument()
    await userEvent.click(within(sheet).getByRole('button', { name: 'Credit from receipt' }))
    const dialog = await screen.findByRole('dialog', { name: 'Credit from a receipt' })
    expect(within(dialog).getByLabelText(/^M-Pesa receipt/)).toHaveValue('QJ7XK2M9PL')
    expect(within(dialog).getByLabelText(/^Amount paid/)).toHaveDisplayValue('1')
    await userEvent.type(within(dialog).getByLabelText(/^Note/), 'statement 31 Oct')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Credit' }))
    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST')
      expect(post?.url.pathname).toBe('/v1/staff/topups/p-2/resolve')
      expect(post?.body).toEqual({ receipt: 'QJ7XK2M9PL', amount_kes: 1, note: 'statement 31 Oct' })
    })
  })
})

const operating: OperatingState = {
  status: { mode: 'sandbox', configured: 'sandbox', real_money: false, last_change: null },
  live_requests: [], history: [],
}

const running: AreaSwitch[] = [{ area: 'top-ups', paused: false }, { area: 'plays', paused: false }, { area: 'withdrawals', paused: false }, { area: 'payouts', paused: false }]

/** Answers the System page's reads: the system state, the operating mode and the money switches. */
const systemServer = (sys: SystemState, op: OperatingState = operating, switches: AreaSwitch[] = running) => (_m: string, url: URL) =>
  ({ status: 200, body: url.pathname === '/v1/staff/operating' ? op : url.pathname === '/v1/staff/operating/switches' ? { switches } : sys })

describe('system status', () => {
  it('shows the settings, failing jobs and reconciliations', async () => {
    renderSignedIn(<SystemPage />, { roles: ['staff'], route: systemServer(system) })
    expect(await screen.findByText('12 slots, 1 lucky')).toBeInTheDocument()
    expect(await screen.findAllByText('Sandbox')).toHaveLength(2) // the operating mode and the Daraja environment
    expect(screen.getByText('2 min')).toBeInTheDocument()
    expect(screen.getByText('1 failing')).toBeInTheDocument()
    expect(screen.getByText('daraja: unavailable')).toBeInTheDocument()
    expect(screen.getByText('1 found mismatches')).toBeInTheDocument()
    expect(screen.getByText('Never run')).toBeInTheDocument()
    expect(screen.getByText('12 slots, 1 lucky')).toBeInTheDocument()
  })
})

describe('money switches and jobs (ADR 0036)', () => {
  it('pauses an area with a reason from its switch; staff cannot resume it, an admin can', async () => {
    const posts: { url: URL; body: unknown }[] = []
    const { unmount } = renderSignedIn(<SystemPage />, { roles: ['staff'], route: (m, url, body) => {
      if (m === 'POST') { posts.push({ url, body }); return { status: 200, body: {} } }
      return systemServer(system)(m, url)
    } })
    const plays = await screen.findByRole('switch', { name: 'Plays' })
    expect(plays).toBeChecked()
    await userEvent.click(plays)
    const dialog = await screen.findByRole('dialog', { name: 'Pause plays?' })
    await userEvent.type(within(dialog).getByRole('textbox', { name: 'Why (required)' }), 'odds under review')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Pause' }))
    await waitFor(() => { expect(posts.at(-1)?.url.pathname).toBe('/v1/staff/operating/switches/plays/pause') })
    expect(posts.at(-1)?.body).toEqual({ reason: 'odds under review' })
    unmount()

    const paused: AreaSwitch[] = running.map((s) => (s.area === 'plays' ? { ...s, paused: true, reason: 'odds under review', changed_by: 'staff:s9', at: '2026-11-11T09:00:00Z' } : s))
    const staffView = renderSignedIn(<SystemPage />, { roles: ['staff'], route: systemServer(system, operating, paused) })
    const off = await screen.findByRole('switch', { name: 'Plays' })
    expect(off).not.toBeChecked()
    expect(off).toBeDisabled()
    expect(screen.getByText('An admin resumes it.')).toBeInTheDocument()
    expect(screen.getByText(/odds under review/)).toBeInTheDocument()
    staffView.unmount()
    renderSignedIn(<SystemPage />, { roles: ['admin'], route: systemServer(system, operating, paused) })
    expect(await screen.findByRole('switch', { name: 'Plays' })).toBeEnabled()
  })

  it('names each job, and lets an admin retry a failing one', async () => {
    const posts: URL[] = []
    renderSignedIn(<SystemPage />, { roles: ['admin'], route: (m, url) => {
      if (m === 'POST') { posts.push(url); return { status: 204 } }
      return systemServer(system)(m, url)
    } })
    expect(await screen.findByText('Send M-Pesa prompts')).toBeInTheDocument()
    expect(screen.getAllByText('Asks M-Pesa to show the player the PIN prompt for a top-up they started.')).toHaveLength(1)
    const failing = screen.getByRole('region', { name: 'Failing jobs' })
    await userEvent.click(within(failing).getByRole('button', { name: 'Retry now' }))
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Retry now' }))
    await waitFor(() => { expect(posts.at(-1)?.pathname).toMatch(/^\/v1\/staff\/jobs\/\d+\/retry$/) })
  })

  it('offers no job actions to staff who are not admins', async () => {
    renderSignedIn(<SystemPage />, { roles: ['staff'], route: systemServer(system) })
    const failing = await screen.findByRole('region', { name: 'Failing jobs' })
    expect(within(failing).queryByRole('button', { name: 'Retry now' })).toBeNull()
  })
})

describe('operating mode (ADR 0027)', () => {
  it('lets anyone on staff stop money moving, with a reason', async () => {
    const posts: { url: URL; body: unknown }[] = []
    renderSignedIn(<SystemPage />, { roles: ['staff'], route: (m, url, body) => {
      if (m === 'POST') { posts.push({ url, body }); return { status: 200, body: {} } }
      return systemServer(system)(m, url)
    } })
    expect(await screen.findByText(/A demonstration with sandbox money/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Resume/ })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: /Stop money operations/ }))
    await userEvent.type(await screen.findByRole('textbox', { name: 'Why (required)' }), 'suspected fraud')
    await userEvent.click(screen.getByRole('button', { name: 'Stop now' }))
    await waitFor(() => { expect(posts.at(-1)?.url.pathname).toBe('/v1/staff/operating/stop') })
    expect(posts.at(-1)?.body).toEqual({ reason: 'suspected fraud' })
  })

  it('shows a stop and lets only an admin resume in sandbox', async () => {
    const stopped: OperatingState = { ...operating, status: { ...operating.status, mode: 'disabled',
      last_change: { id: 'c1', mode: 'disabled', reason: 'suspected fraud', changed_by: 'staff:s9', at: '2026-11-07T09:00:00Z' } } }
    const { unmount } = renderSignedIn(<SystemPage />, { roles: ['staff'], route: systemServer(system, stopped) })
    expect(await screen.findByText('Stopped')).toBeInTheDocument()
    expect(screen.getByText(/suspected fraud/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Resume/ })).toBeNull()
    unmount()
    renderSignedIn(<SystemPage />, { roles: ['admin'], route: systemServer(system, stopped) })
    expect(await screen.findByRole('button', { name: /Resume \(sandbox\)/ })).toBeInTheDocument()
  })

  it('offers the approval of a live request only to a different admin', async () => {
    const req = { id: 'r1', licence_reference: 'GRA/OPL/1', approval_reference: 'A1', evidence_sha256: 'ab'.repeat(32), evidence_location: 'vault', reason: 'licensed', requested_by: 'staff:s1', at: '2026-11-07T09:00:00Z' }
    const waiting: OperatingState = { status: { mode: 'disabled', configured: 'live', real_money: false, last_change: null }, live_requests: [req], history: [] }
    const { unmount } = renderSignedIn(<SystemPage />, { roles: ['admin'], route: systemServer(system, waiting) })
    expect(await screen.findByText('Another admin must approve your request.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull()
    unmount()
    renderSignedIn(<SystemPage />, { roles: ['admin'], route: systemServer(system, { ...waiting, live_requests: [{ ...req, requested_by: 'staff:s2' }] }) })
    expect(await screen.findByRole('button', { name: 'Approve' })).toBeInTheDocument()
  })
})

describe('activity', () => {
  const ev: AuditEvent = { id: '42', at: '2026-10-30T09:22:29Z', actor_type: 'player', actor_id: 'u-1', action: 'payment.topup.started', entity_type: 'payment', entity_id: 'p-1', details: { amount_kes: 1 } }
  const byStaff: AuditEvent = { id: '43', at: '2026-10-30T09:30:00Z', actor_type: 'staff', actor_id: 'staff:m1', action: 'identity.account_suspended', entity_type: 'user', entity_id: 'u-1', details: { reason: 'chargeback dispute' } }
  const grace: StaffMember = { id: 'm1', email: 'grace@staff.local', name: 'Grace Wanjiku', roles: ['staff'], active: true, invited_by: 'x', providers: [], last_sign_in: null, created_at: '2026-10-01T00:00:00Z', has_password: true, must_change_password: false }
  const server = (asked: URL[]) => (_m: string, url: URL) => {
    asked.push(url)
    if (url.pathname === '/v1/staff/members') return { status: 200, body: { members: [grace], roles: ['admin', 'staff'] } }
    if (url.pathname === '/v1/staff/activity/export') return { status: 200, body: 'id,at_eat' }
    return { status: 200, body: { events: [ev, byStaff], next_before: null } }
  }

  it('lists audit records with who did what, and filters by actor on the server', async () => {
    const asked: URL[] = []
    renderSignedIn(<ActivityPage />, { roles: ['admin'], route: server(asked) })
    const table = await screen.findByRole('table', { name: /Activity, newest first/ })
    expect(within(table).getByText('payment.topup.started')).toBeInTheDocument()
    expect(within(table).getByText('amount kes: 1')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: 'Staff' }))
    await waitFor(() => { expect(asked.filter((u) => u.pathname === '/v1/staff/activity').at(-1)?.searchParams.get('actor_type')).toBe('staff') })
  })

  it('names staff, links ids to their pages, sends the day, entity and actor filters, and downloads the file (owner, 2026-10-31)', async () => {
    const asked: URL[] = []
    const created: string[] = []
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:activity'), configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: vi.fn(), configurable: true })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { created.push(this.download) })
    renderSignedIn(<ActivityPage />, { roles: ['admin'], route: server(asked) })
    const table = await screen.findByRole('table', { name: /Activity, newest first/ })
    expect(await within(table).findByText('Grace Wanjiku')).toBeInTheDocument()
    expect(within(table).getByText('reason: chargeback dispute')).toBeInTheDocument()
    expect(within(table).getByRole('link', { name: 'p-1' })).toHaveAttribute('href', '/topups?topup=p-1')
    expect(within(table).getAllByRole('link', { name: 'u-1' })[0]).toHaveAttribute('href', '/players?player=u-1')
    const user = userEvent.setup()
    fireEvent.change(screen.getByLabelText('From'), { target: { value: '2026-10-01' } })
    fireEvent.change(screen.getByLabelText('To'), { target: { value: '2026-10-31' } })
    await user.type(screen.getByLabelText('Entity type'), 'user')
    await user.type(screen.getByLabelText('Entity id'), 'u-1')
    await user.type(screen.getByLabelText('Actor id'), 'staff:m1')
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() => {
      const last = asked.filter((u) => u.pathname === '/v1/staff/activity').at(-1)
      expect(last?.searchParams.get('from')).toBe('2026-10-01')
      expect(last?.searchParams.get('to')).toBe('2026-10-31')
      expect(last?.searchParams.get('entity_type')).toBe('user')
      expect(last?.searchParams.get('entity_id')).toBe('u-1')
      expect(last?.searchParams.get('actor_id')).toBe('staff:m1')
    })
    await user.click(screen.getByRole('button', { name: 'Download CSV' }))
    await waitFor(() => { expect(created).toEqual(['activity-2026-10-01-2026-10-31.csv']) })
    const exported = asked.find((u) => u.pathname === '/v1/staff/activity/export')
    expect(exported?.searchParams.get('entity_id')).toBe('u-1')
    expect(exported?.searchParams.get('from')).toBe('2026-10-01')
    expect(await screen.findByRole('status')).toHaveTextContent(/Saved activity-2026-10-01-2026-10-31.csv/)
    click.mockRestore()
  })

  it('arrives with the filters of a link from a player’s timeline', async () => {
    const asked: URL[] = []
    renderSignedIn(<ActivityPage />, { roles: ['admin'], route: server(asked), path: '/activity?entity_id=u-1&action=identity.' })
    await screen.findByRole('table', { name: /Activity, newest first/ })
    const first = asked.find((u) => u.pathname === '/v1/staff/activity')
    expect(first?.searchParams.get('entity_id')).toBe('u-1')
    expect(first?.searchParams.get('action')).toBe('identity.')
    expect(screen.getByLabelText('Entity id')).toHaveValue('u-1')
  })

  it('is for administrators only', () => {
    renderSignedIn(<App />, { roles: ['staff'], route: () => ({ status: 200, body: {} }) })
    const nav = screen.getByRole('navigation', { name: 'Back office' })
    expect(within(nav).queryByRole('link', { name: 'Activity log' })).not.toBeInTheDocument()
    expect(within(nav).getByRole('link', { name: 'Plays' })).toBeInTheDocument()
    expect(within(nav).getByRole('link', { name: 'System status' })).toBeInTheDocument()
  })
})

describe('system alerts everywhere', () => {
  const paging: Alert = { id: 'a1', kind: 'reconciliation', subject: 'payouts', severity: 'page', freezes_payouts: true, summary: 'Payouts reconciliation found mismatches', first_seen_at: '2026-10-30T09:00:00Z', last_seen_at: '2026-10-30T09:30:00Z', occurrences: 2 }
  const warning: Alert = { ...paging, id: 'a2', severity: 'warn', freezes_payouts: false, summary: 'KRA remittance due soon' }

  it('a paging alert shows on every page and the sidebar counts open alerts', async () => {
    renderSignedIn(<App />, { roles: ['staff'], route: (_m, url) => url.pathname === '/v1/staff/alerts' ? { status: 200, body: { alerts: [paging, warning] } } : { status: 200, body: {} } })
    const banner = await screen.findByRole('alert')
    expect(within(banner).getByText('Payouts reconciliation found mismatches')).toBeInTheDocument()
    expect(within(banner).getByText(/Payouts are frozen/)).toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Back office' })
    expect(within(nav).getByRole('link', { name: 'Alerts, 2 open' })).toBeInTheDocument()
  })

  it('warnings alone are counted but do not take over the page', async () => {
    renderSignedIn(<App />, { roles: ['staff'], route: (_m, url) => url.pathname === '/v1/staff/alerts' ? { status: 200, body: { alerts: [warning] } } : { status: 200, body: {} } })
    const nav = screen.getByRole('navigation', { name: 'Back office' })
    expect(await within(nav).findByRole('link', { name: 'Alerts, 1 open' })).toBeInTheDocument()
    expect(screen.queryByText('KRA remittance due soon')).not.toBeInTheDocument()
  })
})

describe('alerts page', () => {
  const unconfirmed: Alert = {
    id: 'a1', kind: 'topup.unconfirmed', subject: 'p-1', severity: 'page', freezes_payouts: false,
    summary: 'A top-up of KES 50 from 254705***005 is not confirmed 4 h 31 min after the player paid',
    details: { payment: 'p-1', amount_kes: 50, msisdn: '254705***005', reason: 'still_pending_at_expiry', escalated_at: '2026-10-30T13:00:00Z', escalated_from: 'warn' },
    first_seen_at: '2026-10-30T09:00:00Z', last_seen_at: '2026-10-30T13:30:00Z', occurrences: 55, notifications: 2,
  }
  const suspense: Alert = { ...unconfirmed, id: 'a2', kind: 'paybill.suspense_open', subject: 'RCPT1', severity: 'warn', summary: 'KES 300 paid to the paybill has sat in suspense 2 h', details: { receipt: 'RCPT1', amount_kes: 300 } }
  const kinds = [{ kind: 'paybill.suspense_open', open: 1 }, { kind: 'topup.unconfirmed', open: 1 }, { kind: 'kyc.queue_stale', open: 0 }]
  const detail: AlertDetail = { ...unconfirmed, check: 'topups', history: [{ at: '2026-10-30T09:00:00Z', event: 'raised', actor: 'system' }, { at: '2026-10-30T13:00:00Z', event: 'escalated', actor: 'system', note: 'a warning became a page' }] }

  const server = (asked: URL[], posted: unknown[]) => (method: string, url: URL, body: unknown) => {
    asked.push(url)
    if (method === 'POST') { posted.push(body); return { status: 200, body: { ...unconfirmed, acknowledged_by: 'staff:s1', acknowledged_at: '2026-10-30T14:00:00Z', ack_note: 'credited from the statement' } } }
    if (url.pathname === '/v1/staff/alerts/a1') return { status: 200, body: detail }
    const sev = url.searchParams.get('severity')
    const kind = url.searchParams.get('kind')
    const rows = [unconfirmed, suspense].filter((a) => (!sev || a.severity === sev) && (!kind || a.kind === kind))
    return { status: 200, body: { alerts: rows, next_before: null, kinds } }
  }

  it('lists alerts with their kind, state and age, and sends the filters to the server', async () => {
    const asked: URL[] = []
    renderSignedIn(<AlertsPage />, { roles: ['staff'], route: server(asked, []) })
    const table = await screen.findByRole('table', { name: /Alerts, newest first/ })
    expect(within(table).getByText(/KES 50 from 254705\*\*\*005/)).toBeInTheDocument()
    expect(within(table).getByText('topup.unconfirmed')).toBeInTheDocument()
    expect(within(table).getAllByText('Open')).toHaveLength(2 + 2) // two state pills, two Open buttons
    expect(asked[0]?.searchParams.get('state')).toBe('open')
    await userEvent.click(screen.getByRole('tab', { name: 'Resolved' }))
    await waitFor(() => { expect(asked.at(-1)?.searchParams.get('state')).toBe('resolved') })
    await userEvent.click(screen.getByRole('tab', { name: 'Pages' }))
    await waitFor(() => { expect(asked.at(-1)?.searchParams.get('severity')).toBe('page') })
    const kindSelect = screen.getByRole('combobox', { name: 'Kind' })
    expect(within(kindSelect).getByRole('option', { name: 'topup.unconfirmed (1 open)' })).toBeInTheDocument()
    expect(within(kindSelect).getByRole('option', { name: 'kyc.queue_stale' })).toBeInTheDocument()
    await userEvent.selectOptions(kindSelect, 'paybill.suspense_open')
    await waitFor(() => { expect(asked.at(-1)?.searchParams.get('kind')).toBe('paybill.suspense_open') })
    expect(asked.at(-1)?.searchParams.get('limit')).toBe('100')
  })

  it('opens an alert in a sheet with its details, the runbook hint, its history and the acknowledge action', async () => {
    const asked: URL[] = []
    const posted: unknown[] = []
    renderSignedIn(<AlertsPage />, { roles: ['staff'], route: server(asked, posted) })
    const table = await screen.findByRole('table', { name: /Alerts, newest first/ })
    await userEvent.click(within(table).getAllByRole('button', { name: 'Open' })[0] as HTMLElement)
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByText(/raised by the topups check/)).toBeInTheDocument()
    expect(within(sheet).getByText('amount kes')).toBeInTheDocument()
    expect(within(sheet).getByText('50')).toBeInTheDocument()
    expect(within(sheet).getByText('still_pending_at_expiry')).toBeInTheDocument()
    expect(within(sheet).getByText(/M-Pesa may have taken the money/)).toBeInTheDocument()
    expect(within(sheet).getByRole('link', { name: /Open the top-up/ })).toHaveAttribute('href', '/topups?topup=p-1')
    expect(within(sheet).getByText('escalated')).toBeInTheDocument()
    expect(within(sheet).getByText('a warning became a page')).toBeInTheDocument()
    await userEvent.click(within(sheet).getByRole('button', { name: 'Acknowledge' }))
    await userEvent.type(await screen.findByLabelText(/^What you did or found/), 'credited from the statement')
    await userEvent.click(screen.getAllByRole('button', { name: 'Acknowledge' }).at(-1) as HTMLElement)
    await waitFor(() => { expect(posted).toEqual([{ note: 'credited from the statement' }]) })
    expect(asked.some((u) => u.pathname === '/v1/staff/alerts/a1/acknowledge')).toBe(true)
  })

  it('opens the alert an email linked to', async () => {
    renderSignedIn(<AlertsPage />, { roles: ['staff'], route: server([], []), path: '/alerts?alert=a1' })
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByText(/raised by the topups check/)).toBeInTheDocument()
  })

  it('names the channels alerts go to on the System page', async () => {
    renderSignedIn(<SystemPage />, { roles: ['staff'], route: systemServer({ ...system, settings: { ...system.settings, alerts_delivery: 'webhook, email' } }) })
    expect(await screen.findByText('Webhook')).toBeInTheDocument()
    expect(screen.getAllByText('Email')).toHaveLength(2) // the SMTP row and the channel
  })
})
