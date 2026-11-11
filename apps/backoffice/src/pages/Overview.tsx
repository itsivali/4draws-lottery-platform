import { ArrowRight, Banknote, Bell, ChartSpline, Dices, Gauge, History, IdCard, Landmark, Receipt, ShieldAlert, Smartphone, Ticket, UserRound, UserX, Users, Wallet, type LucideIcon } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useAlertWatch } from '../alertWatch'
import { useSession } from '../context'
import { Coin3D, StatCard, Tilt } from '../components/polish'
import { ReconcileCard } from '../components/Reconcile'
import { PageHeader, SectionTitle } from '../components/ui'
import { eat } from '../format'
import { useLoad } from '../hooks'
import { useReviewQueue } from '../reviewQueue'
import { roleLabels, visibleSections } from '../roles'
import { Badge } from '@/components/ui/badge'

const icons: Record<string, LucideIcon> = {
  '/finance': Landmark, '/metrics': ChartSpline, '/players': UserRound, '/kyc': IdCard, '/draws': Ticket, '/payouts': Banknote, '/payments': Wallet, '/tax': Receipt, '/alerts': Bell, '/closures': UserX, '/staff': Users, '/compliance': ShieldAlert,
  '/plays': Dices, '/topups': Smartphone, '/system': Gauge, '/activity': History,
}

const blurbs: Record<string, string> = {
  '/finance': 'Money in and out, what the platform earned and owes.',
  '/metrics': 'How the web and mobile apps are used, per platform.',
  '/players': 'Look any player up; suspend or reinstate an account.',
  '/kyc': 'Compare submissions with the national ID and decide.',
  '/draws': 'Sign frozen draws and approve conducted ones.',
  '/payouts': 'Approve large withdrawals; settle unclear ones.',
  '/payments': 'Reconciliations, reversals and suspense.',
  '/tax': 'Prepare KRA batches and record remittances.',
  '/alerts': 'Acknowledge what went wrong and lift freezes.',
  '/closures': 'Close accounts whose holders asked.',
  '/staff': 'Invite members, set roles and passwords.',
  '/compliance': 'Monitoring cases, holds on players’ money, and the rules.',
  '/plays': 'Every spin: who played, what the wheel landed on, and the proof it was fair.',
  '/topups': 'M-Pesa prompts and their receipts; credit, close or reverse one.',
  '/system': 'Health, background jobs and switches that pause money areas.',
  '/activity': 'Who did what, and when: the audit log of every staff action.',
}

/** A count from a list the server pages: "100+" when there are more than one page holds. */
function countOf(n: number | undefined, more: boolean): string {
  if (n === undefined) return '…'
  return more ? `${String(n)}+` : String(n)
}

export function OverviewPage() {
  const { me, api } = useSession()
  const { queue } = useReviewQueue()
  const { open: alerts } = useAlertWatch()
  const sections = visibleSections(me.roles)
  const held = useLoad((signal) => api.get<{ payouts: unknown[]; next_after: string | null }>('/v1/staff/payouts', { state: 'held', limit: '100' }, { signal }))
  const cases = useLoad((signal) => api.get<{ cases: { hold: boolean }[]; next_after: string | null }>('/v1/staff/aml/cases', { view: 'open', limit: '200' }, { signal }))
  const first = me.name.split(/\s+/)[0] ?? me.name
  const urgent = (alerts ?? []).filter((a) => a.severity === 'page' || a.freezes_payouts).length
  // a list the server did not return (or returned without its rows) counts as nothing known yet, never as a crash
  const caseRows = Array.isArray(cases.data?.cases) ? cases.data.cases : undefined
  const heldRows = Array.isArray(held.data?.payouts) ? held.data.payouts : undefined
  const holds = caseRows?.filter((c) => c.hold).length ?? 0
  return (
    <section>
      <PageHeader title={`Welcome back, ${first}`}>
        What needs a person today, at a glance. Every action you take is recorded in the audit log under your name.
      </PageHeader>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="mr-1 text-[14px] text-muted-foreground">Your roles:</span>
        {me.roles.map((r) => <Badge key={r} variant="secondary" className="rounded-full px-3 py-0.5 text-[13px]">{roleLabels[r]}</Badge>)}
      </div>

      <h2 className="sr-only">Waiting for a person</h2>
      <ul className="mt-6 grid list-none gap-4 p-0 sm:grid-cols-2 xl:grid-cols-4">
        <li>
          <StatCard hero title="Payouts held" to="/payouts" value={countOf(heldRows?.length, held.data?.next_after != null)}
            note="Large or unpayable payouts waiting for two approvals" decoration={<Coin3D size={88} />} />
        </li>
        <li>
          <StatCard title="Identity reviews" to="/kyc" value={queue ? String(queue.waiting) : '…'}
            note={queue?.oldest_submitted_at ? `Oldest since ${eat(queue.oldest_submitted_at)}` : 'Submissions to compare with the ID'} />
        </li>
        <li>
          <StatCard title="Open alerts" to="/alerts" value={alerts ? String(alerts.length) : '…'}
            note={urgent > 0 ? `${String(urgent)} urgent: payouts may be frozen` : 'What the platform’s checks found wrong'} />
        </li>
        <li>
          <StatCard title="Compliance cases" to="/compliance" value={countOf(caseRows?.length, cases.data?.next_after != null)}
            note={holds > 0 ? `${String(holds)} holding a player’s money` : 'Raised by monitoring or by staff'} />
        </li>
      </ul>

      <SectionTitle>Sections</SectionTitle>
      <p className="-mt-1 mb-4 text-[14px] text-muted-foreground">Where each kind of work is done. Only the sections your roles open are shown.</p>
      <ul className="grid list-none gap-4 p-0 sm:grid-cols-2 xl:grid-cols-3">
        {sections.map((s, i) => {
          const Icon = icons[s.path] ?? Landmark
          return (
            <li key={s.path} className="animate-rise-in" style={{ animationDelay: `${String(i * 30)}ms` }}>
              <Tilt className="h-full" max={4}>
                <Link to={s.path} className="lift hover-hero group block h-full rounded-3xl border bg-card p-5 shadow-card outline-none focus-visible:ring-2 focus-visible:ring-ring">
                  <div className="flex items-start gap-3.5">
                    <span aria-hidden="true" data-chip className="flex size-12 shrink-0 items-center justify-center rounded-2xl bg-primary-container text-primary-container-foreground"><Icon className="size-[22px]" /></span>
                    <div className="min-w-0 flex-1">
                      <span className="flex items-center gap-1 text-[15.5px] font-semibold">{s.label}<ArrowRight aria-hidden="true" className="size-4 -translate-x-1 opacity-0 transition-all group-hover:translate-x-0 group-hover:opacity-100" /></span>
                      <p className="m-0 mt-1 text-[14px] leading-relaxed text-muted-foreground">{blurbs[s.path]}</p>
                    </div>
                  </div>
                </Link>
              </Tilt>
            </li>
          )
        })}
      </ul>
      <SectionTitle>Reconciliations</SectionTitle>
      <p className="-mt-1 mb-4 text-[14px] text-muted-foreground">Check the books against M-Pesa and the ledger now, without waiting for the scheduled run.</p>
      <div className="grid gap-4 lg:grid-cols-2">
        <ReconcileCard path="/v1/staff/draws/reconcile" title="Draws reconciliation" description="Entries, seeds and prize pools against the ledger." />
        <ReconcileCard path="/v1/staff/payouts/reconcile" title="Payouts reconciliation" description="Payouts against M-Pesa and the ledger." />
      </div>
    </section>
  )
}
