import { CircleAlert, CircleCheck, CircleDot, CircleX, Clock, Inbox, TriangleAlert } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import { ProblemError } from '../api'
import type { ActionState, Load } from '../hooks'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'

export type Tone = 'success' | 'warning' | 'error' | 'pending' | 'neutral'

const tones: Record<Tone, { icon: typeof CircleCheck; className: string }> = {
  success: { icon: CircleCheck, className: 'border-success/40 bg-success/10 text-success' },
  warning: { icon: TriangleAlert, className: 'border-warning/40 bg-warning/10 text-warning' },
  error: { icon: CircleX, className: 'border-destructive/40 bg-destructive/10 text-destructive' },
  pending: { icon: Clock, className: 'border-pending/40 bg-pending/10 text-pending' },
  neutral: { icon: CircleDot, className: 'border-border bg-muted text-muted-foreground' },
}

/** A state shown with a symbol, a word and a colour: never colour alone. */
export function Pill({ tone, children, className }: { tone: Tone; children: ReactNode; className?: string }) {
  const Icon = tones[tone].icon
  return (
    <Badge variant="outline" className={cn('gap-1 rounded-full px-2.5 py-0.5 font-medium', tones[tone].className, className)}>
      <Icon aria-hidden="true" className="size-3.5" />
      {children}
    </Badge>
  )
}

export function PageHeader({ title, children, actions }: { title: string; children?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-8 flex flex-wrap items-start justify-between gap-4 animate-in fade-in-0 slide-in-from-bottom-1 duration-300 motion-reduce:animate-none">
      <div className="max-w-[70ch]">
        <h1 className="font-display text-3xl font-extrabold leading-tight tracking-tight md:text-4xl">{title}</h1>
        {children ? <p className="mt-2 text-[15px] text-muted-foreground md:text-base">{children}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  )
}

/** A short monospace reference (ids, receipts, hashes) that wraps anywhere and can be selected. */
export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn('font-mono text-[13px] [overflow-wrap:anywhere]', className)}>{children}</span>
}

export function SectionTitle({ children, className }: { children: ReactNode; className?: string }) {
  return <h2 className={cn('mb-3 mt-10 font-display text-xl font-bold tracking-tight', className)}>{children}</h2>
}

function LoadError({ what, error, reload }: { what: string; error: unknown; reload: () => void }) {
  const msg = error instanceof ProblemError ? error.title : 'The server could not be reached'
  return (
    <Alert variant="destructive" className="my-4">
      <CircleAlert />
      <AlertTitle>Couldn’t load {what}</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-3">
        <span>{msg}</span>
        <Button type="button" size="sm" variant="outline" onClick={reload}>Try again</Button>
      </AlertDescription>
    </Alert>
  )
}

/**
 * Loading, error and data for one read. Data stays up while it reloads (with a progress line); an error on reload is
 * shown above the data it could not refresh, so stale figures are never mistaken for fresh ones.
 */
export function Loaded<T>({ load, what, children, skeleton }: { load: Load<T>; what: string; children: (data: T) => ReactNode; skeleton?: ReactNode }) {
  const error = load.error !== undefined ? <LoadError what={what} error={load.error} reload={load.reload} /> : null
  if (load.data !== undefined) {
    return (
      <div className="relative">
        {load.loading ? <div aria-hidden="true" className="absolute inset-x-0 -top-2 h-0.5 overflow-hidden rounded-full"><div className="h-full w-1/3 animate-[slide_1.1s_ease-in-out_infinite] rounded-full bg-primary" /></div> : null}
        {error}
        {children(load.data)}
      </div>
    )
  }
  if (error) return error
  return (
    <div aria-live="polite">
      <p className="sr-only">Loading {what}…</p>
      {skeleton ?? <TableSkeleton />}
    </div>
  )
}

export function TableSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div aria-hidden="true" className="mt-4 overflow-hidden rounded-xl border bg-card shadow-card">
      <div className="h-10 border-b bg-muted/60" />
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-6 border-b px-4 py-3.5 last:border-b-0">
          <Skeleton className="h-4 w-40" /><Skeleton className="h-4 w-24" /><Skeleton className="h-4 w-32" /><Skeleton className="ml-auto h-8 w-20 rounded-lg" />
        </div>
      ))}
    </div>
  )
}

/** What happened to the last action, in words, announced to screen readers. */
export function ActionResult({ state, onCheckAgain }: { state: ActionState; onCheckAgain?: (retry: () => Promise<unknown>) => void }) {
  switch (state.kind) {
    case 'idle':
    case 'busy':
      return null
    case 'done':
      return (
        <Alert role="status" className="my-4 border-success/40 bg-success/10 text-success [&>svg]:text-success">
          <CircleCheck />
          <AlertTitle>{state.message}</AlertTitle>
        </Alert>
      )
    case 'refused':
      return (
        <Alert variant="destructive" className="my-4">
          <CircleX />
          <AlertTitle>{state.error.title}</AlertTitle>
          <AlertDescription>
            {state.error.detail ? <p>{state.error.detail}</p> : null}
            <p className="text-muted-foreground">Nothing was changed.</p>
          </AlertDescription>
        </Alert>
      )
    case 'uncertain':
      return (
        <Alert className="my-4 border-pending/40 bg-pending/10 text-pending [&>svg]:text-pending">
          <Clock />
          <AlertTitle>No answer from the server.</AlertTitle>
          <AlertDescription className="text-foreground">
            <p>The action may or may not have been done. Checking repeats the same request, so it cannot happen twice.</p>
            {onCheckAgain ? <Button type="button" size="sm" variant="outline" className="mt-1" onClick={() => { onCheckAgain(state.retry) }}>Check again</Button> : null}
          </AlertDescription>
        </Alert>
      )
    case 'failed':
      return (
        <Alert variant="destructive" className="my-4">
          <CircleX />
          <AlertTitle>{state.message}</AlertTitle>
        </Alert>
      )
  }
}

export interface ConfirmField {
  name: string
  label: string
  required?: boolean
  /** number: whole shillings above zero; email: a plausible address; password: 12 characters or more; radio: one of options */
  type?: 'text' | 'number' | 'email' | 'checkbox' | 'password' | 'radio'
  options?: { value: string; label: string; description?: string }[]
  help?: string
}

const emailRE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** The staff password rules the server applies (ADR 0019), checked here first so the form says so before sending. */
export const MIN_PASSWORD = 12

/** How many characters (code points, as the server counts them) a password has. */
export const characters = (s: string): number => Array.from(s).length

/** Why a field's value cannot be sent, or null. Empty values are the `required` check's business. */
export function fieldProblem(f: ConfirmField, value: string): string | null {
  const v = f.type === 'password' ? value : value.trim()
  if (!v) return null
  if (f.type === 'number') return /^\d+$/.test(v) && Number(v) > 0 ? null : 'Enter a whole number of shillings above zero.'
  if (f.type === 'email') return emailRE.test(v) ? null : 'Enter an email address.'
  if (f.type === 'password') return characters(v) >= MIN_PASSWORD ? null : `Use at least ${MIN_PASSWORD} characters (a short sentence is fine).`
  return null
}

/** Asks before an action: what it does, and the reason or reference the audit trail needs. */
export function ConfirmDialog({
  open,
  title,
  body,
  fields = [],
  validate,
  confirmLabel,
  danger = false,
  initial,
  onConfirm,
  onCancel,
}: {
  open: boolean
  title: string
  body: ReactNode
  fields?: ConfirmField[]
  /** a check across fields; its message is shown and blocks the confirm button */
  validate?: (values: Record<string, string>) => string | null
  /** values the fields start with (checkboxes: "true") */
  initial?: Record<string, string> | undefined
  confirmLabel: string
  danger?: boolean
  onConfirm: (values: Record<string, string>) => void
  onCancel: () => void
}) {
  const id = useId()
  // the caller remounts the dialog (key) for each question, so initial values apply afresh
  const [values, setValues] = useState<Record<string, string>>(initial ?? {})
  const cancel = () => { setValues({}); onCancel() }
  const missing = fields.some((f) => f.required && f.type !== 'checkbox' && !(values[f.name] ?? '').trim())
  const problems = Object.fromEntries(fields.map((f) => [f.name, fieldProblem(f, values[f.name] ?? '')]))
  const crossProblem = validate?.(values) ?? null
  const blocked = missing || Object.values(problems).some(Boolean) || crossProblem !== null
  const checks = fields.filter((f) => f.type === 'checkbox')
  const radios = fields.filter((f) => f.type === 'radio')
  const inputs = fields.filter((f) => f.type !== 'checkbox' && f.type !== 'radio')
  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) cancel() }}>
      <DialogContent showCloseButton={false} aria-describedby={undefined} className="sm:max-w-[520px]">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (blocked) return
            const submitted = values
            setValues({})
            onConfirm(submitted)
          }}
          className="grid gap-4"
        >
          <DialogHeader>
            <DialogTitle className="text-xl">{title}</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground [&_p]:m-0">{body}</div>
          {inputs.map((f) => (
            <div key={f.name} className="grid gap-1.5">
              <Label htmlFor={`${id}-${f.name}`}>{f.label}{f.required ? ' (required)' : ''}</Label>
              <Input
                id={`${id}-${f.name}`}
                type={f.type === 'number' ? 'text' : (f.type ?? 'text')}
                required={f.required}
                inputMode={f.type === 'number' ? 'numeric' : f.type === 'email' ? 'email' : undefined}
                autoComplete={f.type === 'email' ? 'email' : f.type === 'password' ? 'new-password' : 'off'}
                aria-invalid={problems[f.name] ? true : undefined}
                aria-describedby={problems[f.name] ? `${id}-${f.name}-p` : undefined}
                value={values[f.name] ?? ''}
                onChange={(e) => { setValues({ ...values, [f.name]: e.target.value }) }}
              />
              {problems[f.name] ? <p id={`${id}-${f.name}-p`} className="text-xs text-destructive">{problems[f.name]}</p> : f.help ? <p className="text-xs text-muted-foreground">{f.help}</p> : null}
            </div>
          ))}
          {radios.map((f) => (
            <fieldset key={f.name} className="grid gap-2 rounded-lg border bg-muted/40 p-3">
              <legend className="px-1 text-sm font-medium">{f.label}</legend>
              {(f.options ?? []).map((o) => (
                <label key={o.value} className="flex cursor-pointer items-start gap-2.5 rounded-md p-1.5 has-[:checked]:bg-primary-container/60">
                  <input type="radio" name={`${id}-${f.name}`} value={o.value} className="mt-1 size-4 accent-primary" checked={(values[f.name] ?? '') === o.value}
                    onChange={() => { setValues({ ...values, [f.name]: o.value }) }} />
                  <span className="grid gap-0.5 text-sm"><span className="font-medium">{o.label}</span>{o.description ? <span className="text-muted-foreground">{o.description}</span> : null}</span>
                </label>
              ))}
            </fieldset>
          ))}
          {checks.length > 0 ? (
            <div className="grid gap-2.5 rounded-lg border bg-muted/40 p-3 sm:grid-cols-2">
              {checks.map((f) => (
                <div key={f.name} className="flex items-center gap-2.5">
                  <Checkbox id={`${id}-${f.name}`} checked={values[f.name] === 'true'} onCheckedChange={(c) => { setValues({ ...values, [f.name]: String(c === true) }) }} />
                  <Label htmlFor={`${id}-${f.name}`} className="font-normal">{f.label}</Label>
                </div>
              ))}
            </div>
          ) : null}
          {crossProblem ? <p className="text-sm text-destructive" role="alert">{crossProblem}</p> : null}
          <DialogFooter className="mt-2">
            <Button type="button" variant="outline" onClick={cancel}>Cancel</Button>
            <Button type="submit" variant={danger ? 'destructive' : 'default'} disabled={blocked}>{confirmLabel}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function Empty({ children, icon: Icon = Inbox }: { children: ReactNode; icon?: typeof Inbox }) {
  return (
    <div className="mt-4 flex flex-col items-center gap-2 rounded-xl border border-dashed bg-card/60 px-6 py-10 text-center text-sm text-muted-foreground">
      <Icon aria-hidden="true" className="size-6 opacity-60" />
      <p className="m-0">{children}</p>
    </div>
  )
}
