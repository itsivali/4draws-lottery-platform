import { FileUp } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useSession } from '../context'
import { eat, kes, msisdn } from '../format'
import { useLoad } from '../hooks'
import type { StatementApplied, StatementCredit, StatementImport, StatementMatch, StatementReport } from '../types'
import { Loaded, Mono, Pill, type Tone } from './ui'
import { useConfirmedAction } from './useConfirmedAction'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ReceiptCode } from './polish'

const kindLabel: Record<StatementMatch['kind'], [Tone, string]> = {
  credited: ['success', 'Already credited'],
  claimed: ['pending', 'Player’s receipt'],
  matched: ['success', 'Matched'],
  ambiguous: ['warning', 'Several fit'],
  unmatched: ['error', 'No prompt fits'],
  skipped: ['neutral', 'Not a payment in'],
}

/**
 * The M-Pesa statement (ADR 0024): upload Safaricom's CSV, see what each paid row matches, confirm what to credit.
 * Nothing is credited by the upload; every confirmed credit stands alone and the import is kept with its report.
 */
export function StatementImportCard() {
  const { api } = useSession()
  const file = useRef<HTMLInputElement>(null)
  const [filename, setFilename] = useState('')
  const [report, setReport] = useState<StatementReport | null>(null)
  const [reading, setReading] = useState(false)
  const [readError, setReadError] = useState<string | null>(null)
  const [chosen, setChosen] = useState<Record<string, StatementCredit | null>>({}) // by receipt; null: left out
  const [applied, setApplied] = useState<StatementApplied | null>(null)
  const imports = useLoad((signal) => api.get<{ imports: StatementImport[] }>('/v1/staff/payments/statements', { limit: '20' }, { signal }).then((r) => r.imports), [applied?.import_id])
  const { ask, busy, view } = useConfirmedAction(() => undefined)

  const credits = useMemo(() => Object.values(chosen).filter((c): c is StatementCredit => c !== null), [chosen])
  const total = credits.reduce((n, c) => n + c.amount_kes, 0)

  async function match(f: File) {
    setReading(true)
    setReadError(null)
    setApplied(null)
    try {
      const text = await readText(f)
      const r = await api.postText<{ filename: string; report: StatementReport }>('/v1/staff/payments/statement/match', text)
      setFilename(f.name)
      setReport(r.report)
      const next: Record<string, StatementCredit | null> = {}
      for (const c of r.report.to_credit) next[c.receipt] = c
      setChosen(next)
    } catch (e) {
      setReadError(e instanceof Error ? e.message : 'The statement could not be read.')
      setReport(null)
    } finally {
      setReading(false)
    }
  }

  function pick(m: StatementMatch, paymentId: string | null) {
    setChosen((c) => ({ ...c, [m.row.receipt]: paymentId ? { payment_id: paymentId, receipt: m.row.receipt, amount_kes: m.row.paid_in_kes } : null }))
  }

  return (
    <Card className="shadow-card">
      <CardHeader>
        <CardTitle className="text-lg">Import an M-Pesa statement</CardTitle>
        <CardDescription>
          Safaricom’s CSV export (organisation portal → Completed transactions). Each paid row is matched to a top-up by receipt, or by
          amount, number and time. Nothing is credited until you confirm the list.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {view}
        <div className="flex flex-wrap items-center gap-3">
          <input ref={file} type="file" accept=".csv,text/csv" className="sr-only" aria-label="Statement CSV" onChange={(e) => { const f = e.target.files?.[0]; if (f) void match(f); e.target.value = '' }} />
          <Button type="button" variant="outline" disabled={reading} onClick={() => file.current?.click()}><FileUp aria-hidden="true" />{reading ? 'Reading…' : 'Choose statement CSV'}</Button>
          {filename ? <span className="text-sm text-muted-foreground">{filename}</span> : null}
        </div>
        {readError ? <p role="alert" className="text-sm text-destructive">{readError}</p> : null}
        {report ? (
          <>
            <div className="flex flex-wrap gap-2 text-sm">
              <span>{report.rows} rows · {eat(report.from)} to {eat(report.to)}</span>
              {Object.entries(report.counts).map(([k, n]) => {
                const known = (kindLabel as Partial<Record<string, [Tone, string]>>)[k]
                return <Pill key={k} tone={known?.[0] ?? 'neutral'}>{n} {known?.[1].toLowerCase() ?? k}</Pill>
              })}
            </div>
            {report.skipped?.length ? (
              <details className="rounded-lg border bg-muted/30 px-3 py-2 text-sm">
                <summary className="cursor-pointer font-medium">{report.skipped.length} rows the platform could not read</summary>
                <ul className="mt-2 grid gap-1 pl-4">{report.skipped.map((s) => <li key={s}>{s}</li>)}</ul>
              </details>
            ) : null}
            <div className="overflow-x-auto rounded-xl border bg-card shadow-card">
              <Table>
                <TableCaption className="sr-only">Statement rows and what they match</TableCaption>
                <TableHeader className="bg-muted/50">
                  <TableRow>
                    <TableHead scope="col"><span className="sr-only">Credit</span></TableHead>
                    <TableHead scope="col">Receipt</TableHead>
                    <TableHead scope="col">Completed</TableHead>
                    <TableHead scope="col" className="text-right">Paid in</TableHead>
                    <TableHead scope="col">Payer</TableHead>
                    <TableHead scope="col">Match</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {report.matches.filter((m) => m.kind !== 'skipped').map((m) => {
                    const [tone, text] = kindLabel[m.kind]
                    const pickable = m.kind === 'matched' || m.kind === 'claimed' || m.kind === 'ambiguous'
                    const current = chosen[m.row.receipt] ?? null
                    return (
                      <TableRow key={m.row.receipt} className="align-top">
                        <TableCell className="py-3">
                          {pickable && m.kind !== 'ambiguous' ? (
                            <Checkbox aria-label={`Credit ${m.row.receipt}`} checked={current !== null} onCheckedChange={(v) => { pick(m, v === true ? (m.payment_id ?? null) : null) }} />
                          ) : null}
                        </TableCell>
                        <TableCell className="py-3"><ReceiptCode value={m.row.receipt} /></TableCell>
                        <TableCell className="py-3 tabular-nums">{eat(m.row.completed_at)}</TableCell>
                        <TableCell className="py-3 text-right tabular-nums">{kes(m.row.paid_in_kes)}</TableCell>
                        <TableCell className="py-3">{m.row.party || <span className="text-muted-foreground">—</span>}</TableCell>
                        <TableCell className="py-3">
                          <div className="grid gap-1">
                            <div><Pill tone={tone}>{text}</Pill></div>
                            {m.payment_id ? (
                              <span className="text-sm">
                                <Link to={`/topups?topup=${encodeURIComponent(m.payment_id)}`} className="underline-offset-2 hover:underline"><Mono>{m.payment_id}</Mono></Link>
                                {m.msisdn ? <> · {msisdn(m.msisdn)}</> : null}{m.status ? <> · {m.status.replaceAll('_', ' ')}</> : null}
                              </span>
                            ) : null}
                            {m.reason ? <span className="text-sm text-muted-foreground">{m.reason}</span> : null}
                            {m.kind === 'ambiguous' && m.candidates ? (
                              <fieldset className="grid gap-1 text-sm">
                                <legend className="sr-only">Which prompt is {m.row.receipt}?</legend>
                                {m.candidates.map((c) => (
                                  <label key={c.payment_id} className="flex items-start gap-2">
                                    <input type="radio" name={`pick-${m.row.receipt}`} checked={current?.payment_id === c.payment_id} onChange={() => { pick(m, c.payment_id) }} />
                                    <span><Mono>{c.payment_id}</Mono> · {msisdn(c.msisdn)} · {eat(c.created_at)} · {c.status.replaceAll('_', ' ')} <span className="text-muted-foreground">({c.why})</span></span>
                                  </label>
                                ))}
                                <label className="flex items-center gap-2"><input type="radio" name={`pick-${m.row.receipt}`} checked={current === null} onChange={() => { pick(m, null) }} /> Leave it</label>
                              </fieldset>
                            ) : null}
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
            {report.missing_from_statement?.length ? (
              <details className="rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm">
                <summary className="cursor-pointer font-medium">{report.missing_from_statement.length} credited top-ups in this period are not on the statement</summary>
                <p className="mt-1 text-muted-foreground">Either the file is partial, or a credit is wrong. Each opens the top-up.</p>
                <ul className="mt-2 grid gap-1 pl-4">
                  {report.missing_from_statement.map((x) => (
                    <li key={x.payment_id}><Link to={`/topups?topup=${encodeURIComponent(x.payment_id)}`} className="underline-offset-2 hover:underline"><Mono>{x.payment_id}</Mono></Link> · <ReceiptCode value={x.receipt} /> · {kes(x.amount_kes)} · {eat(x.created_at)}</li>
                  ))}
                </ul>
              </details>
            ) : null}
            <div className="flex flex-wrap items-center gap-3">
              <Button type="button" disabled={busy || credits.length === 0} onClick={() => {
                const r = report
                ask({
                  title: `Credit ${credits.length} top-up${credits.length === 1 ? '' : 's'}`, confirmLabel: 'Credit', success: 'Statement applied.',
                  body: <p>{kes(total)} in all, each credited once exactly like a confirmed callback, and the import kept with this report. A receipt that was credited meanwhile is refused on its own.</p>,
                  act: async () => {
                    setApplied(await api.post<StatementApplied>('/v1/staff/payments/statement/apply', { filename, sha256: r.sha256, rows: r.rows, credits, report: r }))
                  },
                })
              }}>Credit {credits.length} selected · {kes(total)}</Button>
              <span className="text-sm text-muted-foreground">Rows already credited and rows with no match are left as they are.</span>
            </div>
            {applied ? (
              <div role="status" className="grid gap-2 rounded-lg border bg-muted/30 px-3 py-2 text-sm">
                <strong>Credited {applied.credited} top-up{applied.credited === 1 ? '' : 's'} · {kes(applied.credited_kes)}</strong>
                <ul className="grid gap-1">
                  {applied.results.map((r) => (
                    <li key={r.payment_id} className="flex flex-wrap items-center gap-2">
                      <Pill tone={r.credited ? 'success' : 'error'}>{r.credited ? 'Credited' : 'Refused'}</Pill>
                      <Link to={`/topups?topup=${encodeURIComponent(r.payment_id)}`} className="underline-offset-2 hover:underline"><Mono>{r.payment_id}</Mono></Link>
                      <ReceiptCode value={r.receipt} /> · {kes(r.amount_kes)}{r.error ? <span className="text-muted-foreground"> · {r.error}</span> : null}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
          </>
        ) : null}
        <Loaded load={imports} what="past imports">
          {(rows) => rows.length === 0 ? <p className="text-sm text-muted-foreground">No statement imported yet.</p> : (
            <div className="overflow-x-auto rounded-xl border bg-card">
              <Table>
                <TableCaption className="sr-only">Past statement imports</TableCaption>
                <TableHeader className="bg-muted/50">
                  <TableRow>
                    <TableHead scope="col">Imported</TableHead>
                    <TableHead scope="col">File</TableHead>
                    <TableHead scope="col" className="text-right">Rows</TableHead>
                    <TableHead scope="col" className="text-right">Confirmed</TableHead>
                    <TableHead scope="col" className="text-right">Credited</TableHead>
                    <TableHead scope="col">By</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell className="tabular-nums">{eat(r.imported_at)}</TableCell>
                      <TableCell><span>{r.filename}</span> <Mono className="text-muted-foreground">{r.sha256.slice(0, 12)}</Mono></TableCell>
                      <TableCell className="text-right tabular-nums">{r.rows}</TableCell>
                      <TableCell className="text-right tabular-nums">{r.matched}</TableCell>
                      <TableCell className="text-right tabular-nums">{r.credited} · {kes(r.credited_kes)}</TableCell>
                      <TableCell><Mono>{r.imported_by}</Mono></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </Loaded>
      </CardContent>
    </Card>
  )
}

/** The file as text (FileReader: File.text() is missing in some browsers and in jsdom). */
function readText(f: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => { resolve(typeof r.result === 'string' ? r.result : '') }
    r.onerror = () => { reject(r.error ?? new Error('The file could not be read.')) }
    r.readAsText(f)
  })
}
