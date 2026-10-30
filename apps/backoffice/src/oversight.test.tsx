import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { ActivityPage } from './pages/Activity'
import { PlaysPage } from './pages/Plays'
import { SystemPage } from './pages/System'
import { TopUpsPage } from './pages/TopUps'
import { renderSignedIn } from './test/render'
import type { Alert, AuditEvent, PlayRow, SystemState, TopUpRow } from './types'

const play: PlayRow = {
  ticket_id: 't-1', user_id: 'u-1', msisdn: '254705369005', full_name: 'Willis Ivali', play_date: '2026-10-30', seq: 2,
  price_kes: 1, status: 'paid', slot: 0, lucky: true, created_at: '2026-10-30T09:23:04Z',
}

const topUp: TopUpRow = {
  id: 'p-1', user_id: 'u-1', msisdn: '254705369005', full_name: 'Willis Ivali', channel: 'stk', amount_requested_kes: 1,
  amount_confirmed_kes: 1, status: 'credited', failure_reason: null, result_code: 1037, receipt: 'SANDBOX-p-1', sandbox: true,
  created_at: '2026-10-30T09:22:29Z', updated_at: '2026-10-30T09:22:58Z',
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
  open_alerts: 1, players: 4, plays_today: 3, top_ups_pending: 0,
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
    expect(within(table).getByText(/phone did not answer/)).toBeInTheDocument()
    expect(within(table).getByText('SANDBOX-p-1')).toBeInTheDocument()
  })
})

describe('system status', () => {
  it('shows the settings, failing jobs and reconciliations', async () => {
    renderSignedIn(<SystemPage />, { roles: ['staff'], route: () => ({ status: 200, body: system }) })
    expect(await screen.findByText('Sandbox')).toBeInTheDocument()
    expect(screen.getByText('2 min')).toBeInTheDocument()
    expect(screen.getByText('1 failing')).toBeInTheDocument()
    expect(screen.getByText('daraja: unavailable')).toBeInTheDocument()
    expect(screen.getByText('1 found mismatches')).toBeInTheDocument()
    expect(screen.getByText('Never run')).toBeInTheDocument()
    expect(screen.getByText('12 slots, 1 lucky')).toBeInTheDocument()
  })
})

describe('activity', () => {
  it('lists audit records with who did what, and filters by actor on the server', async () => {
    const ev: AuditEvent = { id: '42', at: '2026-10-30T09:22:29Z', actor_type: 'player', actor_id: 'u-1', action: 'payment.topup.started', entity_type: 'payment', entity_id: 'p-1', details: { amount_kes: 1 } }
    const asked: URL[] = []
    renderSignedIn(<ActivityPage />, { roles: ['admin'], route: (_m, url) => { asked.push(url); return { status: 200, body: { events: [ev], next_before: null } } } })
    const table = await screen.findByRole('table', { name: /Activity, newest first/ })
    expect(within(table).getByText('payment.topup.started')).toBeInTheDocument()
    expect(within(table).getByText('amount kes: 1')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: 'Staff' }))
    await waitFor(() => { expect(asked.at(-1)?.searchParams.get('actor_type')).toBe('staff') })
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
