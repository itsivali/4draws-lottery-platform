import { createContext, useContext } from 'react'
import type { ApiClient } from './api'
import type { Me } from './types'

export interface Session {
  me: Me
  api: ApiClient
  signOut: () => void
}

export const SessionContext = createContext<Session | null>(null)

export function useSession(): Session {
  const s = useContext(SessionContext)
  if (!s) throw new Error('useSession outside a signed-in session')
  return s
}
