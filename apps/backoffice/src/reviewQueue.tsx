import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useSession } from './context'
import { can } from './roles'

/** How many identity submissions wait for a decision (counts only: reading it unmasks nobody). */
export interface ReviewQueue { waiting: number; oldest_submitted_at: string | null }

interface ReviewQueueState {
  /** null until first read, and always for members who do not review identities */
  queue: ReviewQueue | null
  /** submissions that arrived since the reviewer last looked */
  arrived: number
  dismiss: () => void
  /** asks the server again now (after a decision, so the count and the list agree) */
  refresh: () => void
}

export const POLL_MS = 30_000
const TITLE = 'Back office'

const ReviewQueueContext = createContext<ReviewQueueState>({ queue: null, arrived: 0, dismiss: () => undefined, refresh: () => undefined })

export const useReviewQueue = () => useContext(ReviewQueueContext)

/**
 * Polls the identity queue for reviewers (every 30 s while the tab is visible, and whenever it regains focus) and
 * counts arrivals. One request at a time; an answer that arrives after a newer one is ignored.
 */
export function ReviewQueueProvider({ children }: { children: ReactNode }) {
  const { me, api } = useSession()
  const reviewer = can(me.roles, 'kyc_reviewer')
  const [queue, setQueue] = useState<ReviewQueue | null>(null)
  const [arrived, setArrived] = useState(0)
  const last = useRef<number | null>(null)
  const inflight = useRef(false)
  const live = useRef(true)

  const check = useCallback(() => {
    if (!reviewer || inflight.current) return
    inflight.current = true
    api.get<ReviewQueue>('/v1/staff/kyc/queue').then(
      (q) => {
        if (!live.current) return
        const grew = last.current === null ? 0 : q.waiting - last.current
        if (grew > 0) setArrived((a) => a + grew)
        last.current = q.waiting
        setQueue(q)
      },
      () => undefined, // keep the last count; the next poll asks again
    ).finally(() => { inflight.current = false })
  }, [api, reviewer])

  useEffect(() => {
    if (!reviewer) return
    live.current = true
    last.current = null // a new session or reviewer starts counting arrivals afresh
    const onFocus = () => { check() }
    const onVisible = () => { if (document.visibilityState === 'visible') check() }
    const tick = () => { if (document.visibilityState !== 'hidden') check() }
    check()
    const timer = setInterval(tick, POLL_MS)
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      live.current = false
      clearInterval(timer)
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [check, reviewer])

  useEffect(() => {
    document.title = queue && queue.waiting > 0 ? `(${queue.waiting}) ${TITLE}` : TITLE
    return () => { document.title = TITLE }
  }, [queue])

  const dismiss = useCallback(() => { setArrived(0) }, [])
  return <ReviewQueueContext.Provider value={{ queue, arrived, dismiss, refresh: check }}>{children}</ReviewQueueContext.Provider>
}

export const submissions = (n: number) => `${n} identity ${n === 1 ? 'submission waits' : 'submissions wait'} for review`
