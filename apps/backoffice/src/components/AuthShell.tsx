import type { ReactNode } from 'react'
import { BrandGlyph } from './BrandGlyph'

/**
 * The signed-out page (MASTER.md v4.1, as in the player app): the brand's indigo fills the screen and the form sits in
 * the middle of it on one white card with a deep shadow, the mark above it in white. Sign-in, the password reset and
 * the first-sign-in steps share it. `bare` leaves the card to the caller (a shadcn Card).
 */
export function AuthShell({ children, bare = false, labelledBy }: { children: ReactNode; bare?: boolean; labelledBy?: string }) {
  return (
    <main id="main" className="relative grid min-h-svh place-items-center overflow-hidden bg-hero px-4 py-8 md:p-8">
      {/* a lighter indigo from the top left, a faint gold from the far corner (the logo's fourth segment) */}
      <div aria-hidden="true" className="pointer-events-none absolute inset-0 bg-[radial-gradient(70%_60%_at_0%_0%,rgb(143_136_255_/_0.45)_0%,transparent_60%),radial-gradient(60%_50%_at_100%_100%,rgb(255_224_138_/_0.16)_0%,transparent_60%)]" />
      <div className="relative w-full max-w-[440px]">
        <p className="m-0 mb-6 flex items-center justify-center gap-3 font-display text-2xl font-extrabold tracking-tight text-white">
          <span aria-hidden="true" className="flex size-11 items-center justify-center rounded-2xl bg-white/15 ring-1 ring-white/25"><BrandGlyph className="size-8" /></span>
          4draws
        </p>
        {bare ? children : (
          <section aria-labelledby={labelledBy} className="rounded-3xl bg-card p-7 shadow-sheet animate-swing-in md:p-9">{children}</section>
        )}
        <p className="m-0 mt-5 text-center text-xs text-white/75">Staff back office · Nairobi time</p>
      </div>
    </main>
  )
}
