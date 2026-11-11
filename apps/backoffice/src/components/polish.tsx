import { ArrowUpRight, Check, Copy } from 'lucide-react'
import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { BrandGlyph } from './BrandGlyph'
import { cn } from '@/lib/utils'

/** Whether the person asked for less motion; read once per render, updated when the setting changes. */
function useReducedMotion(): boolean {
  const query = typeof window !== 'undefined' && typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-reduced-motion: reduce)') : null
  const [reduced, setReduced] = useState(query?.matches ?? false)
  useEffect(() => {
    if (!query) return
    const on = () => { setReduced(query.matches) }
    query.addEventListener('change', on)
    return () => { query.removeEventListener('change', on) }
  }, [query])
  return reduced
}

/**
 * The 4draws coin, turning in 3D (CSS only: perspective, preserve-3d and a stack of thin layers for its edge).
 * Decorative: hidden from screen readers. Still when the person asks for reduced motion.
 */
export function Coin3D({ size = 120, className }: { size?: number; className?: string }) {
  const reduced = useReducedMotion()
  const edge = 12 // layers that make the coin's thickness
  const face: CSSProperties = {
    position: 'absolute', inset: 0, borderRadius: '9999px', backfaceVisibility: 'hidden',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    background: 'radial-gradient(circle at 32% 28%, #ffe7a6 0%, #f4b544 38%, #c9811a 72%, #8a5208 100%)',
    boxShadow: 'inset 0 0 0 4px rgb(255 240 200 / 0.55), inset 0 0 18px rgb(120 60 0 / 0.45)',
  }
  return (
    <div aria-hidden="true" className={cn('pointer-events-none select-none', className)} style={{ width: size, height: size, perspective: size * 6 }}>
      <div style={{ width: '100%', height: '100%', animation: reduced ? undefined : 'coin-float 4s ease-in-out infinite' }}>
        <div
          style={{
            position: 'relative', width: '100%', height: '100%', transformStyle: 'preserve-3d',
            transform: reduced ? 'rotateY(-24deg) rotateX(12deg)' : undefined,
            animation: reduced ? undefined : 'coin-turn 7s linear infinite',
          }}
        >
          {Array.from({ length: edge }, (_, i) => (
            <div key={i} style={{ position: 'absolute', inset: 0, borderRadius: '9999px', background: i % 2 ? '#a8650d' : '#b8741a', transform: `translateZ(${String(i - edge / 2)}px)` }} />
          ))}
          <div style={{ ...face, transform: `translateZ(${String(edge / 2)}px)` }}>
            <span style={{ width: size * 0.5, height: size * 0.5, color: '#7a3d00' }}><BrandGlyph className="size-full" /></span>
          </div>
          <div style={{ ...face, transform: `rotateY(180deg) translateZ(${String(edge / 2)}px)` }}>
            <span style={{ fontFamily: 'var(--font-display)', fontWeight: 800, fontSize: size * 0.22, color: '#7a3d00', letterSpacing: '-0.02em' }}>KES 1</span>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * A wrapper for a tappable card (it was a 3D tilt until the owner asked for less 3D, 2026-11-11): it only passes its
 * class on. The card inside carries `lift`, which raises it and deepens its shadow under the pointer.
 */
export function Tilt({ children, className }: { children: ReactNode; className?: string; max?: number }) {
  return <div className={className}>{children}</div>
}

/**
 * An M-Pesa receipt (or another short reference) set to be read aloud or typed: monospace, spaced, on its own chip,
 * with a button that copies it. The text stays selectable.
 */
export function ReceiptCode({ value, className }: { value: string; className?: string }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => { setCopied(false) }, 1500)
    return () => { clearTimeout(t) }
  }, [copied])
  const canCopy = typeof navigator !== 'undefined' && 'clipboard' in navigator
  return (
    <span className={cn('inline-flex max-w-full items-center gap-1 rounded-lg border bg-muted/70 py-0.5 pl-2.5 pr-1 align-middle', className)}>
      <span className="font-mono text-[14px] font-semibold tracking-[0.12em] text-foreground [overflow-wrap:anywhere]">{value}</span>
      {canCopy ? (
        <button
          type="button"
          className="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
          aria-label={copied ? `Copied ${value}` : `Copy ${value}`}
          onClick={() => { void navigator.clipboard.writeText(value).then(() => { setCopied(true) }) }}
        >
          {copied ? <Check aria-hidden="true" className="size-4 text-success" /> : <Copy aria-hidden="true" className="size-3.5" />}
        </button>
      ) : null}
    </span>
  )
}

/** A label and its value on one line of a receipt, the value right-aligned, a dotted line between. */
export function ReceiptRow({ label, children, strong = false }: { label: string; children: ReactNode; strong?: boolean }) {
  return (
    <div className="flex items-baseline gap-3 py-1.5">
      <dt className="shrink-0 text-[14px] text-muted-foreground">{label}</dt>
      <span aria-hidden="true" className="min-w-4 flex-1 translate-y-[-3px] border-b border-dotted border-border" />
      <dd className={cn('m-0 min-w-0 text-right text-[15px] [overflow-wrap:anywhere]', strong ? 'font-bold text-foreground' : 'font-medium')}>{children}</dd>
    </div>
  )
}

/**
 * A panel laid out like a printed receipt: a title and state, the amount large, then labelled rows. The bottom edge
 * is torn, as a till roll's.
 */
export function ReceiptCard({ title, status, amount, amountLabel, children, className }: {
  title: ReactNode
  status?: ReactNode
  amount?: ReactNode
  amountLabel?: string
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('relative rounded-t-2xl bg-card px-5 pb-7 pt-5 shadow-card animate-rise-in', className)}
      style={{ maskImage: 'linear-gradient(to bottom, #000 calc(100% - 10px), transparent calc(100% - 10px)), radial-gradient(circle at 8px 100%, transparent 6px, #000 6.5px) bottom left / 16px 10px repeat-x', WebkitMaskImage: 'linear-gradient(to bottom, #000 calc(100% - 10px), transparent calc(100% - 10px)), radial-gradient(circle at 8px 100%, transparent 6px, #000 6.5px) bottom left / 16px 10px repeat-x' }}>
      <header className="flex flex-wrap items-start justify-between gap-2">
        <h3 className="m-0 font-display text-lg font-bold tracking-tight">{title}</h3>
        {status}
      </header>
      {amount !== undefined ? (
        <div className="mt-3 border-y border-dashed py-3">
          {amountLabel ? <p className="m-0 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{amountLabel}</p> : null}
          <p className="m-0 font-display text-3xl font-extrabold tracking-tight tabular-nums">{amount}</p>
        </div>
      ) : null}
      <dl className="m-0 mt-2">{children}</dl>
    </section>
  )
}

/**
 * A figure on the overview: what it counts, the number large, a line of context, and a round arrow to the page that
 * acts on it. `hero` is the highlighted card, on the brand gradient.
 */
export function StatCard({ title, value, note, to, hero = false, decoration }: {
  title: string
  value: ReactNode
  note?: ReactNode
  to: string
  hero?: boolean
  decoration?: ReactNode
}) {
  return (
    <Tilt className="h-full">
      <Link
        to={to}
        className={cn(
          'lift group relative flex h-full min-h-40 flex-col overflow-hidden rounded-3xl p-5 outline-none focus-visible:ring-2 focus-visible:ring-ring',
          hero ? 'bg-hero text-white shadow-float' : 'hover-hero border bg-card shadow-card',
        )}
      >
        <span className="flex items-start justify-between gap-3">
          <span className={cn('text-[15px] font-semibold', hero ? 'text-white' : 'text-foreground')}>{title}</span>
          <span aria-hidden="true" data-chip className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-full border transition-transform group-hover:rotate-45',
            hero ? 'border-white/40 bg-white text-primary' : 'border-border bg-background text-foreground',
          )}>
            <ArrowUpRight className="size-4" />
          </span>
        </span>
        <span className={cn('mt-5 font-display text-5xl font-extrabold leading-none tracking-tight tabular-nums', hero ? 'text-white' : 'text-foreground')}>{value}</span>
        {note ? <span className={cn('mt-auto pt-4 text-[13px] leading-snug', hero ? 'max-w-[70%] text-white/90' : 'text-muted-foreground')}>{note}</span> : null}
        {decoration ? <span className="pointer-events-none absolute -bottom-5 -right-3 opacity-95">{decoration}</span> : null}
      </Link>
    </Tilt>
  )
}

/** A file an export saved: what the server said about it (the hash it recorded, the rows it holds). */
export interface SavedFile {
  name: string
  sha256: string | null
  rows: number | null
  at: Date
  /** where the export itself is recorded, or another line for the reader */
  note?: string
}

/**
 * The receipt of an export: which file was saved, how many rows it holds, when, and the SHA-256 the server recorded,
 * to check the file against (sha256sum on the saved file must print the same). Announced to screen readers.
 */
export function ExportReceipt({ file, className }: { file: SavedFile; className?: string }) {
  return (
    <div role="status" className={cn('my-4', className)}>
      <ReceiptCard
        title={<span className="flex items-center gap-2"><Check aria-hidden="true" className="size-5 text-success" /> Saved {file.name}</span>}
      >
        <ReceiptRow label="File">{file.name}</ReceiptRow>
        {file.rows !== null ? <ReceiptRow label="Rows">{file.rows.toLocaleString('en-KE')}</ReceiptRow> : null}
        <ReceiptRow label="Saved">{file.at.toLocaleString('en-KE', { timeZone: 'Africa/Nairobi', dateStyle: 'medium', timeStyle: 'short' })} EAT</ReceiptRow>
        <div className="grid gap-1.5 pt-2">
          <dt className="text-[14px] text-muted-foreground">SHA-256 (check the saved file against it)</dt>
          <dd className="m-0">{file.sha256 ? <ReceiptCode value={file.sha256} className="w-full justify-between" /> : <span className="text-[14px] text-muted-foreground">The server gave no hash for this file.</span>}</dd>
        </div>
        {file.note ? <p className="m-0 mt-3 text-[14px] text-muted-foreground">{file.note}</p> : null}
      </ReceiptCard>
    </div>
  )
}
