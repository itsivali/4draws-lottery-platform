/** The 4draws mark without its tile (design-system/lottery-platform/logo/4draws-glyph.svg): the 4 in a ring of four
 *  segments, one gold. Drawn in currentColor so it sits on the sidebar's tile and on the sign-in panel's gradient alike. */
export function BrandGlyph({ className = 'size-5' }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} aria-hidden="true" fill="none" strokeLinecap="round" strokeLinejoin="round">
      <path d="M36.68 9.99 A22.5 22.5 0 0 1 54.01 27.32" stroke="#FFC53D" strokeWidth="4.5" />
      <path d="M54.01 36.68 A22.5 22.5 0 0 1 36.68 54.01 M27.32 54.01 A22.5 22.5 0 0 1 9.99 36.68 M9.99 27.32 A22.5 22.5 0 0 1 27.32 9.99" stroke="currentColor" strokeWidth="4.5" opacity="0.92" />
      <path d="M36.5 44.5 V19.5 L22 38 H43" stroke="currentColor" strokeWidth="5.5" />
    </svg>
  )
}
