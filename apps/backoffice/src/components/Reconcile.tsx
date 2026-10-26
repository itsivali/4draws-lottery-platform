import { useState } from 'react'
import { useSession } from '../context'
import { useAction } from '../hooks'
import type { Reconciliation } from '../types'
import { ActionResult, Pill } from './ui'

/** Runs one reconciliation now and shows its findings (the run is stored by the server). */
export function ReconcileCard({ path, title }: { path: string; title: string }) {
  const { api } = useSession()
  const [report, setReport] = useState<Reconciliation | null>(null)
  const { state, run } = useAction()
  const issues = (m: Record<string, string[] | undefined> | null | undefined) => Object.entries(m ?? {}).filter(([, v]) => (v?.length ?? 0) > 0)
  return (
    <div className="card">
      <div className="card-head">
        <h2>{title}</h2>
        <button type="button" className="secondary" disabled={state.kind === 'busy'} onClick={() => void run(async () => { setReport(await api.post<Reconciliation>(path)) }, 'Reconciliation finished.')}>
          {state.kind === 'busy' ? 'Running…' : 'Run now'}
        </button>
      </div>
      <ActionResult state={state} />
      {report ? (
        <div>
          <Pill tone={report.ok ? 'success' : 'error'}>{report.ok ? 'Everything matches' : 'Mismatches found'}</Pill>
          {issues(report.mismatches).map(([k, v]) => (
            <details key={k} open><summary>{k} ({v?.length})</summary><ul>{v?.map((x) => <li key={x} className="mono">{x}</li>)}</ul></details>
          ))}
          {issues(report.attention).map(([k, v]) => (
            <details key={k}><summary>Needs attention: {k} ({v?.length})</summary><ul>{v?.map((x) => <li key={x} className="mono">{x}</li>)}</ul></details>
          ))}
        </div>
      ) : null}
    </div>
  )
}
