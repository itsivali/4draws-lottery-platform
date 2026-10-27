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
}

export const POLL_MS = 30_000
const TITLE = 'Back office'

const ReviewQueueContext = createContext<ReviewQueueState>({ queue: null, arrived: 0, dismiss: () => undefined })

export const useReviewQueue = () => useContext(ReviewQueueContext)

/** Polls the identity queue for reviewers (every 30 s and whenever the tab regains focus) and counts arrivals. */
export function ReviewQueueProvider({ children }: { children: ReactNode }) {
  const { me, api } = useSession()
  const reviewer = can(me.roles, 'kyc_reviewer')
  const [queue, setQueue] = useState<ReviewQueue | null>(null)
  const [arrived, setArrived] = useState(0)
  const last = useRef<number | null>(null)

  useEffect(() => {
    if (!reviewer) return
    let live = true
    const check = () => {
      api.get<ReviewQueue>('/v1/staff/kyc/queue').then(
        (q) => {
          if (!live) return
          const grew = last.current === null ? 0 : q.waiting - last.current
          if (grew > 0) setArrived((a) => a + grew)
          last.current = q.waiting
          setQueue(q)
        },
        () => undefined, // keep the last count; the next poll asks again
      )
    }
    const onVisible = () => { if (document.visibilityState === 'visible') check() }
    check()
    const timer = setInterval(check, POLL_MS)
    window.addEventListener('focus', check)
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      live = false
      clearInterval(timer)
      window.removeEventListener('focus', check)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [api, reviewer])

  useEffect(() => {
    document.title = queue && queue.waiting > 0 ? `(${queue.waiting}) ${TITLE}` : TITLE
    return () => { document.title = TITLE }
  }, [queue])

  const dismiss = useCallback(() => { setArrived(0) }, [])
  return <ReviewQueueContext.Provider value={{ queue, arrived, dismiss }}>{children}</ReviewQueueContext.Provider>
}

export const submissions = (n: number) => `${n} identity ${n === 1 ? 'submission waits' : 'submissions wait'} for review`
