import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { PaymentsPage } from './pages/Payments'
import { renderSignedIn } from './test/render'
import type { StatementApplied, StatementReport } from './types'

const csv = 'Receipt No.,Completion Time,Paid In,Other Party Info,A/C No.\nST22222222,2026-10-30 21:40:00,100.00,254796***457 - WILLIS,4draws\n'

const report: StatementReport = {
  sha256: 'a'.repeat(64), rows: 2, from: '2026-10-30T18:30:00Z', to: '2026-10-30T18:40:00Z', counts: { matched: 1, unmatched: 1 },
  matches: [
    { row: { line: 2, receipt: 'ST22222222', completed_at: '2026-10-30T18:40:00Z', paid_in_kes: 100, party: '254796***457 - WILLIS', party_number: '254796***457', account: '4draws', details: '', status: 'Completed' },
      kind: 'matched', payment_id: 'p-9', user_id: 'u-1', msisdn: '254796992457', status: 'unconfirmed', reason: 'same amount, number and time' },
    { row: { line: 3, receipt: 'XX33333333', completed_at: '2026-10-30T18:41:00Z', paid_in_kes: 250, party: '254700***000 - NOBODY', party_number: '254700***000', account: '4draws', details: '', status: 'Completed' },
      kind: 'unmatched', reason: 'no prompt with this amount and number around this time' },
  ],
  to_credit: [{ payment_id: 'p-9', receipt: 'ST22222222', amount_kes: 100 }],
}

const applied: StatementApplied = { import_id: 'imp-1', credited: 1, credited_kes: 100, results: [{ payment_id: 'p-9', receipt: 'ST22222222', amount_kes: 100, credited: true }] }

describe('the M-Pesa statement', () => {
  it('is uploaded, matched, confirmed and applied, and the import is listed', async () => {
    const calls: { method: string; url: URL; body: unknown }[] = []
    let imported = false
    renderSignedIn(<PaymentsPage />, {
      roles: ['staff'],
      route: (method, url, body) => {
        calls.push({ method, url, body })
        if (url.pathname === '/v1/staff/payments/statement/match') return { status: 200, body: { filename: '', report } }
        if (url.pathname === '/v1/staff/payments/statement/apply') { imported = true; return { status: 200, body: applied } }
        if (url.pathname === '/v1/staff/payments/statements') {
          return { status: 200, body: { imports: imported ? [{ id: 'imp-1', imported_by: 'staff:s1', imported_at: '2026-10-31T09:00:00Z', filename: 'oct.csv', sha256: 'a'.repeat(64), rows: 2, matched: 1, credited: 1, credited_kes: 100 }] : [] } }
        }
        return { status: 200, body: {} }
      },
    })
    expect(await screen.findByText('No statement imported yet.')).toBeInTheDocument()
    const file = new File([csv], 'oct.csv', { type: 'text/csv' })
    await userEvent.upload(screen.getByLabelText('Statement CSV'), file)
    const table = await screen.findByRole('table', { name: 'Statement rows and what they match' })
    expect(within(table).getByText('Matched')).toBeInTheDocument()
    expect(within(table).getByText('No prompt fits')).toBeInTheDocument()
    expect(within(table).getByRole('checkbox', { name: 'Credit ST22222222' })).toBeChecked()
    const match = calls.find((c) => c.url.pathname.endsWith('/statement/match'))
    expect(match?.body).toBe(csv)

    await userEvent.click(screen.getByRole('button', { name: /Credit 1 selected/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Credit 1 top-up' })
    await userEvent.click(within(dialog).getByRole('button', { name: 'Credit' }))
    await waitFor(() => {
      const apply = calls.find((c) => c.url.pathname.endsWith('/statement/apply'))
      expect(apply?.body).toMatchObject({ filename: 'oct.csv', sha256: 'a'.repeat(64), rows: 2, credits: [{ payment_id: 'p-9', receipt: 'ST22222222', amount_kes: 100 }] })
    })
    expect(await screen.findByText(/Credited 1 top-up/)).toBeInTheDocument()
    const past = await screen.findByRole('table', { name: 'Past statement imports' })
    expect(within(past).getByText('oct.csv')).toBeInTheDocument()
  })
})
