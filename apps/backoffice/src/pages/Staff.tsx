import { useConfirmedAction } from '../components/useConfirmedAction'
import { Loaded, PageHeader, Pill } from '../components/ui'
import { useSession } from '../context'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { roleLabels } from '../roles'
import type { Role, StaffMember } from '../types'

const allRoles = Object.keys(roleLabels) as Role[]

const roleFields = allRoles.map((r) => ({ name: r, label: roleLabels[r], type: 'checkbox' as const }))

const checked = (v: Record<string, string>): Role[] => allRoles.filter((r) => v[r] === 'true')

/** Admins invite staff by the email of their Google or Apple account, assign roles and disable members (ADR 0018). */
export function StaffPage() {
  const { api, me } = useSession()
  const load = useLoad(() => api.get<{ members: StaffMember[] }>('/v1/staff/members'))
  const { ask, busy, view } = useConfirmedAction(load.reload)
  return (
    <section>
      <PageHeader title="Staff">
        Staff sign in with the Google or Apple account of the email address you invite. Roles apply from their next
        request; disabling a member ends their sessions at once. Every change is recorded in the audit log.
      </PageHeader>
      <button
        type="button"
        className="primary"
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
        Invite a member
      </button>
      {view}
      <Loaded load={load} what="staff">
        {({ members }) => (
          <table>
            <caption className="sr-only">Staff members, active first</caption>
            <thead>
              <tr><th scope="col">Member</th><th scope="col">Roles</th><th scope="col">Sign-in</th><th scope="col"><span className="sr-only">Actions</span></th></tr>
            </thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.id}>
                  <td><strong>{m.name || m.email}</strong><br /><span className="muted">{m.email}</span></td>
                  <td>{m.roles.length ? m.roles.map((r) => roleLabels[r]).join(', ') : <span className="muted">No role</span>}</td>
                  <td>
                    {m.active ? null : <><Pill tone="error">Disabled</Pill><br /></>}
                    {m.last_sign_in ? <>Last {eat(m.last_sign_in)}<br /><span className="muted">{m.providers.join(', ')}</span></> : <span className="muted">Never signed in</span>}
                  </td>
                  <td className="actions">
                    {m.id === me.id ? <span className="muted">You</span> : (
                      <>
                        <button
                          type="button"
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
                        </button>
                        <button
                          type="button"
                          className={m.active ? 'danger-outline' : ''}
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
                        </button>
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Loaded>
    </section>
  )
}
