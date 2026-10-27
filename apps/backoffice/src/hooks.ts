import { useCallback, useEffect, useEffectEvent, useState } from 'react'
import { ProblemError, UncertainError } from './api'

export interface Load<T> {
  data: T | undefined
  error: unknown
  loading: boolean
  reload: () => void
}

interface Answer<T> { id: string; key: string; data?: T; error?: unknown }

/**
 * Loads server data; reload() asks again. A reload keeps the last data up while it runs (and keeps it, with the error,
 * if it fails). A change of deps is a different question: the old answer is dropped, so stale rows are never shown
 * under new filters. The request in flight is cancelled when the question changes or the component unmounts.
 */
export function useLoad<T>(fn: (signal: AbortSignal) => Promise<T>, deps: unknown[] = []): Load<T> {
  const [tick, setTick] = useState(0)
  const key = JSON.stringify(deps)
  const id = `${tick}:${key}` // which question the answer below belongs to
  const [answer, setAnswer] = useState<Answer<T> | null>(null)
  const load = useEffectEvent((signal: AbortSignal) => fn(signal))
  useEffect(() => {
    const ac = new AbortController()
    load(ac.signal).then(
      (data) => { if (!ac.signal.aborted) setAnswer({ id, key, data }) },
      (error: unknown) => {
        // a failed reload keeps the data it could not refresh; a failed new question has none
        if (!ac.signal.aborted) setAnswer((a) => ({ id, key, error, ...(a?.key === key ? { data: a.data } : {}) }))
      },
    )
    return () => { ac.abort() }
  }, [id, key])
  const current = answer?.key === key ? answer : null
  return {
    data: current?.data,
    error: current?.error,
    loading: answer?.id !== id,
    reload: useCallback(() => { setTick((t) => t + 1) }, []),
  }
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
