# iOS release (S7)

How the App Store build of the 4draws app is set up. Archiving, signing and TestFlight need the owner's Apple
Developer account and a Mac; they cannot run on Linux. The store listing, age rating and the gambling review notes are
S10.

## What the build is

| Item | Value | Where |
|---|---|---|
| Bundle id | `ke.co.fourdraws.app` (never changes once the app exists in App Store Connect) | `Runner.xcodeproj` |
| Display and bundle name | 4draws | `Runner/Info.plist` |
| Devices | iPhone only (`TARGETED_DEVICE_FAMILY = 1`): no iPad screenshots or layout to review | `Runner.xcodeproj` |
| Orientation | portrait only, as the app is designed | `Info.plist` |
| Deployment target | iOS 15.0 | `Runner.xcodeproj` |
| Export compliance | `ITSAppUsesNonExemptEncryption = false`: only the system's encryption (HTTPS, the keychain), exempt, so App Store Connect does not ask at each upload | `Info.plist` |
| Privacy manifest | `Runner/PrivacyInfo.xcprivacy`, bundled (`docs/mobile/privacy-manifests.md`) | Xcode project |
| Sign in with Apple | entitlement | `Runner/Runner.entitlements` |
| Google sign-in | URL scheme `$(GOOGLE_REVERSED_CLIENT_ID)` | `Info.plist`, xcconfig |
| Build number / version | the `mobile` workflow's run number / the tag | `.github/workflows/mobile.yml` |

## The owner's values: `Local.xcconfig`

The Apple team id and the Google iOS client are the owner's. They are kept out of the repository: copy
`apps/mobile/ios/Flutter/Local.xcconfig.example` to `Local.xcconfig` (git ignores it) and fill in:

- **`DEVELOPMENT_TEAM`:** developer.apple.com → Membership details → Team ID.
- **`GOOGLE_REVERSED_CLIENT_ID`:** the *reversed* `GOOGLE_IOS_CLIENT_ID` (`com.googleusercontent.apps.…`), from
  Google Cloud console → Credentials → "4draws iOS" (`docs/runbooks/player-sign-in.md`).

Without it the app still builds: signing has no team (archives fail until it is set), and the Google URL scheme is a
placeholder nothing answers, so Google sign-in on iOS stays off. Phone sign-in and Sign in with Apple are unaffected.

## Releasing (needs the Apple account)

1. Apple Developer: the App ID `ke.co.fourdraws.app` with Sign in with Apple
   (`docs/runbooks/player-sign-in.md`).
2. App Store Connect: create the app with that bundle id.
3. On a Mac with Xcode and `Local.xcconfig` in place:
   `flutter build ipa --release --build-number=<n> --dart-define=API_BASE_URL=https://api.tajirika.co.ke`. Then
   upload `build/ios/ipa/*.ipa` with Transporter, or `xcrun altool`.
4. TestFlight first. Real-money gambling apps need Apple's approval under guideline 5.3 and the licence before the
   review: S10, and Milestone 7 is blocked on licensing.

CI builds the app unsigned on tags (`flutter build ios --no-codesign`) to prove it compiles. Signed archives in CI wait
for the Apple account: an App Store Connect API key and a distribution certificate as secrets.

## Checked by tests

`apps/mobile/test/release/ios_release_config_test.dart` fails if any of these regresses:

- the bundle name;
- the export flag;
- iPhone-only portrait;
- the Google URL scheme;
- `Local.xcconfig` being optional and ignored;
- a team id committed to the project;
- the privacy manifest missing from the bundle.
