import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { App } from './App'
import { renderSignedIn, type Route } from './test/render'

const sub = (id: string) => ({ user_id: id, msisdn: '254712345678', national_id: '1234567' + id.slice(-1), full_name: `Player ${id}`, date_of_birth: '1992-03-04', submitted_at: '2026-10-26T09:00:00Z' })

/** A server whose identity queue holds the given submissions; queue reads report counts only. */
function server(queue: { subs: ReturnType<typeof sub>[] }): Route {
  return (_method, url) => {
    if (url.pathname === '/v1/staff/kyc/queue') {
      return { status: 200, body: { waiting: queue.subs.length, oldest_submitted_at: queue.subs[0]?.submitted_at ?? null } }
    }
    if (url.pathname === '/v1/staff/kyc/pending') return { status: 200, body: { submissions: queue.subs } }
    return { status: 404 }
  }
}

function poll() {
  act(() => { fireEvent.focus(window) })
}

afterEach(() => { document.title = 'Back office' })

describe('identity review notifications', () => {
  it('a reviewer sees how many submissions wait, in the navigation and the tab title', async () => {
    renderSignedIn(<App />, { roles: ['kyc_reviewer'], route: server({ subs: [sub('u1'), sub('u2')] }) })
    const nav = screen.getByRole('navigation', { name: 'Back office' })
    expect(await within(nav).findByRole('link', { name: 'Identity review, 2 waiting' })).toBeInTheDocument()
    await waitFor(() => { expect(document.title).toBe('(2) Back office') })
    expect(await screen.findByRole('link', { name: /2 identity submissions wait for review/ })).toHaveAttribute('href', '/kyc')
  })

  it('a new submission raises a notice that leads to the review', async () => {
    const queue = { subs: [sub('u1')] }
    renderSignedIn(<App />, { roles: ['kyc_reviewer'], route: server(queue) })
    await screen.findByRole('link', { name: 'Identity review, 1 waiting' })
    expect(screen.queryByText(/new identity submission/i)).not.toBeInTheDocument()

    queue.subs = [sub('u1'), sub('u2')]
    poll()
    const notice = await screen.findByRole('status')
    expect(notice).toHaveTextContent('A new identity submission is waiting for review.')
    const user = userEvent.setup()
    await user.click(within(notice).getByRole('link', { name: 'Review now' }))
    expect(await screen.findByRole('heading', { name: 'Identity review' })).toBeInTheDocument()
    expect(screen.queryByText(/is waiting for review/)).not.toBeInTheDocument()
    expect(document.title).toBe('(2) Back office')
  })

  it('the review list offers newly arrived submissions instead of unmasking them unasked', async () => {
    const queue = { subs: [sub('u1')] }
    const { fetch } = renderSignedIn(<App />, { roles: ['kyc_reviewer'], route: server(queue), path: '/kyc' })
    expect(await screen.findByText('Player u1')).toBeInTheDocument()
    const listed = () => fetch.mock.calls.filter(([u]) => (u as string).endsWith('/v1/staff/kyc/pending')).length
    expect(listed()).toBe(1)

    queue.subs = [sub('u1'), sub('u2')]
    poll()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Show 1 new submission' }))
    expect(await screen.findByText('Player u2')).toBeInTheDocument()
    expect(listed()).toBe(2)
  })

  it('nobody else polls the identity queue', async () => {
    const { fetch } = renderSignedIn(<App />, { roles: ['auditor'], route: () => ({ status: 200, body: { ok: true, totals: {}, attention: {} } }) })
    expect(await screen.findByRole('heading', { name: /Signed in as Alice/ })).toBeInTheDocument()
    poll()
    expect(fetch.mock.calls.some(([u]) => (u as string).includes('/kyc/'))).toBe(false)
    expect(document.title).toBe('Back office')
  })
})
