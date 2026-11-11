# Mobile release audit (baseline, 2026-11-07)

Read from `apps/mobile` on `main` at `9e11bf4`. Flutter is pinned to 3.47.7 (`mise.toml:4`, `.github/workflows/ci.yml:150`,
`mobile.yml:34`).

## Android

| Item | Current | Evidence | Status |
|---|---|---|---|
| applicationId / namespace | `ke.tajirika.lottery_mobile` | `android/app/build.gradle.kts:8,19` | OK; template TODOs removed (S6) |
| Label | Tajirika | `src/main/AndroidManifest.xml:5` | OK |
| min / target / compile SDK | `flutter.*` defaults | `build.gradle.kts:9,10,22,23` | Fixed (S6): compile and target 36 pinned, min 24 (Flutter 3.47.7) |
| AGP / Gradle / Kotlin / JVM | 9.1.0 / 9.3.1 / 2.4.0 / 17 | `settings.gradle.kts:22-23`, `gradle-wrapper.properties:5` | OK |
| versionCode | `flutter.versionCode` from `pubspec.yaml` `1.0.0+1`; CI never passes `--build-number` | `build.gradle.kts:28-29` | Fixed (S6): CI passes the run number as `--build-number`, the tag as `--build-name` |
| Release signing | **debug key** | `build.gradle.kts:33-37` | Fixed (S6): upload key from `key.properties` or CI secrets; a release build without it stops (`docs/mobile/android-release.md`) |
| Permissions | `INTERNET` only | `src/main/AndroidManifest.xml:3` | OK |
| Network security | release: platform default (https only); debug: cleartext to local hosts | `src/debug/res/xml/network_security_config.xml` | OK |
| Backup | `allowBackup=false`, backup and device-transfer rules exclude everything | manifest, `res/xml/` | Fixed (S6) |
| Deep / app links | none; `signinwithapple://callback` only | manifest `:32-42` | OK for launch; app links later |
| Play Integrity | verifier on server, no client plugin | ADR 0014 | Owner dependency |

## iOS

| Item | Current | Evidence | Status |
|---|---|---|---|
| Bundle id | `ke.tajirika.lotteryMobile` | `Runner.xcodeproj/project.pbxproj:387` | OK (differs in style from Android; harmless) |
| Deployment target | 15.0 | `project.pbxproj:363` | OK |
| Device family | iPhone and iPad (`1,2`) | `project.pbxproj:368` | iPad screenshots will be required; consider iPhone only |
| Team / signing | no `DEVELOPMENT_TEAM`, automatic | `project.pbxproj` | **Blocker** (owner's Apple account) |
| Entitlements | Sign in with Apple only | `Runner/Runner.entitlements` | OK |
| URL types | none | `Runner/Info.plist` | **Blocker** if Google sign-in is on for iOS |
| Privacy manifest | none | — | **Blocker** |
| Export compliance | `ITSAppUsesNonExemptEncryption` unset | `Info.plist` | Gap (asked at every upload) |
| Orientation | iPhone allows landscape | `Info.plist:56-61` | Gap (the app is designed portrait) |
| `CFBundleName` | `lottery_mobile` | `Info.plist:17-18` | Gap |
| CI | `flutter build ios --no-codesign` on tags | `mobile.yml:39-51` | No archive, no TestFlight |

## Store prerequisites

- No privacy policy, terms, responsible-gambling or account-deletion page (`apps/web` is empty); Play requires an
  account-deletion URL and both stores require a privacy policy URL.
- No in-app links to policies.
- Account deletion in the app is a request completed by staff (`features/account/presentation/closure_screen.dart`).
- No fastlane, store metadata or screenshots.
- No push notifications (no Firebase project).
- Licensing evidence: none; both stores require it for real-money gambling (see `docs/compliance/requirements-matrix.md`).
