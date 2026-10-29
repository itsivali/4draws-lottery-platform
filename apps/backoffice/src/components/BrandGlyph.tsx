/** The Tajirika T with the wheel's gold pointer (design-system/lottery-platform/logo/tajirika-glyph.svg), drawn in
 *  currentColor so it sits on the sidebar's coral tile and on the sign-in panel's gradient alike. */
export function BrandGlyph({ className = 'size-5' }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} aria-hidden="true">
      <rect x="15" y="17" width="34" height="10" rx="3" fill="currentColor" />
      <rect x="27" y="17" width="10" height="33" rx="3" fill="currentColor" />
      <path d="M32 7.5 L36.6 14 L27.4 14 Z" fill="#F2C14E" />
    </svg>
  )
}
