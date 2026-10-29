import { ArrowRight, Banknote, Bell, ChartSpline, IdCard, Landmark, Receipt, Ticket, UserRound, UserX, Users, Wallet, type LucideIcon } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useSession } from '../context'
import { ReconcileCard } from '../components/Reconcile'
import { PageHeader, SectionTitle } from '../components/ui'
import { eat } from '../format'
import { submissions, useReviewQueue } from '../reviewQueue'
import { roleLabels, visibleSections } from '../roles'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

const icons: Record<string, LucideIcon> = {
  '/finance': Landmark, '/metrics': ChartSpline, '/players': UserRound, '/kyc': IdCard, '/draws': Ticket, '/payouts': Banknote, '/payments': Wallet, '/tax': Receipt, '/alerts': Bell, '/closures': UserX, '/staff': Users,
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
}

export function OverviewPage() {
  const { me } = useSession()
  const { queue } = useReviewQueue()
  const sections = visibleSections(me.roles)
  return (
    <section>
      <PageHeader title={`Signed in as ${me.name}`}>
        Every action you take is recorded in the audit log under your name.
      </PageHeader>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="mr-1 text-sm text-muted-foreground">Your roles:</span>
        {me.roles.map((r) => <Badge key={r} variant="secondary" className="rounded-full px-2.5">{roleLabels[r]}</Badge>)}
      </div>
      {queue && queue.waiting > 0 ? (
        <Card className="mt-6 border-pending/40 bg-pending/5 shadow-card">
          <CardContent className="flex flex-wrap items-center gap-3">
            <IdCard aria-hidden="true" className="size-5 text-pending" />
            <Link to="/kyc" className="font-medium underline-offset-4 hover:underline">{submissions(queue.waiting)}</Link>
            {queue.oldest_submitted_at ? <span className="text-sm text-muted-foreground">· oldest since {eat(queue.oldest_submitted_at)}</span> : null}
          </CardContent>
        </Card>
      ) : null}
      <SectionTitle>Sections</SectionTitle>
      <ul className="grid list-none gap-3 p-0 sm:grid-cols-2 xl:grid-cols-3">
        {sections.map((s) => {
          const Icon = icons[s.path] ?? Landmark
          return (
            <li key={s.path}>
              <Link to={s.path} className="group block h-full rounded-2xl border bg-card p-5 shadow-card outline-none transition-[transform,box-shadow] hover:-translate-y-0.5 hover:shadow-float focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none">
                <div className="flex items-start gap-3">
                  <span aria-hidden="true" className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary-container text-primary-container-foreground"><Icon className="size-5" /></span>
                  <div className="min-w-0 flex-1">
                    <span className="flex items-center gap-1 font-semibold">{s.label}<ArrowRight aria-hidden="true" className="size-4 opacity-0 transition-opacity group-hover:opacity-100" /></span>
                    <p className="mt-0.5 text-sm text-muted-foreground">{blurbs[s.path]}</p>
                  </div>
                </div>
              </Link>
            </li>
          )
        })}
      </ul>
      <SectionTitle>Reconciliations</SectionTitle>
      <div className="grid gap-4 lg:grid-cols-2">
        <ReconcileCard path="/v1/staff/draws/reconcile" title="Draws reconciliation" description="Entries, seeds and prize pools against the ledger." />
        <ReconcileCard path="/v1/staff/payouts/reconcile" title="Payouts reconciliation" description="Payouts against M-Pesa and the ledger." />
      </div>
    </section>
  )
}
