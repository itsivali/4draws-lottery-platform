# Player app (Flutter: iOS, Android, web)

The player app for the lottery platform (ADR 0012). The Go API is the source of truth for every play, slot, chance,
balance and payment; the app shows what it answers and never decides an outcome. Design system:
[`design-system/lottery-platform/MASTER.md`](../../design-system/lottery-platform/MASTER.md).

## Run

```sh
mise install                       # pinned Flutter 3.47.7
make dev                           # from the repository root: the API on http://localhost:8080
cd apps/mobile
flutter run --dart-define=API_BASE_URL=http://localhost:8080             # Android emulator: http://10.0.2.2:8080
flutter run -d chrome --dart-define=API_BASE_URL=http://localhost:8080   # needs CORS_ALLOWED_ORIGINS on the API
```

Release builds must name the server (`--dart-define=API_BASE_URL=https://…`); without it the app refuses to start.
No credentials live in the app.

## Check

```sh
dart format --output=none --set-exit-if-changed lib test integration_test
flutter analyze
flutter test                                    # unit, widget, golden and the journey (fake API, no network)
flutter test --update-goldens                   # after an intended visual change; review the images
flutter test integration_test                   # on a device or emulator
flutter build web --release --dart-define=API_BASE_URL=https://…
```

## Layout

```
lib/app/         bootstrap, router (session guards), adaptive shell, theme tokens
lib/core/        API client (typed failures, idempotency keys), session (single-flight refresh), formatting,
                 haptics, shared widgets
lib/features/    play (wheel geometry, motion, haptics, controller), draws, wallet (settlement flows),
                 account (KYC, responsible gaming, closure), fairness (on-device spin verifier), auth, home
test/support/    fixtures shaped like the Go handlers' JSON, and an in-memory fake of the API
```

## How the important parts behave

- **Plays:** `PlayController` sends `POST /v1/plays` with a fresh idempotency key, kept (with its owner) until the
  answer is known. A timeout, dropped connection, 5xx or `request-in-progress` is *uncertain*: it is checked again
  with the same key (the API replays the play, never makes a second), automatically for about half a minute, then
  on request. An unfinished play found at launch is checked by itself only if under 15 minutes old and the same
  player's; otherwise the player decides.
- **Payments:** `SettlementFlow` follows a top-up or withdrawal to the server's final state with bounded backoff.
- **Sessions:** refresh tokens are in the Keychain/Keystore on phones and in memory on the web (a reload signs out).
  Refresh is single-flight because a reused refresh token revokes the session.
- **Haptics:** only through `Haptics`/`SpinHaptics`; off on the web and behind the *Wheel haptics* setting.
