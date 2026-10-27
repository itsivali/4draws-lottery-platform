import { useCallback, useState, type ReactNode } from 'react'
import { useAction } from '../hooks'
import { ActionResult, ConfirmDialog, type ConfirmField } from './ui'

export interface ActionSpec {
  title: string
  body: ReactNode
  fields?: ConfirmField[]
  initial?: Record<string, string>
  /** a check across fields (e.g. at least one role); its message blocks the confirm button */
  validate?: (values: Record<string, string>) => string | null
  confirmLabel: string
  danger?: boolean
  success: string
  act: (values: Record<string, string>) => Promise<unknown>
}

/** Ask, then act, then say what happened (and reload). One action at a time. */
export function useConfirmedAction(reload: () => void) {
  const [spec, setSpec] = useState<ActionSpec | null>(null)
  const [asked, setAsked] = useState(0) // a fresh dialog per question
  const { state, run } = useAction(reload)
  const view = (
    <>
      <ConfirmDialog
        key={asked}
        open={spec !== null}
        title={spec?.title ?? ''}
        body={spec?.body}
        {...(spec?.fields ? { fields: spec.fields } : {})}
        {...(spec?.validate ? { validate: spec.validate } : {})}
        initial={spec?.initial}
        confirmLabel={spec?.confirmLabel ?? 'Confirm'}
        danger={spec?.danger ?? false}
        onCancel={() => { setSpec(null) }}
        onConfirm={(values) => {
          const s = spec
          setSpec(null)
          if (s) void run(() => s.act(values), s.success)
        }}
      />
      <ActionResult state={state} onCheckAgain={(retry) => void run(retry, 'Confirmed by the server.')} />
    </>
  )
  const ask = useCallback((s: ActionSpec) => { setAsked((n) => n + 1); setSpec(s) }, [])
  return { ask, busy: state.kind === 'busy', view }
}
