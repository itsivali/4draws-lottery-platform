import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
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

/** One page of a cursor-paged list, as the API answers it. */
export interface Page<T> { rows: T[]; next: string | null }

export interface Pages<T> extends Load<T[]> {
  /** the server has rows after the last page loaded */
  more: boolean
  /** the next page is on its way (the rows so far stay up) */
  loadingMore: boolean
  loadMore: () => void
}

interface Loaded<T> { id: string; key: string; rows: T[]; next: string | null; pages: number }
interface PagesAnswer<T> { id: string; key: string; rows?: T[]; next: string | null; pages: number; error?: unknown }

/**
 * Loads a cursor-paged list page by page: the first page on mount, loadMore() appends the next. A change of deps is a
 * different question (the old rows are dropped); reload() reads every page loaded so far again, in order, so the
 * effect of an action shows without losing the operator's place in the list. Rows appear once the pages asked for are
 * all in, so a reload never shows a list shrinking to its first page.
 */
export function usePages<T>(fetchPage: (cursor: string | null, signal: AbortSignal) => Promise<Page<T>>, deps: unknown[] = []): Pages<T> {
  const [tick, setTick] = useState(0)
  const key = JSON.stringify(deps)
  const id = `${tick}:${key}`
  const [wanted, setWanted] = useState<{ key: string; pages: number }>({ key, pages: 1 })
  const pagesWanted = wanted.key === key ? wanted.pages : 1
  const [answer, setAnswer] = useState<PagesAnswer<T> | null>(null)
  const [busy, setBusy] = useState(false)
  const progress = useRef<Loaded<T> | null>(null) // the pages read so far for id, kept across an interrupted effect
  const load = useEffectEvent((cursor: string | null, signal: AbortSignal) => fetchPage(cursor, signal))
  useEffect(() => {
    const ac = new AbortController()
    let cur = progress.current?.id === id ? progress.current : { id, key, rows: [], next: null, pages: 0 }
    if (cur.pages >= pagesWanted) return
    setBusy(true)
    void (async () => {
      try {
        while (cur.pages < pagesWanted && (cur.pages === 0 || cur.next !== null)) {
          const page = await load(cur.next, ac.signal)
          if (ac.signal.aborted) return
          cur = { id, key, rows: [...cur.rows, ...page.rows], next: page.next, pages: cur.pages + 1 }
          progress.current = cur
        }
        setAnswer({ id, key, rows: cur.rows, next: cur.next, pages: cur.pages })
      } catch (error: unknown) {
        if (ac.signal.aborted) return
        // a failed reload or a failed next page keeps the rows it could not refresh or extend; a failed new question has none
        setAnswer((a) => ({ id, key, next: cur.next, pages: cur.pages, error, ...(a?.key === key ? { rows: a.rows } : {}) }))
      } finally {
        if (!ac.signal.aborted) setBusy(false)
      }
    })()
    return () => { ac.abort() }
  }, [id, key, pagesWanted])
  const current = answer?.key === key ? answer : null
  const settled = current?.id === id
  return {
    data: current?.rows,
    error: current?.error,
    loading: busy && !settled,
    reload: useCallback(() => { setTick((t) => t + 1) }, []),
    more: settled && current.next !== null,
    loadingMore: busy && settled,
    loadMore: useCallback(() => { setWanted((w) => ({ key, pages: (w.key === key ? w.pages : 1) + 1 })) }, [key]),
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
