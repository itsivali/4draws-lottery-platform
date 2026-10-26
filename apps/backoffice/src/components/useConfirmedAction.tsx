import { useState, type ReactNode } from 'react'
import { useAction } from '../hooks'
import { ActionResult, ConfirmDialog, type ConfirmField } from './ui'

export interface ActionSpec {
  title: string
  body: ReactNode
  fields?: ConfirmField[]
  confirmLabel: string
  danger?: boolean
  success: string
  act: (values: Record<string, string>) => Promise<unknown>
}

/** Ask, then act, then say what happened (and reload). One action at a time. */
export function useConfirmedAction(reload: () => void) {
  const [spec, setSpec] = useState<ActionSpec | null>(null)
  const { state, run } = useAction(reload)
  const view = (
    <>
      <ConfirmDialog
        open={spec !== null}
        title={spec?.title ?? ''}
        body={spec?.body}
        {...(spec?.fields ? { fields: spec.fields } : {})}
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
  return { ask: setSpec, busy: state.kind === 'busy', view }
}
