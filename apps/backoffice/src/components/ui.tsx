import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { ProblemError } from '../api'
import type { ActionState, Load } from '../hooks'

export type Tone = 'success' | 'warning' | 'error' | 'pending' | 'neutral'

const toneIcon: Record<Tone, string> = { success: '✓', warning: '!', error: '×', pending: '◷', neutral: '•' }

/** A state shown with a symbol, a word and a colour: never colour alone. */
export function Pill({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span className={`pill pill-${tone}`}>
      <span aria-hidden="true">{toneIcon[tone]}</span> {children}
    </span>
  )
}

export function PageHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <header className="page-header">
      <h1>{title}</h1>
      {children ? <p className="muted">{children}</p> : null}
    </header>
  )
}

/** Loading, error and data for one read. */
export function Loaded<T>({ load, what, children }: { load: Load<T>; what: string; children: (data: T) => ReactNode }) {
  if (load.data !== undefined) return <>{children(load.data)}</>
  if (load.error !== undefined) {
    const msg = load.error instanceof ProblemError ? load.error.title : 'The server could not be reached'
    return (
      <div className="notice notice-error" role="alert">
        <p>Couldn’t load {what}: {msg}</p>
        <button type="button" onClick={load.reload}>Try again</button>
      </div>
    )
  }
  return <p className="muted" aria-live="polite">Loading {what}…</p>
}

/** What happened to the last action, in words, announced to screen readers. */
export function ActionResult({ state, onCheckAgain }: { state: ActionState; onCheckAgain?: (retry: () => Promise<unknown>) => void }) {
  switch (state.kind) {
    case 'idle':
    case 'busy':
      return null
    case 'done':
      return <p className="notice notice-success" role="status">{state.message}</p>
    case 'refused':
      return (
        <div className="notice notice-error" role="alert">
          <p><strong>{state.error.title}</strong></p>
          {state.error.detail ? <p>{state.error.detail}</p> : null}
          <p className="muted">Nothing was changed.</p>
        </div>
      )
    case 'uncertain':
      return (
        <div className="notice notice-pending" role="alert">
          <p><strong>No answer from the server.</strong> The action may or may not have been done. Checking repeats the same request, so it cannot happen twice.</p>
          {onCheckAgain ? <button type="button" onClick={() => { onCheckAgain(state.retry) }}>Check again</button> : null}
        </div>
      )
    case 'failed':
      return <p className="notice notice-error" role="alert">{state.message}</p>
  }
}

export interface ConfirmField { name: string; label: string; required?: boolean; type?: 'text' | 'number' | 'checkbox'; help?: string }

/** Asks before an action: what it does, and the reason or reference the audit trail needs. */
export function ConfirmDialog({
  open,
  title,
  body,
  fields = [],
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
  /** values the fields start with (checkboxes: "true") */
  initial?: Record<string, string> | undefined
  confirmLabel: string
  danger?: boolean
  onConfirm: (values: Record<string, string>) => void
  onCancel: () => void
}) {
  const ref = useRef<HTMLDialogElement>(null)
  const id = useId()
  // the caller remounts the dialog (key) for each question, so initial values apply afresh
  const [values, setValues] = useState<Record<string, string>>(initial ?? {})
  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open && !d.open) {
      if (typeof d.showModal === 'function') d.showModal()
      else d.setAttribute('open', '')
    } else if (!open && d.open) {
      if (typeof d.close === 'function') d.close()
      else d.removeAttribute('open')
    }
  }, [open])
  const missing = fields.some((f) => f.required && f.type !== 'checkbox' && !(values[f.name] ?? '').trim())
  return (
    <dialog ref={ref} aria-labelledby={`${id}-t`} onCancel={(e) => { e.preventDefault(); setValues({}); onCancel() }}>
      <form
        method="dialog"
        onSubmit={(e) => {
          e.preventDefault()
          if (missing) return
          const submitted = values
          setValues({})
          onConfirm(submitted)
        }}
      >
        <h2 id={`${id}-t`}>{title}</h2>
        <div className="dialog-body">{body}</div>
        {fields.map((f) => (
          <label key={f.name} className={f.type === 'checkbox' ? 'check' : 'field'}>
            {f.type === 'checkbox' ? (
              <input type="checkbox" checked={values[f.name] === 'true'} onChange={(e) => { setValues({ ...values, [f.name]: String(e.target.checked) }) }} />
            ) : null}
            <span>{f.label}{f.required ? ' (required)' : ''}</span>
            {f.type === 'checkbox' ? null : (
              <input
                type={f.type ?? 'text'}
                required={f.required}
                value={values[f.name] ?? ''}
                onChange={(e) => { setValues({ ...values, [f.name]: e.target.value }) }}
              />
            )}
            {f.help ? <small className="muted">{f.help}</small> : null}
          </label>
        ))}
        <div className="dialog-actions">
          <button type="button" className="secondary" onClick={() => { setValues({}); onCancel() }}>Cancel</button>
          <button type="submit" className={danger ? 'danger' : 'primary'} disabled={missing}>{confirmLabel}</button>
        </div>
      </form>
    </dialog>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty">{children}</p>
}
