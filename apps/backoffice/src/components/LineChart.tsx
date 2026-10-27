import { useId, useMemo, useState } from 'react'

export interface Series { key: string; label: string; color: string; values: number[] }

const W = 720
const H = 260
const PAD = { top: 16, right: 112, bottom: 28, left: 64 }

/** A round step for about four gridlines. */
function niceStep(max: number): number {
  if (max <= 0) return 1
  const raw = max / 4
  const p = 10 ** Math.floor(Math.log10(raw))
  const m = raw / p
  return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p
}

const compact = (n: number) => new Intl.NumberFormat('en-KE', { notation: 'compact', maximumFractionDigits: 1 }).format(n)

/**
 * Several money series over the same days, on one axis (KES). Thin lines, a legend and labels at the line ends (the
 * colour is never the only key), a crosshair with every value of the hovered day, and a table view beside it.
 */
export function LineChart({ title, days, series, format, dayLabel }: {
  title: string
  days: string[]
  series: Series[]
  format: (n: number) => string
  dayLabel: (d: string) => string
}) {
  const id = useId()
  const [hover, setHover] = useState<number | null>(null)
  const max = Math.max(0, ...series.flatMap((s) => s.values))
  const step = niceStep(max)
  const top = Math.max(step, Math.ceil(max / step) * step)
  const iw = W - PAD.left - PAD.right
  const ih = H - PAD.top - PAD.bottom
  const x = (i: number) => PAD.left + (days.length <= 1 ? iw / 2 : (i / (days.length - 1)) * iw)
  const y = (v: number) => PAD.top + ih - (Math.max(0, v) / top) * ih
  const ticks = Array.from({ length: Math.round(top / step) + 1 }, (_, i) => i * step)

  // line-end labels, nudged apart so they never overlap
  const ends = useMemo(() => {
    const last = days.length - 1
    const placed = series.map((s) => ({ s, y: y(s.values[last] ?? 0) })).sort((a, b) => a.y - b.y)
    placed.forEach((cur, i) => {
      const prev = placed[i - 1]
      if (prev && cur.y - prev.y < 14) cur.y = prev.y + 14
    })
    return placed
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [series, days.length, top])

  const pick = (clientX: number, rect: DOMRect) => {
    const px = ((clientX - rect.left) / rect.width) * W
    const i = days.length <= 1 ? 0 : Math.round(((px - PAD.left) / iw) * (days.length - 1))
    setHover(Math.min(days.length - 1, Math.max(0, i)))
  }

  return (
    <figure className="m-0 mt-6 rounded-xl border bg-card p-4 shadow-card md:p-5" aria-labelledby={`${id}-t`}>
      <figcaption id={`${id}-t`} className="text-base font-semibold">{title}</figcaption>
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 p-0 text-sm text-muted-foreground" aria-label="Legend">
        {series.map((s) => (
          <li key={s.key} className="flex items-center gap-1.5"><span className="inline-block size-2.5 rounded-sm" style={{ background: s.color }} aria-hidden="true" />{s.label}</li>
        ))}
      </ul>
      <div className="relative mt-3">
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${title}. Values are in the table below.`} className="block h-auto w-full"
          onMouseMove={(e) => { pick(e.clientX, e.currentTarget.getBoundingClientRect()) }}
          onMouseLeave={() => { setHover(null) }}>
          {ticks.map((t) => (
            <g key={t}>
              <line className="stroke-border" x1={PAD.left} x2={W - PAD.right} y1={y(t)} y2={y(t)} />
              <text className="fill-muted-foreground text-[11px]" x={PAD.left - 8} y={y(t)} dy="0.32em" textAnchor="end">{compact(t)}</text>
            </g>
          ))}
          {days.length > 0 ? (
            [0, Math.floor((days.length - 1) / 2), days.length - 1].filter((v, i, a) => a.indexOf(v) === i).map((i) => (
              <text key={i} className="fill-muted-foreground text-[11px]" x={x(i)} y={H - 8} textAnchor={i === 0 ? 'start' : i === days.length - 1 ? 'end' : 'middle'}>{dayLabel(days[i] ?? '')}</text>
            ))
          ) : null}
          {series.map((s) => (
            <polyline key={s.key} className="fill-none stroke-[2] [stroke-linejoin:round] [stroke-linecap:round]" stroke={s.color}
              points={s.values.map((v, i) => `${x(i)},${y(v)}`).join(' ')} />
          ))}
          {ends.map(({ s, y: ly }) => (
            <text key={s.key} className="fill-foreground text-[11px] font-medium" x={W - PAD.right + 8} y={ly} dy="0.32em">{s.label}</text>
          ))}
          {hover !== null ? (
            <g>
              <line className="stroke-foreground/40 [stroke-dasharray:3_3]" x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + ih} />
              {series.map((s) => (
                <circle key={s.key} className="stroke-card stroke-[2]" cx={x(hover)} cy={y(s.values[hover] ?? 0)} r={4} fill={s.color} />
              ))}
            </g>
          ) : null}
        </svg>
        {hover !== null ? (
          <div aria-hidden="true" className="pointer-events-none absolute top-2 z-10 grid min-w-44 -translate-x-1/2 gap-0.5 rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-float" style={{ left: `${(x(hover) / W) * 100}%` }}>
            <strong className="mb-0.5 text-[13px]">{dayLabel(days[hover] ?? '')}</strong>
            {series.map((s) => (
              <span key={s.key} className="flex items-center gap-1.5"><span className="inline-block size-2 rounded-sm" style={{ background: s.color }} />{s.label}: {format(s.values[hover] ?? 0)}</span>
            ))}
          </div>
        ) : null}
      </div>
    </figure>
  )
}
