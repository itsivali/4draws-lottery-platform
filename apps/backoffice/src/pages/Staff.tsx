import type { ColumnDef } from '@tanstack/react-table'
import { KeyRound, UserPlus, Users } from 'lucide-react'
import { useMemo } from 'react'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill, type ConfirmField } from '../components/ui'
import { useSession } from '../context'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { roleDescriptions, roleLabels } from '../roles'
import type { Role, StaffMember } from '../types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

const roleField: ConfirmField = {
  name: 'role', label: 'Role', type: 'radio', required: true,
  options: (['staff', 'admin'] as Role[]).map((r) => ({ value: r, label: roleLabels[r], description: roleDescriptions[r] })),
}

const passwordHelp = 'Tell it to them in person or by phone, never in a chat that keeps history. They must change it at their first sign-in.'

/** Admins invite members, set their role, give them temporary passwords and disable them (ADR 0019). */
export function StaffPage() {
  const { api, me } = useSession()
  const load = useLoad((signal) => api.get<{ members: StaffMember[] }>('/v1/staff/members', undefined, { signal }))
  const { ask, busy, view } = useConfirmedAction(load.reload)

  const columns = useMemo<ColumnDef<StaffMember>[]>(() => [
    {
      id: 'member', header: 'Member', accessorFn: (m) => `${m.name} ${m.email}`,
      cell: ({ row }) => <Stack primary={<span className={row.original.active ? '' : 'text-muted-foreground'}>{row.original.name || row.original.email}</span>} secondary={row.original.email} />,
    },
    {
      id: 'role', header: 'Role', accessorFn: (m) => m.roles.map((r) => roleLabels[r]).join(', '),
      cell: ({ row }) => (row.original.roles.length ? <div className="flex flex-wrap gap-1">{row.original.roles.map((r) => <Badge key={r} variant={r === 'admin' ? 'default' : 'secondary'} className="rounded-full">{roleLabels[r]}</Badge>)}</div> : <span className="text-sm text-muted-foreground">No role</span>),
    },
    {
      id: 'signin', header: 'Sign-in', accessorFn: (m) => m.last_sign_in ?? '',
      cell: ({ row }) => {
        const m = row.original
        const ways = [...(m.has_password ? [m.must_change_password ? 'temporary password' : 'password'] : []), ...m.providers]
        return (
          <div className="grid justify-items-start gap-1">
            {m.active ? null : <Pill tone="error">Disabled</Pill>}
            {m.has_password && m.must_change_password ? <Pill tone="pending">Must change password</Pill> : null}
            {m.last_sign_in ? <Stack primary={<span className="font-normal">Last {eat(m.last_sign_in)}</span>} secondary={ways.join(', ')} /> : <Stack primary={<span className="font-normal text-muted-foreground">Never signed in</span>} secondary={ways.length ? ways.join(', ') : 'no password yet'} />}
          </div>
        )
      },
    },
    {
      id: 'actions', header: () => <span className="sr-only">Actions</span>, enableSorting: false,
      cell: ({ row }) => {
        const m = row.original
        if (m.id === me.id) return <Actions><Badge variant="outline" className="rounded-full">You</Badge></Actions>
        return (
          <Actions>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => {
                ask({
                  title: `Role of ${m.email}`,
                  body: <p>Choose what {m.name || m.email} may do. It applies from their next request.</p>,
                  fields: [roleField],
                  initial: { role: m.roles.includes('admin') ? 'admin' : 'staff' },
                  confirmLabel: 'Save role',
                  success: `Role of ${m.email} saved.`,
                  act: (v) => api.put(`/v1/staff/members/${m.id}/roles`, { roles: [v.role] }),
                })
              }}
            >
              Change role
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => {
                ask({
                  title: `Reset the password of ${m.email}`,
                  body: <p>Their sessions end at once. They sign in with this temporary password and must choose their own.</p>,
                  fields: [{ name: 'password', label: 'Temporary password', type: 'password', required: true, help: passwordHelp }],
                  confirmLabel: 'Set temporary password',
                  success: `${m.email} has a temporary password.`,
                  act: (v) => api.post(`/v1/staff/members/${m.id}/password`, { password: v.password }),
                })
              }}
            >
              <KeyRound aria-hidden="true" /> Reset password
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className={m.active ? 'border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive' : ''}
              disabled={busy}
              onClick={() => {
                ask(m.active ? {
                  title: `Disable ${m.email}?`,
                  body: <p>Their sessions end at once and they cannot sign in until re-enabled.</p>,
                  confirmLabel: 'Disable',
                  danger: true,
                  success: `${m.email} is disabled.`,
                  act: () => api.post(`/v1/staff/members/${m.id}/disable`),
                } : {
                  title: `Enable ${m.email}?`,
                  body: <p>They can sign in again with their role.</p>,
                  confirmLabel: 'Enable',
                  success: `${m.email} is enabled.`,
                  act: () => api.post(`/v1/staff/members/${m.id}/enable`),
                })
              }}
            >
              {m.active ? 'Disable' : 'Enable'}
            </Button>
          </Actions>
        )
      },
    },
  ], [api, ask, busy, me.id])

  return (
    <section>
      <PageHeader
        title="Staff"
        actions={
          <Button
            type="button"
            disabled={busy}
            onClick={() => {
              ask({
                title: 'Invite a member',
                body: <p>Their email address is their username. Give them a temporary password, or leave it empty if they will sign in with the Google or Apple account of that address.</p>,
                fields: [
                  { name: 'email', label: 'Email address', required: true, type: 'email' },
                  { name: 'name', label: 'Name' },
                  { ...roleField },
                  { name: 'password', label: 'Temporary password', type: 'password', help: passwordHelp },
                ],
                initial: { role: 'staff' },
                confirmLabel: 'Invite',
                success: 'Invited.',
                act: (v) => api.post('/v1/staff/members', { email: v.email, name: v.name ?? '', roles: [v.role], ...(v.password ? { password: v.password } : {}) }),
              })
            }}
          >
            <UserPlus aria-hidden="true" /> Invite a member
          </Button>
        }
      >
        Two roles: <strong>Staff</strong> work every queue and see Finance; <strong>Administrators</strong> also manage
        staff and see App metrics. A role applies from the member’s next request; disabling a member or resetting their
        password ends their sessions at once. Every change is recorded in the audit log.
      </PageHeader>
      {view}
      <Loaded load={load} what="staff">
        {({ members }) =>
          members.length === 0 ? <Empty icon={Users}>No members yet.</Empty> : (
            <DataTable columns={columns} data={members} rowId={(m) => m.id} caption="Staff members, active first" filter="Search by name or email" />
          )
        }
      </Loaded>
    </section>
  )
}
