import '@testing-library/jest-dom/vitest'

// jsdom has neither; the sidebar's breakpoint hook and Radix's layout code expect them
if (typeof window.matchMedia !== 'function') {
  window.matchMedia = (query: string) => ({
    matches: false, media: query, onchange: null,
    addListener: () => undefined, removeListener: () => undefined,
    addEventListener: () => undefined, removeEventListener: () => undefined, dispatchEvent: () => false,
  })
}
if (typeof window.ResizeObserver !== 'function') {
  window.ResizeObserver = class { observe() { /* jsdom */ } unobserve() { /* jsdom */ } disconnect() { /* jsdom */ } }
}
if (typeof Element.prototype.scrollIntoView !== 'function') Element.prototype.scrollIntoView = () => undefined
if (typeof Element.prototype.hasPointerCapture !== 'function') Element.prototype.hasPointerCapture = () => false
if (typeof Element.prototype.releasePointerCapture !== 'function') Element.prototype.releasePointerCapture = () => undefined
