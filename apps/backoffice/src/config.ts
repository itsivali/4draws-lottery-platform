/** Build-time configuration (Vite env). Staff sign in with email and password (ADR 0019); Google, Apple and, in
 *  development, the local staff realm are offered when configured. A missing or insecure server stops the app. */
export interface Config {
  apiBaseUrl: string
  google?: { clientId: string }
  apple?: { clientId: string; redirectUri: string }
  /** the local staff realm (Keycloak in compose); never set in a deployed build */
  dev?: { authority: string; clientId: string }
}

function secure(u: string): boolean {
  try {
    const url = new URL(u)
    const local = url.hostname === 'localhost' || url.hostname === '127.0.0.1'
    return url.protocol === 'https:' || (url.protocol === 'http:' && local)
  } catch {
    return false
  }
}

export function loadConfig(env: Record<string, string | undefined> = import.meta.env, origin = typeof location === 'undefined' ? '' : location.origin): Config | null {
  const apiBaseUrl = env.VITE_API_BASE_URL
  if (!apiBaseUrl || !secure(apiBaseUrl)) return null
  const c: Config = { apiBaseUrl }
  if (env.VITE_GOOGLE_CLIENT_ID) c.google = { clientId: env.VITE_GOOGLE_CLIENT_ID }
  if (env.VITE_APPLE_CLIENT_ID) c.apple = { clientId: env.VITE_APPLE_CLIENT_ID, redirectUri: env.VITE_APPLE_REDIRECT_URI ?? `${origin}/` }
  if (env.VITE_DEV_OIDC_AUTHORITY && env.VITE_DEV_OIDC_CLIENT_ID) {
    if (!secure(env.VITE_DEV_OIDC_AUTHORITY)) return null
    c.dev = { authority: env.VITE_DEV_OIDC_AUTHORITY, clientId: env.VITE_DEV_OIDC_CLIENT_ID }
  }
  return c
}
