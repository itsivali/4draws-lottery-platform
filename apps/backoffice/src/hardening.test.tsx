import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { renderSignedIn } from './test/render'
import type { StaffMember } from './types'

const members: StaffMember[] = [
  { id: 's1', email: 'alice@staff.local', name: 'Alice', roles: ['admin'], active: true, invited_by: 'bootstrap', providers: ['google'], last_sign_in: '2026-10-26T09:00:00Z', created_at: '2026-10-26T08:00:00Z', has_password: true, must_change_password: false },
]

const submission = { user_id: 'u1', msisdn: '254712345678', national_id: '12345678', full_name: 'Amina Otieno', date_of_birth: '1992-03-04', submitted_at: '2026-10-26T09:00:00Z' }

describe('forms refuse what the server would refuse', () => {
  it('an invite needs a plausible email, and a temporary password of 12 characters or more when given', async () => {
    renderSignedIn(<App />, { roles: ['admin'], path: '/staff', route: () => ({ status: 200, body: { members } }) })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Invite a member' }))
    const dialog = await screen.findByRole('dialog', { name: 'Invite a member' })
    const invite = within(dialog).getByRole('button', { name: 'Invite' })
    await user.type(within(dialog).getByLabelText(/^Email address/), 'not-an-email')
    expect(within(dialog).getByText('Enter an email address.')).toBeInTheDocument()
    expect(invite).toBeDisabled()
    await user.clear(within(dialog).getByLabelText(/^Email address/))
    await user.type(within(dialog).getByLabelText(/^Email address/), 'tara@staff.local')
    expect(within(dialog).getByRole('radio', { name: /^Staff/ })).toBeChecked() // the usual role, chosen by default
    expect(invite).toBeDisabled() // the back office shows members by name, so an invite needs one
    await user.type(within(dialog).getByLabelText(/^Name/), 'Tara')
    expect(invite).toBeEnabled() // a password is optional: Google or Apple may be their way in
    await user.type(within(dialog).getByLabelText(/^Temporary password/), 'short1')
    expect(within(dialog).getByText(/Use at least 12 characters/)).toBeInTheDocument()
    expect(invite).toBeDisabled()
    await user.type(within(dialog).getByLabelText(/^Temporary password/), '-but-now-long-enough')
    expect(invite).toBeEnabled()
  })

  it('an amount must be whole shillings above zero', async () => {
    renderSignedIn(<App />, { roles: ['staff'], path: '/payments', route: () => ({ status: 200, body: {} }) })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Resolve an unconfirmed top-up' }))
    const dialog = await screen.findByRole('dialog', { name: 'Resolve an unconfirmed top-up' })
    await user.type(within(dialog).getByLabelText(/^Payment id/), 'p1')
    await user.type(within(dialog).getByLabelText(/^M-Pesa receipt/), 'ABC123')
    await user.type(within(dialog).getByLabelText(/^Amount paid/), '0')
    expect(within(dialog).getByText('Enter a whole number of shillings above zero.')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Credit' })).toBeDisabled()
    await user.clear(within(dialog).getByLabelText(/^Amount paid/))
    await user.type(within(dialog).getByLabelText(/^Amount paid/), '250')
    expect(within(dialog).getByRole('button', { name: 'Credit' })).toBeEnabled()
  })
})

describe('the identity queue stays honest', () => {
  it('a decision re-reads the count with the list, so no false arrival is raised', async () => {
    let pending = [submission]
    const { fetch } = renderSignedIn(<App />, {
      roles: ['staff'],
      path: '/kyc',
      route: (method, url) => {
        if (url.pathname === '/v1/staff/kyc/queue') return { status: 200, body: { waiting: pending.length, oldest_submitted_at: null } }
        if (method === 'GET' && url.pathname === '/v1/staff/kyc/pending') return { status: 200, body: { submissions: pending } }
        if (method === 'POST' && url.pathname === '/v1/staff/kyc/u1/approve') { pending = []; return { status: 200, body: { status: 'verified' } } }
        return { status: 404 }
      },
    })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    const dialog = await screen.findByRole('dialog', { name: 'Approve Amina Otieno?' })
    await user.type(within(dialog).getByLabelText(/What you checked/), 'ID card seen')
    await user.click(within(dialog).getByRole('button', { name: 'Approve' }))
    expect(await screen.findByText('No submission is waiting for review.')).toBeInTheDocument()
    const queueReads = fetch.mock.calls.filter(([u]) => (u as string).endsWith('/v1/staff/kyc/queue')).length
    expect(queueReads).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText(/new submission/)).not.toBeInTheDocument()
  })
})
