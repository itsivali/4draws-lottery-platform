import { useCallback, useEffect, useState } from 'react'
import { ProblemError, UncertainError } from './api'

export interface Load<T> {
  data: T | undefined
  error: unknown
  loading: boolean
  reload: () => void
}

/** Loads server data; reload() asks again. Never shows stale data as fresh: errors stay visible until it succeeds. */
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[] = []): Load<T> {
  const [state, setState] = useState<{ data?: T; error?: unknown; loading: boolean }>({ loading: true })
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let live = true
    setState((s) => ({ ...s, loading: true }))
    fn().then(
      (data) => { if (live) setState({ data, loading: false }) },
      (error: unknown) => { if (live) setState((s) => ({ ...s, error, loading: false })) },
    )
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, ...deps])
  return { data: state.data, error: state.error, loading: state.loading, reload: useCallback(() => { setTick((t) => t + 1) }, []) }
}

export type ActionState =
  | { kind: 'idle' }
  | { kind: 'busy' }
  | { kind: 'done'; message: string }
  | { kind: 'refused'; error: ProblemError }
  | { kind: 'uncertain'; retry: () => Promise<unknown> }
  | { kind: 'failed'; message: string }

/** Runs one staff action at a time and keeps its outcome honest: refused, done, or unknown (check again). */
export function useAction(onDone?: () => void) {
  const [state, setState] = useState<ActionState>({ kind: 'idle' })
  const run = useCallback(
    async (fn: () => Promise<unknown>, success: string) => {
      setState({ kind: 'busy' })
      try {
        await fn()
        setState({ kind: 'done', message: success })
        onDone?.()
      } catch (e) {
        if (e instanceof ProblemError) setState({ kind: 'refused', error: e })
        else if (e instanceof UncertainError) setState({ kind: 'uncertain', retry: e.retry })
        else setState({ kind: 'failed', message: e instanceof Error ? e.message : 'Unexpected error' })
      }
    },
    [onDone],
  )
  return { state, run, reset: useCallback(() => { setState({ kind: 'idle' }) }, []) }
}
