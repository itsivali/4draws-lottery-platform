/**
 * Offers the browser's password manager a credential after a sign-in or a password change (the Credential Management
 * API, Chrome and Edge; Safari and Firefox rely on the form's autocomplete attributes instead). Never throws: a manager
 * that is absent or declines changes nothing.
 */
export function rememberCredential(id: string, password: string, name?: string): void {
  try {
    const w = window as Window & { PasswordCredential?: new (init: { id: string; password: string; name?: string }) => Credential }
    const manager = navigator.credentials as CredentialsContainer | undefined
    if (!w.PasswordCredential || !manager) return
    void manager.store(new w.PasswordCredential(name ? { id, password, name } : { id, password })).catch(() => undefined)
  } catch {
    /* no password manager */
  }
}
