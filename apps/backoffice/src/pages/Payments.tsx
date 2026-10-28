import { useSession } from '../context'
import { ReconcileCard } from '../components/Reconcile'
import { useConfirmedAction, type ActionSpec } from '../components/useConfirmedAction'
import { PageHeader, SectionTitle } from '../components/ui'
import { can } from '../roles'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export function PaymentsPage() {
  const { api, me } = useSession()
  const { ask, busy, view } = useConfirmedAction(() => undefined)
  const operator = can(me.roles, 'staff')
  const actions: (ActionSpec & { button: string; help: string })[] = [
    {
      button: 'Record a reversal', help: 'M-Pesa took back a credited top-up (statement or Safaricom report).',
      title: 'Record a reversal', body: <p>The player gives back what they received: from the wallet as far as it goes, the rest as debt that pauses play and withdrawals.</p>,
      fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }, { name: 'reason', label: 'Reason', required: true }],
      confirmLabel: 'Record reversal', danger: true, success: 'Reversal recorded.',
      act: (v) => api.post('/v1/staff/payments/reversals', { receipt: v.receipt, reason: v.reason }),
    },
    {
      button: 'Resolve an unconfirmed top-up', help: 'Daraja never confirmed it, but the statement shows it was paid.',
      title: 'Resolve an unconfirmed top-up', body: <p>Credits the player with the amount on the statement, once.</p>,
      fields: [{ name: 'payment', label: 'Payment id', required: true }, { name: 'receipt', label: 'M-Pesa receipt', required: true }, { name: 'amount', label: 'Amount paid (KES)', type: 'number', required: true }],
      confirmLabel: 'Credit', success: 'Top-up resolved and credited.',
      act: (v) => api.post(`/v1/staff/payments/${encodeURIComponent(v.payment ?? '')}/resolve`, { receipt: v.receipt, amount_kes: Number(v.amount) }),
    },
    {
      button: 'Credit a suspense payment', help: 'A paybill payment with an unknown account number, once you know whose it is.',
      title: 'Credit a payment held in suspense', body: <p>Credits it to the player through the normal credit path (excise, debt recovery).</p>,
      fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }, { name: 'user', label: 'Player id', required: true }],
      confirmLabel: 'Credit', success: 'Suspense payment credited.',
      act: (v) => api.post(`/v1/staff/suspense/${encodeURIComponent(v.receipt ?? '')}/credit`, { user_id: v.user }),
    },
    {
      button: 'Record a suspense refund', help: 'The payment was refunded to the payer outside the platform.',
      title: 'Record the refund of a suspense payment', body: <p>Closes the suspense item.</p>,
      fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }, { name: 'ref', label: 'Refund reference', required: true }],
      confirmLabel: 'Record refund', success: 'Refund recorded.',
      act: (v) => api.post(`/v1/staff/suspense/${encodeURIComponent(v.receipt ?? '')}/refund`, { refund_ref: v.ref }),
    },
    {
      button: 'Check a paybill payment again', help: 'Ask Daraja again about an unverified paybill payment.',
      title: 'Check a paybill payment again', body: <p>A new Transaction Status request is sent.</p>,
      fields: [{ name: 'receipt', label: 'M-Pesa receipt', required: true }],
      confirmLabel: 'Check again', success: 'Requested.',
      act: (v) => api.post(`/v1/staff/paybill/${encodeURIComponent(v.receipt ?? '')}/recheck`),
    },
  ]
  return (
    <section>
      <PageHeader title="Payments">Money in: reconciliation, reversals, unconfirmed top-ups and the paybill suspense queue (listed in the reconciliation’s “needs attention”).</PageHeader>
      <div className="grid gap-4 lg:grid-cols-2">
        <ReconcileCard path="/v1/staff/payments/reconcile" title="Payments reconciliation" description="Top-ups and paybill payments against M-Pesa and the ledger." />
        <ReconcileCard path="/v1/staff/game/reconcile" title="Game reconciliation" description="Plays, stakes and chances against the ledger." />
      </div>
      {operator ? (
        <>
          <SectionTitle>Actions</SectionTitle>
          {view}
          <Card className="shadow-card">
            <CardHeader>
              <CardTitle className="text-lg">Corrections from the M-Pesa statement</CardTitle>
              <CardDescription>Each one asks for the receipt and a reason, and is recorded in the audit log.</CardDescription>
            </CardHeader>
            <CardContent>
              <ul className="m-0 grid list-none gap-3 p-0">
                {actions.map((a) => (
                  <li key={a.button} className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t pt-3 first:border-t-0 first:pt-0">
                    <Button type="button" variant="outline" className="min-w-64 justify-start" disabled={busy} onClick={() => { ask(a) }}>{a.button}</Button>
                    <span className="text-sm text-muted-foreground">{a.help}</span>
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        </>
      ) : null}
    </section>
  )
}
