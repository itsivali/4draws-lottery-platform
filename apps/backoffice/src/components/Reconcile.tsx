import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useSession } from '../context'
import { useAction } from '../hooks'
import type { Reconciliation } from '../types'
import { ActionResult, Mono, Pill } from './ui'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

/** attention lists whose items are top-up ids: each opens the top-up */
const topUpKeys = new Set(['unconfirmed', 'overdue_pending', 'receipt_claims'])

/** Runs one reconciliation now and shows its findings (the run is stored by the server). */
export function ReconcileCard({ path, title, description }: { path: string; title: string; description?: string }) {
  const { api } = useSession()
  const [report, setReport] = useState<Reconciliation | null>(null)
  const { state, run } = useAction()
  const issues = (m: Record<string, string[] | undefined> | null | undefined) => Object.entries(m ?? {}).filter(([, v]) => (v?.length ?? 0) > 0)
  const busy = state.kind === 'busy'
  return (
    <Card className="shadow-card">
      <CardHeader>
        <CardTitle className="text-lg">{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
        <CardAction>
          <Button type="button" variant="outline" disabled={busy} onClick={() => void run(async () => { setReport(await api.post<Reconciliation>(path)) }, 'Reconciliation finished.')}>
            <RefreshCw aria-hidden="true" className={busy ? 'animate-spin' : ''} />
            {busy ? 'Running…' : 'Run now'}
          </Button>
        </CardAction>
      </CardHeader>
      {state.kind !== 'idle' || report ? (
        <CardContent className="grid gap-3">
          <ActionResult state={state} onCheckAgain={(retry) => void run(retry, 'Confirmed by the server.')} />
          {report ? (
            <div className="grid gap-2">
              <div><Pill tone={report.ok ? 'success' : 'error'}>{report.ok ? 'Everything matches' : 'Mismatches found'}</Pill></div>
              {issues(report.mismatches).map(([k, v]) => (
                <details key={k} open className="rounded-lg border bg-muted/30 px-3 py-2 text-sm">
                  <summary className="cursor-pointer font-medium">{k} ({v?.length})</summary>
                  <ul className="mt-2 grid gap-1 pl-4">{v?.map((x) => <li key={x}><Mono>{x}</Mono></li>)}</ul>
                </details>
              ))}
              {issues(report.attention).map(([k, v]) => (
                <details key={k} className="rounded-lg border bg-muted/30 px-3 py-2 text-sm">
                  <summary className="cursor-pointer font-medium">Needs attention: {k} ({v?.length})</summary>
                  <ul className="mt-2 grid gap-1 pl-4">
                    {v?.map((x) => (
                      <li key={x}>
                        {topUpKeys.has(k) ? <Link to={`/topups?topup=${encodeURIComponent(x)}`} className="underline-offset-2 hover:underline"><Mono>{x}</Mono></Link> : <Mono>{x}</Mono>}
                      </li>
                    ))}
                  </ul>
                </details>
              ))}
            </div>
          ) : null}
        </CardContent>
      ) : null}
    </Card>
  )
}
