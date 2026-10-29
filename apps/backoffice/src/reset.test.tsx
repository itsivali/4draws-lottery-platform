import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApiClient } from './api'
import { ResetPassword } from './ResetPassword'

function setup(status: number, token = 'tok-123') {
  const fetch = vi.fn<typeof globalThis.fetch>(() => Promise.resolve(new Response(status === 204 ? null : JSON.stringify({ type: 'https://lottery.example/problems/invalid-token', title: 'This reset link has expired or was already used', status }), { status, headers: { 'content-type': 'application/problem+json' } })))
  const api = new ApiClient('https://api.test', () => Promise.resolve(null), fetch)
  const onDone = vi.fn()
  render(<ResetPassword api={api} token={token} onDone={onDone} />)
  return { fetch, onDone }
}

describe('reset password page', () => {
  it('sets the password from the link’s token, then sends the member to sign in', async () => {
    const { fetch, onDone } = setup(204)
    const user = userEvent.setup()
    const save = screen.getByRole('button', { name: 'Set password' })
    await user.type(screen.getByLabelText('New password'), 'a brand new long password')
    await user.type(screen.getByLabelText('New password again'), 'a brand new long passwor')
    expect(save).toBeDisabled()
    await user.type(screen.getByLabelText('New password again'), 'd')
    await user.click(save)
    expect(await screen.findByRole('status')).toHaveTextContent('Your password is set.')
    const [url, init] = fetch.mock.calls[0] ?? []
    expect(url).toBe('https://api.test/v1/staff/password/reset')
    expect(JSON.parse(init?.body as string)).toEqual({ token: 'tok-123', new_password: 'a brand new long password' })
    await user.click(screen.getByRole('button', { name: 'Go to sign-in' }))
    expect(onDone).toHaveBeenCalled()
  })

  it('says when the link is spent, and asks for a new one', async () => {
    setup(410)
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('New password'), 'a brand new long password')
    await user.type(screen.getByLabelText('New password again'), 'a brand new long password')
    await user.click(screen.getByRole('button', { name: 'Set password' }))
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('This link has expired or was already used.')
    expect(within(alert).getByText(/Ask for a new one/)).toBeInTheDocument()
  })

  it('refuses to start without a token', () => {
    setup(204, '')
    expect(screen.getByText(/This link is incomplete/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Set password' })).toBeDisabled()
  })
})
