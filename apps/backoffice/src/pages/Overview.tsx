import { Link } from 'react-router-dom'
import { useSession } from '../context'
import { ReconcileCard } from '../components/Reconcile'
import { PageHeader } from '../components/ui'
import { can, roleLabels, visibleSections } from '../roles'

export function OverviewPage() {
  const { me } = useSession()
  return (
    <section>
      <PageHeader title={`Signed in as ${me.name || me.email}`}>
        Your roles: {me.roles.map((r) => roleLabels[r]).join(', ')}. Every action you take is recorded in the audit log under your name.
      </PageHeader>
      <ul className="cards">
        {visibleSections(me.roles).map((s) => (
          <li key={s.path} className="card"><Link to={s.path}>{s.label}</Link></li>
        ))}
      </ul>
      {can(me.roles, 'auditor', 'draw_official') ? <ReconcileCard path="/v1/staff/draws/reconcile" title="Draws reconciliation" /> : null}
      {can(me.roles, 'auditor', 'payout_operator') ? <ReconcileCard path="/v1/staff/payouts/reconcile" title="Payouts reconciliation" /> : null}
    </section>
  )
}
