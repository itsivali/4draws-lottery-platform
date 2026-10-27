# 0017. Weekly, monthly and quarterly draws follow each player's own periods

Status: accepted
Date: 2026-10-26

## Context
Draws ran on calendar periods shared by everyone: the day, Monday to Sunday, the calendar month and quarter. A player
who started on a Saturday had two days in their first weekly draw. The owner decided (2026-10-26) that periods are
counted from the day the player starts playing: a player who starts on Wednesday 1 October has the week 1–7 October,
drawn on the 8th, whoever else is playing.

## Decision
- **A player's start** is the Nairobi date of their first paid play, recorded once in `tickets.player_starts` by the
  play itself (append-only; existing players backfilled from their first played day). It never moves.
- **Periods are fixed lengths from the start:** weekly 7 days, monthly 30 days, quarterly 90 days, back to back
  (`draws.PersonalPeriod`). Fixed lengths keep every player's period the same size and make the draw windows below
  identical for everyone in them. Daily draws are unchanged: the day is the same for everyone.
- **One draw per type and closing day.** Each day the scheduler opens a weekly, a monthly and a quarterly draw whose
  window ends that day (e.g. weekly: the last 7 days), if any player's period ends that day (`basis = 'personal'`).
  It holds the play days, in the window, of exactly those players. Everything after that is the existing lifecycle:
  frozen at 00:05 the next day, signed by two officials before the drand round at 09:00, conducted, approved, paid.
- **The pool is a share of the stakes of the play days in the draw** (not of everyone's stakes in the window), so
  each stake funds one draw of each type, as before.
- **A play day enters at most one draw of each type.** At the switch, open calendar weekly, monthly and quarterly
  draws are voided ("draw periods changed to each player's own"); calendar draws already frozen finish as they are,
  and their play days are not entered again in a personal draw. Personal draws therefore pick up every other play
  day whose player's period ends from the switch onwards.
- The certificate of a personal draw says `"basis": "personal"`; players see their own current periods
  (`GET /v1/draws/periods`).

## Consequences
- Up to four draws a day to sign and approve (two officials each) instead of one, plus one weekly, monthly and
  quarterly at the end of each calendar period before.
- Small cohorts: a draw may hold one or two players, who then win its prizes (one prize per player per draw). This is
  how the rule plays out with few players; whether a minimum number of players should apply, and the period lengths,
  are the owner's to confirm with the BCLB along with the game rules.
- Play days whose personal period ended before the switch and that were not in a frozen calendar draw are not drawn
  (test data only at the switch).
