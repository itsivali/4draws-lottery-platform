import { useState } from 'react'
import { LineChart } from '../components/LineChart'
import { Loaded, PageHeader } from '../components/ui'
import { useSession } from '../context'
import { kes } from '../format'
import { useLoad } from '../hooks'
import type { FinanceSummary } from '../types'

const ranges = [7, 30, 90] as const

/** "2026-10-26" → "26 Oct". */
const shortDay = (d: string) => new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(`${d}T00:00:00Z`))

function nairobiToday(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Africa/Nairobi' }).format(new Date())
}

function minusDays(d: string, n: number): string {
  const t = new Date(`${d}T00:00:00Z`)
  t.setUTCDate(t.getUTCDate() - n)
  return t.toISOString().slice(0, 10)
}

function Stat({ label, value, note }: { label: string; value: number; note?: string | undefined }) {
  return (
    <div className="stat" role="group" aria-label={label}>
      <span className="stat-label">{label}</span>
      <span className="stat-value">{kes(value)}</span>
      {note ? <span className="muted">{note}</span> : null}
    </div>
  )
}

/** The financial dashboard: every figure is read from the ledger (GET /v1/staff/finance). */
export function FinancePage() {
  const { api } = useSession()
  const [days, setDays] = useState<number>(30)
  const load = useLoad(() => {
    const to = nairobiToday()
    return api.get<FinanceSummary>('/v1/staff/finance', { from: minusDays(to, days - 1), to })
  }, [days])
  return (
    <section>
      <PageHeader title="Finance">
        Money in and out, what the platform earned and what it owes, from the ledger. Days are Nairobi days.
      </PageHeader>
      <div className="tabs" role="group" aria-label="Range">
        {ranges.map((n) => (
          <button key={n} type="button" className={`tab${days === n ? ' active' : ''}`} aria-pressed={days === n} onClick={() => { setDays(n) }}>
            Last {n} days
          </button>
        ))}
      </div>
      <Loaded load={load} what="the figures">
        {(s) => (
          <>
            <h2 className="section-title">{shortDay(s.from)} – {shortDay(s.to)}</h2>
            <div className="stats">
              <Stat label="Deposits" value={s.totals.deposits_kes} note={s.totals.reversals_kes ? `${kes(s.totals.reversals_kes)} reversed` : undefined} />
              <Stat label="Stakes" value={s.totals.stakes_kes} note={`${s.activity.plays.toLocaleString('en-KE')} plays by ${s.activity.players.toLocaleString('en-KE')} players`} />
              <Stat label="Prizes awarded" value={s.totals.prizes_awarded_kes} note={`${kes(s.totals.prizes_paid_kes)} paid out`} />
              <Stat label="Gross gaming revenue" value={s.totals.ggr_kes} note="stakes less prizes awarded" />
              <Stat label="Withdrawals" value={s.totals.withdrawals_kes} />
              <Stat label="Excise (operator pays)" value={s.totals.excise_kes} />
              <Stat label="Tax withheld from prizes" value={s.totals.tax_withheld_kes} note={`${kes(s.totals.tax_remitted_kes)} remitted`} />
              <div className="stat" role="group" aria-label="New players">
                <span className="stat-label">New players</span>
                <span className="stat-value">{s.activity.signups.toLocaleString('en-KE')}</span>
              </div>
            </div>

            <LineChart
              title="Deposits, stakes, prizes paid and withdrawals per day (KES)"
              days={s.days.map((d) => d.day)}
              dayLabel={shortDay}
              format={kes}
              series={[
                { key: 'deposits', label: 'Deposits', color: 'var(--series-1)', values: s.days.map((d) => d.deposits_kes) },
                { key: 'stakes', label: 'Stakes', color: 'var(--series-2)', values: s.days.map((d) => d.stakes_kes) },
                { key: 'prizes', label: 'Prizes paid', color: 'var(--series-3)', values: s.days.map((d) => d.prizes_paid_kes) },
                { key: 'withdrawals', label: 'Withdrawals', color: 'var(--series-4)', values: s.days.map((d) => d.withdrawals_kes) },
              ]}
            />

            <h2 className="section-title">Now</h2>
            <div className="stats">
              <Stat label="Owed to players" value={s.balances.owed_to_players_kes} note="wallet balances" />
              <Stat label="Excise owed to KRA" value={s.balances.excise_owed_kes} />
              <Stat label="Withheld tax owed to KRA" value={s.balances.tax_owed_kes} />
              <Stat label="Prize pools" value={s.balances.prize_pools_kes} note="set aside, not yet paid" />
              <Stat label="Player debt" value={s.balances.player_debt_kes} note="from reversed top-ups" />
              <Stat label="Unmatched paybill" value={s.balances.suspense_kes} note="in suspense" />
            </div>

            <table>
              <caption>Per day</caption>
              <thead>
                <tr>
                  <th scope="col">Day</th><th scope="col" className="num">Deposits</th><th scope="col" className="num">Stakes</th>
                  <th scope="col" className="num">Prizes paid</th><th scope="col" className="num">Withdrawals</th><th scope="col" className="num">GGR</th>
                </tr>
              </thead>
              <tbody>
                {[...s.days].reverse().map((d) => (
                  <tr key={d.day}>
                    <th scope="row">{shortDay(d.day)}</th>
                    <td className="num">{d.deposits_kes.toLocaleString('en-KE')}</td>
                    <td className="num">{d.stakes_kes.toLocaleString('en-KE')}</td>
                    <td className="num">{d.prizes_paid_kes.toLocaleString('en-KE')}</td>
                    <td className="num">{d.withdrawals_kes.toLocaleString('en-KE')}</td>
                    <td className="num">{d.ggr_kes.toLocaleString('en-KE')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </Loaded>
    </section>
  )
}
