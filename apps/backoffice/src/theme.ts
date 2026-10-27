/** Light, dark or the system setting; remembered per browser. The CSS follows the system unless <html> is classed. */
export type ThemeMode = 'light' | 'dark' | 'system'

const KEY = 'backoffice.theme'

export function readTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'dark' ? v : 'system'
  } catch {
    return 'system'
  }
}

export function applyTheme(mode: ThemeMode) {
  const root = document.documentElement
  root.classList.remove('light', 'dark')
  if (mode !== 'system') root.classList.add(mode)
  try {
    if (mode === 'system') localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, mode)
  } catch { /* not remembered */ }
}
