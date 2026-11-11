# Play — page rules (overrides MASTER.md where stated)

The only screen with depth, metal and motion. Everything else about it follows MASTER.md.

## The wheel is a display, never a decider
- The Go API picks the slot (`POST /v1/plays` → `play.spin.slot` of `play.spin.slots`; slots below
  `lucky_slots` are lucky). The wheel's segments are those slots, numbered as the server numbers them, and it stops
  on the server's slot. Nothing on the client generates randomness, and no outcome is ever drawn before the answer.
- Before the server has described the wheel (no play yet today), it is drawn as 12 plain segments with no numbers and
  no lucky mark. Once a play arrives, the wheel takes the server's slot count and lucky slots.
- While waiting for the answer the wheel spins without meaning; the outcome-bound slowing is planned once, from the
  answer, and then only evaluated per frame (`WheelMotion`). Resizes, rebuilds and dropped frames cannot change it.
- A refused or unanswered play slows to rest without an outcome (no stop haptic, no result).

## Anatomy (`wheel_painters.dart`, proportions of the radius)
| Part | Rule |
|---|---|
| Backplate | One soft shadow, static |
| Disc (rotates) | Segments alternate `wheelSegmentA` / `wheelSegmentB`; lucky segments `wheelLucky` with a drawn star. Radial shading per segment. Hairline separators, boundary studs at 0.86. Numbers at 0.70, upright along the radius; labels containing 6 or 9 are underlined |
| Rim (static) | 0.90–1.00, brushed gold sweep lit from the top left, bevels |
| Sheen (static) | Diagonal light across the face: the light does not turn, which makes the disc read as turning |
| Hub (static) | Gold ring, emerald cap |
| Pointer (static) | Gold, at 12 o'clock, tip inside the rim |

Gold appears here and nowhere else in the app except the lucky star.

## Motion (`SpinTiming`)
Accelerate 600 ms (smoothstep velocity) → cruise at 2 turns/s for at least 1.2 s and for as long as the answer takes
→ decelerate with an ease-out cubic whose starting velocity equals the cruise speed (seamless hand-off), about 2.25–3.75 s
so the stop lands inside the slot at a ticket-specific spot (0.25–0.75 of the slot) → rest. A quick answer gives a
4–5.5 s spin. Reduced motion: no spin; a still progress ring in the hub while waiting, then the wheel is set on the slot.

## Haptics (`SpinHaptics`)
`start` (light) once per play · `tick` (selection) only while decelerating, only below 14 segments/s, ≥ 45 ms apart,
≤ 28 per spin, each at a real pointer crossing of a stud · `stop` (medium) once, only on a server outcome. Off on the
web and when the player turns *Wheel haptics* off.

## Copy
- Heading: "Play recorded" or "Lucky slot — today's chances are doubled" (or "…today was already lucky").
- Always: the play number, the slot of N, the day's chances, "This is not a prize: prizes are decided only in the
  draws." Never "win", "jackpot", "so close", celebration effects or countdowns.
- The day's counts and today's plays update only after the wheel stops, so nothing gives the slot away early.

## Layout
Phone: wheel (≤ 46 % of the height, ≤ 420) above the day strip, status panel, the play button and "What a play
does". Desktop (≥ 1024): wheel left, panel right, so price, plays left and the result are visible together.
