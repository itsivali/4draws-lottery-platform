# 4draws — Design System (Master, v4 “Indigo, Material”)

The single source of truth for how the player app (`apps/mobile`, Flutter: iOS, Android, web) looks, moves and feels.
Page files in `pages/` record only deviations from this file; when a page file exists its rules win for that page.

The tokens below are implemented once in `apps/mobile/lib/app/theme/` (`tokens.dart`, `theme.dart`). Widgets read
them through `Theme.of(context)` and `context.tokens`; **no widget contains a raw hex colour, font size, radius,
spacing value or animation duration.** `test/app/theme/tokens_test.dart` checks every text/surface pair for contrast.

Generated with the UI/UX Pro Max skill (`search.py --design-system`, `--domain color|ux`, `--stack flutter`) and then
adapted; the deviations from its output are listed under *Provenance* at the end.

**v3 (2026-11-11, owner: "redesign the web app and the mobile apps"):** the Sunset palette is unchanged; the shapes,
type and depth follow two reference shots (a light wellness app and a frosted fitness dashboard). A soft off-white
page; white cards with 28 dp corners and a faint ambient shadow; big numbers in a light (300) weight with tight
negative tracking under small muted captions; soft coral and violet radial glows behind key numbers (`GlowPanel`);
frosted quick-action tiles (`GlassChip`); a dark selected segment for toggles, chips and navigation (`PillToggle`,
`selected`); small round icon buttons, the dark one meaning "go on" (`RoundIconButton`); ring gauges with a number
inside (`RingGauge`); a dotted dial around the wheel (`DottedDial`); a bar chart whose selected bar glows, with round
chips under it (`GlowBars`); pill-shaped buttons. All of it lives in `core/widgets/glow.dart` and the theme. Where this
file and v3 disagree below, v3 wins and the section says so.

## 1. Brand personality

**Calm, exact, honest — with two moments of theatre.** The app sits between a fintech wallet and a well-made physical
object. Everything is quiet, legible and financially transparent; the balance and the wheel are the two places with
depth, metal and motion (the *hero surfaces*, §5), and even there they only ever show what the server decided.

- Trustworthy before exciting. Numbers are large, aligned and never ambiguous.
- Plain language. "Play recorded", "1 draw chance", "Pending M-Pesa confirmation" — never "JACKPOT!", "So close!" or
  "You're due".
- No manufactured urgency: no countdown pressure, no scarcity, no near-miss staging, no confetti for ordinary plays.

**Mark and wordmark:** the 4draws mark is a sunset tile (coral into violet, 25% corner radius) carrying a white T,
with the wheel's pointer above it in decorative gold (`logo/4draws-mark.svg`; `BrandMark` draws it, `scripts/
gen-icons.sh` renders every app icon and favicon from it). The wordmark is "4draws" in the heading face, semibold.
Layouts only give the mark a height.

**v4 (2026-11-11, owner, with the 4draws name: "native, Google Material design, especially Android; reduce the 3D;
polish the colour scheme, the buttons, the icons and the wheel"):** the app is a Material 3 app in the logo's indigo.
Where this file and v4 disagree below, v4 wins.

- **Colour:** the Indigo palette (§2). Coral and violet are gone with the old name; gold stays decorative.
- **Components are the platform's own:** flat filled, outlined and tonal buttons (no gradient, no gloss, no shadow);
  outlined text fields; a full-width `NavigationBar` with the tonal indicator pill (a `NavigationRail` on wide
  screens); filter chips; a bottom sheet with a drag handle; ripples and state layers as the only press feedback.
- **Transitions are the platform's own:** fade-forwards on Android, the slide with edge-swipe back on iOS; tabs fade
  through. Lists still rise in, 10 dp, once.
- **Icons:** Material Symbols Rounded, outlined at rest and filled when chosen in navigation; an icon that heads a
  card, row or notice sits in a flat tonal disc (`IconDisc`).
- **Depth is reduced to two things:** the gold coin that turns beside the balance (`Coin3D`), and the wheel's own
  lighting. Nothing else is 3D: no tilting cards, no glossy orbs or buttons, no moving backdrop, no 3D page turns.
- **The balance is back on the hero card** (indigo gradient, white numerals, three tonal quick actions); the profile
  header and every other panel are plain white cards. The wheel faces the player squarely, centred in its dotted dial.
- **Shapes:** cards 20 dp, fields 14 dp, hero surfaces 24 dp, sheets 28 dp, buttons fully rounded; a card's lift is
  barely there (`Elevation.card`).

## 2. Colour

**One theme: light, on a warm near-white page** (owner, 2026-10-28; the page took a linen tint on 2026-10-29 so the
white cards stand off it). The palette is “Sunset”: coral leads, violet accents,
peach and lavender tints, warm gold kept for metal. Semantic roles only. Gold is **decorative** (the wheel rim, the
lucky-slot star, the pointer). Gold is never the colour of an ordinary success, a button or a balance: it must not
suggest that money was won.

| Role | Value | Use |
|---|---|---|
| `background` | `#F7F7FB` cool off-white | Page background (white cards stand off it) |
| `surface` | `#FFFFFF` | Cards, sheets, bars (a hairline border and a soft lift set them off the page) |
| `surfaceRaised` | `#FBFBFE` | Panels that sit on `surface` |
| `surfaceMuted` | `#EFEEF8` | Tonal: inset panels, skeletons, the segmented toggle's track |
| `ink` | `#1B1A2E` | Primary text |
| `inkMuted` | `#5A5870` | Secondary text, captions |
| `primary` | `#4338CA` indigo (the logo's) | Primary action, focus ring |
| `onPrimary` | `#FFFFFF` | Text/icons on primary |
| `primaryContainer` | `#E4E2FF` | Icon discs, info highlights |
| `onPrimaryContainer` | `#211A78` | Text and icons on primaryContainer |
| `accent` | `#5B4FE0` lighter indigo | The far end of bars and ring gauges |
| `gold` | `#B47A12` | Decorative metal only (rim, pointer, lucky star) |
| `goldText` | `#7A5200` | The word "Lucky" next to the star |
| `success` | `#1B7A43` | Confirmed by the server (credited, paid, recorded) |
| `warning` | `#9A5600` | Needs attention (limit reached, KYC in review) |
| `error` | `#C62828` | Refused or failed |
| `pending` | `#0F6A80` teal | Waiting on M-Pesa / the server: never green, and never the indigo of an action |
| `border` | `#E4E3F0` | Hairlines and dividers (decorative) |
| `outline` | `#7C7A93` | Input and control boundaries (≥ 3:1) |
| `wheelSegments` | `#FF7A1A` `#FFC53D` `#2DD4BF` `#4CC9FF` `#B794FF` `#FF6FB5` | The wheel's six segment colours, used in turn; the lucky slot is `wheelLucky` `#FFE066`; labels `wheelLabel` `#2A1A2E` on all of them; each segment is two-tone: solid where its number sits, a white-and-colour checker (`wheelChecker` `#FFFFFF`) from the hub out to half the radius; the lucky star is `wheelStar` `#000000` (owner, 2026-10-28: the wheel is bright, the one place colour runs free) |
| `heroStart` → `heroEnd` | `#5B4FE9` → `#2A2385` | The hero surfaces' indigo gradient (through `primary`), lit from the top left by `heroGlow` `#8F88FF` |
| `onHero` / `onHeroMuted` | `#FFFFFF` / `#DDDBFF` | Text on a hero surface |
| `selected` / `onSelected` | `#E4E2FF` / `#211A78` | Material's selected state: the navigation indicator, a chosen chip, tonal icon buttons |
| `glass` / `glassBorder` | white 10% / white 18% | A tonal wash on a hero surface (the quick actions on the balance) |
| `glowViolet` | `#B7B1FF` | A soft indigo highlight |
| `track` | `#E6E5F2` | The empty part of a bar or ring (decorative, never text) |

Every text colour clears 4.5:1 on the page, on cards and on muted panels; outlines and the pointer clear 3:1
(`tokens_test.dart`). Every status colour is paired with an icon and a word (`StatusPill`): colour is never the only
signal.

**Pending is teal, not green.** A pending top-up or payout must not look like a success.

The staff back office (`apps/backoffice`) maps the same roles onto its own tokens (`src/index.css`), and still wears
the earlier coral and violet until its own pass.

## 3. Typography

**IBM Plex Sans** (bundled, OFL; no runtime font download) for everything. Headlines, titles and the balance are the
*display* styles: heavier (700 for display sizes) and set tighter (−0.2 to −1.2 tracking) so they read as a distinct
voice from the body without a second family. `AppType.display` names the display family: **Manrope** (bundled, OFL; a
variable font whose weight the theme sets through `FontVariation`). Tabular figures (`tnum`) are switched on for
every number style so KES amounts, counts and times align and do not jitter as they change.

| Token | Size / line height | Weight | Use |
|---|---|---|---|
| `display` | 60 / 64 and 48 / 54 | 300, −2.4 / −1.8 tracking (v3) | The balance on Home and Wallet (`BigAmount`: the currency small and muted, the figure large) |
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

- Cards are `surface` (white) on the linen page, with a 1 dp `border` hairline, 24 dp of padding and a **pronounced
  lift** (`Elevation.card`: `0 1 3 / 6%` + `0 14 36 / 13%` plum ink; raised on 2026-10-29, owner: cards must stand
  out). `flat` cards (inside sheets, nested lists) keep the border and drop the lift; `tinted` cards sit on
  `primaryContainer` for quiet emphasis. The primary button carries a coral shadow of its own.
- Elevation exists in three steps: `card` (above), `float` (sheets, dialogs, tooltips: `0 20 56 / 22%`) and `hero`
  (`0 22 56 / 48% heroEnd`). The wheel has its own lighting (see `pages/play.md`).
- **Hero surfaces** (`HeroCard`): the sunset gradient `heroStart → primary → accent → heroEnd` (coral through violet,
  four stops since 2026-10-29 so both ends read) with a radial
  `heroGlow` from the top left; the wheel's stage takes the dusk end only (`heroEnd → shadow`, `Gradients.stage`) so
  the coral and violet segments and the gold rim stand out from it, `hero` elevation and `xl` corners (no ornaments:
  the corner rings were removed on 2026-10-29, owner). Exactly two uses: the balance
  (Home, Wallet) and the wheel's stage (Play). On wide screens the sign-in page's brand panel borrows it for words,
  never for money. Nothing else is a hero.
- Radii: `sm 8` (chips), `md 12` (buttons, inputs), `lg 20` (cards; 16 until 2026-10-29), `xl 24` (hero cards, dialogs), `xxl 28` (sheets'
  top corners, the wheel's stage), `full` (pills).
- v3: glows and frost are allowed, but only as `GlowPanel` (the balance on Home and Wallet, the profile header),
  `GlassChip` (quick actions on a glow), the glow under the selected bar and around the wheel, and the bar/ring
  gradient (coral into violet). No backdrop blur (cost on low-end phones); frost is translucent white. Text on a glow
  or on glass is checked for 4.5:1 at the glow's strongest point (`tokens_test.dart`).
- v3: the balance is no longer on the sunset hero; it is the big light numeral on a `GlowPanel`. `HeroCard` remains
  for the wheel's stage and the sign-in brand panel.
- Radii (v3): `lg` is 24 (banners, panels), `card` 28 (cards, dialogs), `field` 16 (inputs), buttons are stadiums.

## 6. Iconography

Material Symbols **Rounded** (the Flutter SDK's `Icons.*_rounded`), 24 dp, one weight; set in glossy `Orb`s where they
head a card, row or notice (§8.1). No emoji as icons. Decorative
icons beside visible text are excluded from semantics; icon-only buttons always carry a tooltip and semantic label.

Fixed meanings: `casino_rounded` is **not** used (no casino imagery). Play = `play_circle_rounded` (a spin's own actions use `play_arrow_rounded`);
draws = `emoji_events_rounded`; wallet = `account_balance_wallet_rounded`; account = `person_rounded`;
home = `home_rounded`; verify = `verified_rounded`; lucky = `star_rounded`; pending = `schedule_rounded`;
success = `check_circle_rounded`; error = `error_rounded`; warning = `warning_rounded`.

## 7. Components

- **Buttons:** one primary (`FilledButton`, coral) per screen; secondary `OutlinedButton`; tertiary `TextButton`.
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
- **Navigation:** 5 destinations — Home, Play, Draws, Wallet, Account. Icon + label always. v3: on phones the bar is
  a white pill floating over the page; on tablets and desktops the rail is a white panel. The selected destination is
  a dark (`selected`) pill with a white icon and a bold label.
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

### 8.1 Depth (v4: reduced)

The second pass of v3 put the whole app in perspective (tilting cards, glossy orbs and buttons, a drifting backdrop,
3D page turns, a leaning wheel). The owner asked for less of it the same day, and for a native feel: all of that is
removed. What remains:

- **The coin:** a gold `Coin3D` (a struck "4", a star on the back, a milled edge built from layers sorted by depth)
  turns slowly beside the balance on Home and Wallet. It stands for money held, so it never appears on a play's
  result: a play wins draw chances, not cash.
- **The wheel's lighting:** its rim, studs and lights (`pages/play.md`). It faces the player squarely.
- Under `disableAnimations` the coin holds a three-quarter pose; ambient loops (`Ambient.enabled`) are off in tests.

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

## 12. Light only

There is one theme, light, on a white page (owner, 2026-10-28); the app does not follow the system's dark setting,
so a balance or a status never changes colour between devices. Reduced motion is still honoured (§8).

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
  coral/white/ink palette above (emerald/ivory in v2). A gold primary would make every ordinary action look like a prize, which conflicts
  with the product's honesty rules; purple-on-navy is the "AI gradient" look the same output lists as an anti-pattern.
  The skill's banking palette (navy + muted gold accent) informed keeping gold muted and decorative.
