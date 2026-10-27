import type { ColumnDef } from '@tanstack/react-table'
import { UserPlus, Users } from 'lucide-react'
import { useMemo } from 'react'
import { Actions, DataTable, Stack } from '../components/DataTable'
import { useConfirmedAction } from '../components/useConfirmedAction'
import { Empty, Loaded, PageHeader, Pill } from '../components/ui'
import { useSession } from '../context'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { roleLabels } from '../roles'
import type { Role, StaffMember } from '../types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

const allRoles = Object.keys(roleLabels) as Role[]

const roleFields = allRoles.map((r) => ({ name: r, label: roleLabels[r], type: 'checkbox' as const }))

const checked = (v: Record<string, string>): Role[] => allRoles.filter((r) => v[r] === 'true')

/** Admins invite staff by the email of their Google or Apple account, assign roles and disable members (ADR 0018). */
export function StaffPage() {
  const { api, me } = useSession()
  const load = useLoad(() => api.get<{ members: StaffMember[] }>('/v1/staff/members'))
  const { ask, busy, view } = useConfirmedAction(load.reload)

  const columns = useMemo<ColumnDef<StaffMember>[]>(() => [
    {
      id: 'member', header: 'Member', accessorFn: (m) => `${m.name} ${m.email}`,
      cell: ({ row }) => <Stack primary={<span className={row.original.active ? '' : 'text-muted-foreground'}>{row.original.name || row.original.email}</span>} secondary={row.original.email} />,
    },
    {
      id: 'roles', header: 'Roles', accessorFn: (m) => m.roles.map((r) => roleLabels[r]).join(', '), enableSorting: false,
      cell: ({ row }) => (row.original.roles.length ? <div className="flex flex-wrap gap-1">{row.original.roles.map((r) => <Badge key={r} variant="secondary" className="rounded-full">{roleLabels[r]}</Badge>)}</div> : <span className="text-sm text-muted-foreground">No role</span>),
    },
    {
      id: 'signin', header: 'Sign-in', accessorFn: (m) => m.last_sign_in ?? '',
      cell: ({ row }) => {
        const m = row.original
        return (
          <div className="grid justify-items-start gap-1">
            {m.active ? null : <Pill tone="error">Disabled</Pill>}
            {m.last_sign_in ? <Stack primary={<span className="font-normal">Last {eat(m.last_sign_in)}</span>} secondary={m.providers.join(', ')} /> : <span className="text-sm text-muted-foreground">Never signed in</span>}
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
                  title: `Roles of ${m.email}`,
                  body: <p>Choose what {m.name || m.email} may do.</p>,
                  fields: roleFields,
                  initial: Object.fromEntries(m.roles.map((r) => [r, 'true'])),
                  confirmLabel: 'Save roles',
                  success: `Roles of ${m.email} saved.`,
                  act: (v) => api.put(`/v1/staff/members/${m.id}/roles`, { roles: checked(v) }),
                })
              }}
            >
              Change roles
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
                  body: <p>They can sign in again with their roles.</p>,
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
                body: <p>They can sign in once they use a Google or Apple account with this verified email address.</p>,
                fields: [
                  { name: 'email', label: 'Email address', required: true },
                  { name: 'name', label: 'Name' },
                  ...roleFields,
                ],
                confirmLabel: 'Invite',
                success: 'Invited.',
                act: (v) => api.post('/v1/staff/members', { email: v.email, name: v.name ?? '', roles: checked(v) }),
              })
            }}
          >
            <UserPlus aria-hidden="true" /> Invite a member
          </Button>
        }
      >
        Staff sign in with the Google or Apple account of the email address you invite. Roles apply from their next
        request; disabling a member ends their sessions at once. Every change is recorded in the audit log.
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
