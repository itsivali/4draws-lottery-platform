export interface Config { apiBaseUrl: string; authority: string; clientId: string }

/** Build-time configuration (Vite env). Missing values stop the app instead of guessing a server. */
export function loadConfig(env: Record<string, string | undefined> = import.meta.env): Config | null {
  const apiBaseUrl = env.VITE_API_BASE_URL
  const authority = env.VITE_OIDC_AUTHORITY
  const clientId = env.VITE_OIDC_CLIENT_ID
  if (!apiBaseUrl || !authority || !clientId) return null
  for (const u of [apiBaseUrl, authority]) {
    try {
      const url = new URL(u)
      const local = url.hostname === 'localhost' || url.hostname === '127.0.0.1'
      if (url.protocol !== 'https:' && !(url.protocol === 'http:' && local)) return null
    } catch {
      return null
    }
  }
  return { apiBaseUrl, authority, clientId }
}
