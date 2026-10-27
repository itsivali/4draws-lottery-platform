# Lottery Platform — Design System (Master, v2)

The single source of truth for how the player app (`apps/mobile`, Flutter: iOS, Android, web) looks, moves and feels.
Page files in `pages/` record only deviations from this file; when a page file exists its rules win for that page.

The tokens below are implemented once in `apps/mobile/lib/app/theme/` (`tokens.dart`, `theme.dart`). Widgets read
them through `Theme.of(context)` and `context.tokens`; **no widget contains a raw hex colour, font size, radius,
spacing value or animation duration.** `test/app/theme/tokens_test.dart` checks every text/surface pair for contrast.

Generated with the UI/UX Pro Max skill (`search.py --design-system`, `--domain color|ux`, `--stack flutter`) and then
adapted; the deviations from its output are listed under *Provenance* at the end.

## 1. Brand personality

**Calm, exact, honest — with two moments of theatre.** The app sits between a fintech wallet and a well-made physical
object. Everything is quiet, legible and financially transparent; the balance and the wheel are the two places with
depth, metal and motion (the *hero surfaces*, §5), and even there they only ever show what the server decided.

- Trustworthy before exciting. Numbers are large, aligned and never ambiguous.
- Plain language. "Play recorded", "1 draw chance", "Pending M-Pesa confirmation" — never "JACKPOT!", "So close!" or
  "You're due".
- No manufactured urgency: no countdown pressure, no scarcity, no near-miss staging, no confetti for ordinary plays.

**Wordmark:** "Bahati" set in the heading face, semibold, with a small emerald disc carrying a gold pointer notch
(`BrandMark` widget). It is a placeholder: swap `BrandMark` for the final logo without touching layouts (it sizes to
its parent height).

## 2. Colour

Semantic roles only. Gold is **decorative** (the wheel rim, the lucky-slot star, the pointer). Gold is never the colour
of an ordinary success, a button or a balance: it must not suggest that money was won.

| Role | Light | Dark | Use |
|---|---|---|---|
| `background` | `#F7F4EC` warm ivory | `#0B1411` | Page background |
| `surface` | `#FFFDF8` | `#121E1A` | Cards, sheets, bars |
| `surfaceMuted` | `#EFEADF` | `#1A2823` | Inset panels, skeletons, chips |
| `ink` | `#13201B` | `#ECF1ED` | Primary text |
| `inkMuted` | `#56625C` | `#A3AFA9` | Secondary text, captions |
| `primary` | `#0B5D45` forest emerald | `#62C9A0` | Primary action, selected nav, focus ring |
| `onPrimary` | `#FFFFFF` | `#00291C` | Text/icons on primary |
| `primaryContainer` | `#D3EBDF` | `#123D30` | Selected chips, nav indicator, info highlights |
| `onPrimaryContainer` | `#053B2B` | `#C6EBDB` | Text on primaryContainer |
| `accent` | `#0F6B66` restrained teal | `#5CC4BA` | Links, secondary emphasis (sparingly) |
| `gold` | `#9C7A2E` | `#D4B36A` | Decorative metal only (rim, pointer, lucky star) |
| `goldText` | `#7A5B17` | `#D4B36A` | The word "Lucky" next to the star |
| `success` | `#1B7344` | `#72D39C` | Confirmed by the server (credited, paid, recorded) |
| `warning` | `#8A5300` | `#F0B45B` | Needs attention (limit reached, KYC in review) |
| `error` | `#B3261E` | `#FF8A80` | Refused or failed |
| `pending` | `#33528F` | `#9DB6F2` | Waiting on M-Pesa / the server — never green |
| `border` | `#D9D2C2` | `#2B3B35` | Hairlines and dividers (decorative) |
| `outline` | `#7C8580` | `#7F8C86` | Input and control boundaries (≥ 3:1) |
| `surfaceRaised` | `#FFFFFF` | `#182822` | Inputs and panels that sit on `surface` |
| `heroStart` → `heroEnd` | `#0E6A4F` → `#063A2C` | `#145640` → `#0B2E24` | The hero surfaces' gradient, lit from the top left by `heroGlow` |
| `onHero` / `onHeroMuted` | `#FFFFFF` / `#CFE6DB` | `#F2F7F3` / `#B8D8C8` | Text on a hero surface |

Every status colour is paired with an icon and a word (`StatusPill`): colour is never the only signal.

**Pending is blue, not green.** A pending top-up or payout must not look like a success.

## 3. Typography

**IBM Plex Sans** (bundled, OFL; no runtime font download) for everything. Headlines, titles and the balance are the
*display* styles: heavier (700 for display sizes) and set tighter (−0.2 to −1.2 tracking) so they read as a distinct
voice from the body without a second family. `AppType.display` names the display family: **Manrope** (bundled, OFL; a
variable font whose weight the theme sets through `FontVariation`). Tabular figures (`tnum`) are switched on for
every number style so KES amounts, counts and times align and do not jitter as they change.

| Token | Size / line height | Weight | Use |
|---|---|---|---|
| `display` | 48 / 56 and 40 / 48 | 700 | The balance on Wallet and Home (on the hero surface) |
| `headline` | 28 / 36 | 700 / 600 | Screen titles |
| `title` | 20 / 28 | 600 | Card titles, sheet titles |
| `titleSmall` | 16 / 24 | 600 | Section headers, list titles |
| `body` | 16 / 24 | 400 | Body text (minimum body size: 16) |
| `bodySmall` | 14 / 20 | 400 | Captions, helper text |
| `label` | 14 / 20 | 500 | Buttons, nav labels, pills |
| `amount` | 20 / 28 | 600 + `tnum` | Inline amounts (`MoneyText`) |
| `mono` | 13 / 20 | 400 (system monospace) | Hashes, seeds, receipts — selectable, wraps anywhere |

Rules: sentence case everywhere; never below 12 logical px; all sizes scale with the system text scaler (tested at
2.0×). Money is formatted only by `formatKes()` ("KES 1,250"), dates only by `core/formatting`.

## 4. Spacing, layout and breakpoints

4-point scale: `xxs 2 · xs 4 · sm 8 · md 12 · lg 16 · xl 24 · xxl 32 · xxxl 48`.

| Width | Class | Navigation | Content |
|---|---|---|---|
| < 600 | compact (phones) | Bottom `NavigationBar`, 5 destinations | Full width, 16 gutters |
| 600–1023 | medium (tablets) | `NavigationRail` (labels shown) | Max 720, centred |
| ≥ 1024 | expanded (laptop/desktop web) | Extended `NavigationRail` with wordmark | Max 1120; Play and Home use two columns |

The phone layout is never stretched across a desktop: `ContentFrame` caps width; two-column layouts put the wheel left
and price/plays/result right so the price and result stay visible without scrolling.

Touch targets ≥ 48×48 dp, ≥ 8 dp apart. Safe areas respected; the bottom bar reserves its own padding.

## 5. Surfaces, elevation, radius

- Cards are `surface` with a 1 dp `border` hairline and a **soft lift** (`Elevation.card`: `0 1 2 / 4%` + `0 8 24 / 6%`
  ink in light; `0 8 24 / 35%` black in dark). `flat` cards (inside sheets, nested lists) keep the border and drop the
  lift; `tinted` cards sit on `primaryContainer` for quiet emphasis.
- Elevation exists in three steps: `card` (above), `float` (sheets, dialogs, tooltips: `0 12 40 / 14%`) and `hero`
  (`0 16 40 / 35% heroEnd`). The wheel has its own lighting (see `pages/play.md`).
- **Hero surfaces** (`HeroCard`): the deep-emerald gradient `heroStart → heroEnd` with a radial `heroGlow` from the
  top left, faint concentric rings in a corner, `hero` elevation and `xl` corners. Exactly two uses: the balance
  (Home, Wallet) and the wheel's stage (Play). On wide screens the sign-in page's brand panel borrows it for words,
  never for money. Nothing else is a hero.
- Radii: `sm 8` (chips), `md 12` (buttons, inputs), `lg 16` (cards), `xl 24` (hero cards, dialogs), `xxl 28` (sheets'
  top corners, the wheel's stage), `full` (pills).
- No glassmorphism, no blur as decoration; no gradients except the hero surfaces and the wheel's rim and hub.

## 6. Iconography

Material Symbols **Rounded** (the Flutter SDK's `Icons.*_rounded`), 24 dp, one weight. No emoji as icons. Decorative
icons beside visible text are excluded from semantics; icon-only buttons always carry a tooltip and semantic label.

Fixed meanings: `casino_rounded` is **not** used (no casino imagery). Play = `donut_large_rounded`;
draws = `event_rounded`; wallet = `account_balance_wallet_rounded`; account = `person_rounded`;
home = `home_rounded`; verify = `verified_rounded`; lucky = `star_rounded`; pending = `schedule_rounded`;
success = `check_circle_rounded`; error = `error_rounded`; warning = `warning_rounded`.

## 7. Components

- **Buttons:** one primary (`FilledButton`, emerald) per screen; secondary `OutlinedButton`; tertiary `TextButton`.
  Height 52, radius `md`, label style. Busy state replaces the label with a 20 dp progress ring and keeps the width;
  the button is disabled while busy. Destructive actions (self-exclusion, sign out) use `error` and sit apart from
  other actions.
- **Fields:** visible label above, helper text below, error text below in `error` with an icon, announced. Phone fields
  use the phone keyboard and autofill hints; password fields have a show/hide toggle and allow paste and managers.
- **Cards:** `AppCard` — `surface`, radius `lg`, 16 padding, optional title row; `flat`, `tinted` and `onTap` variants.
  `HeroCard` for the two hero surfaces. `FactRow` / `FactGrid` for every label-over-value fact (prices, dates, hashes).
- **Rows:** `ListRow` (icon in a tinted disc, title, second line, trailing amount or pill, chevron) in a `RowGroup`
  with hairlines, or inside a title-only `AppCard`. No bare `ListTile`s.
- **Banners:** `InlineBanner` (tone, icon, title, one sentence, at most one action) for every in-page notice: blocks,
  next steps, errors (`ErrorNotice` is one), pending states. `EmptyState` (icon disc, sentence, one action) for every
  empty list. `SectionHeader` for a titled run of rows.
- **Status pill:** icon + word + colour (`StatusPill`, `compact` inside rows), for draw, top-up, payout and KYC states.
- **Sheets:** confirmation of anything that moves money is a bottom sheet (compact) or dialog (medium+): what it costs,
  what you get, what happens next, then *Confirm* (primary) and *Cancel*. Focus starts on the title.
- **Navigation:** 5 destinations — Home, Play, Draws, Wallet, Account. Icon + label always. Play's selected icon is the
  only filled-emerald circle in the bar; nothing else about it is louder.
- **Lists:** `ListTile`-based rows, 56–72 dp, trailing amount in `amount` style.

## 8. Motion

Durations (`MotionTokens`): `quick 120ms` (press, toggles), `standard 220ms` (state changes, crossfades),
`emphasis 360ms` (sheets, result reveal), `exit` = 65% of enter. Curves: `Curves.easeOutCubic` arriving,
`Curves.easeInCubic` leaving; linear only for constant rotation.

- Animate 1–2 elements per view. Content replacement crossfades; nothing slides in from nowhere.
- **Entrance:** a screen's cards arrive together with `Staggered`: each fades and rises 10 dp over `emphasis`, starting
  `stagger` (45 ms) after the one before. **Press:** tappable cards and the play button shrink to 0.98 for `press`
  (90 ms) under the finger (`PressScale`). Status panels enter with a fade and a 4% rise. All of it is off under
  reduced motion.
- Skeletons are static `surfaceMuted` blocks (a slow 1.2 s opacity pulse, off under reduced motion).
- Balance and counts do **not** count up: they crossfade to the new server value.
- **Reduced motion** (`MediaQuery.disableAnimations` or the in-app setting): page transitions become fades, skeletons
  stop pulsing, the wheel does not spin (it settles directly on the server's slot in 220 ms), and results appear
  without scale.

The wheel's motion is specified in `pages/play.md`.

## 9. Haptics

Haptics enhance, never inform alone. All feedback goes through `Haptics` (`core/platform/haptics.dart`); no widget
calls `HapticFeedback` directly.

| Moment | Effect | Rule |
|---|---|---|
| Play confirmed (request accepted locally) | `lightImpact` | Once per accepted play |
| Pointer crosses a segment boundary while decelerating | `selectionClick` | Only below the speed threshold; ≥ 45 ms apart; ≤ 28 per spin |
| Wheel comes to rest on the server's slot | `mediumImpact` | Once, only when a confirmed result exists |
| Confirmed money/state change (top-up credited, payout paid) | `lightImpact` | Once per state change, never for pending |
| Refusal or failure | none | The message carries it |

Disabled entirely on the web and when the player turns off *Wheel haptics* in Account → Preferences. The setting is
optional feedback only and has no effect on any outcome.

## 10. Accessibility

WCAG 2.2 AA as the floor:
- Text contrast ≥ 4.5:1, controls and focus ≥ 3:1 (enforced by `tokens_test.dart`).
- Text scale to 200 % without clipping important controls (widget tests run at 2.0×).
- Every interactive element has a semantic label; state changes (result, payment status) are announced through
  `SemanticsService` live announcements, without moving focus.
- The wheel is decorative to screen readers; the result panel carries the meaning ("Play 2 of 4 recorded. Slot 7 of
  12, not a lucky slot. You now have 2 draw chances today.").
- Visible focus ring (2 dp `primary`, offset 2) on web/keyboard; tab order follows reading order; Space/Enter operate
  the spin button.
- Nothing conveyed by colour, sound or vibration alone.

## 11. States

Every data view implements all five, with `AsyncView`:

| State | Pattern |
|---|---|
| Loading | Skeleton in the final layout's shape (no layout shift); never fake numbers |
| Empty | Short sentence + one action ("No plays yet today" → *Go to Play*) |
| Error | What failed + *Try again*; problem `title`/`detail` from the API when present |
| Offline | Banner "You're offline — showing what we last confirmed" + retry; money actions disabled |
| Pending | `pending` colour, `schedule` icon, what we are waiting for, that it is safe to leave |

Uncertain outcomes (a play or payment whose answer was lost) get their own state: "We're checking whether your play went
through" — never shown as failed, never shown as succeeded.

## 12. Light and dark

Both are first-class and designed together (tokens above). The app follows the system setting. Dark mode desaturates
and lightens the emerald rather than inverting; the wheel uses the same structure with darker segment fills and a
brighter rim.

## 13. Consistency rules (review checklist)

- [ ] No `Color(0x…)`, `fontSize:`, `BorderRadius.circular(<literal>)`, `EdgeInsets.all(<literal>)`,
      `BoxShadow(` or `Duration(milliseconds: <literal>)` outside `lib/app/theme/` and the wheel painter's geometry
      constants. Depth comes from `Elevation`, gradients from `Gradients`.
- [ ] `HeroCard` appears only for the balance, the wheel's stage and the sign-in brand panel.
- [ ] No bare `ListTile`; rows are `ListRow`. No ad-hoc icon-and-text notices; they are `InlineBanner`.
- [ ] Money only via `formatKes` / `MoneyText`; dates only via `core/formatting`.
- [ ] Every status shown with `StatusPill` (icon + word + colour).
- [ ] One primary button per screen.
- [ ] Gold only on the wheel and the lucky star.
- [ ] "Win"/"won" appears only next to a server-reported prize.
- [ ] Every screen has loading, empty, error and (where relevant) pending states.

## Provenance (UI/UX Pro Max output and what was changed)

- Kept: *Minimalism & Swiss Style* (clean, high-contrast, grid-based, low performance cost); *Trust & Authority*
  pattern (transparent pricing, low-friction forms); typography *IBM Plex Sans* ("financial, trustworthy"); its
  anti-patterns (playful design, unclear fees, purple/pink gradients); the pre-delivery checklist; the Flutter stack
  rules (Semantics, `TextScaler`, slivers).
- Changed: the generated palette (amber primary `#F59E0B`, purple accent `#8B5CF6`, navy background) was replaced by the
  emerald/ivory/ink palette above. A gold primary would make every ordinary action look like a prize, which conflicts
  with the product's honesty rules; purple-on-navy is the "AI gradient" look the same output lists as an anti-pattern.
  The skill's banking palette (navy + muted gold accent) informed keeping gold muted and decorative.
