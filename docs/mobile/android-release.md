# Android release (S6)

How the store build of the Tajirika app is made and signed. Store listing, the gambling-app application and the data
safety form are S10 (`docs/release/store-submission-checklist.md`, to come).

## What a release build is

| Item | Value | Where |
|---|---|---|
| Application id | `ke.tajirika.lottery_mobile` (never changes once published) | `android/app/build.gradle.kts` |
| Target / compile SDK | 36, pinned (Play requires 36 for new apps and updates; recheck at each submission) | same |
| Minimum SDK | 24 (Flutter 3.47.7's default) | same |
| Version code | the `mobile` workflow's run number (only grows) | `.github/workflows/mobile.yml` |
| Version name | the tag without its `v` (`v1.2.0` → `1.2.0`), else `pubspec.yaml` | same |
| Signing | the upload key, or the build stops; never the debug key | `build.gradle.kts` |
| Backup | none: `allowBackup=false`, and the backup and device-transfer rules exclude everything | `AndroidManifest.xml`, `res/xml/` |
| Permissions | `INTERNET` only | `AndroidManifest.xml` |

The app keeps a session and a per-install device fingerprint. A backup restored on another phone would carry a session
that nobody signed in to there, so nothing is backed up. A player on a new phone signs in again.

## The upload key and Play App Signing

Google Play signs what players download with the **app signing key**, which Google keeps (Play App Signing, required
for new apps). We sign what we upload with the **upload key**. If the upload key is lost or leaks, Google can reset it.
The app signing key never leaves Google.

Creating the upload key (once, by the owner, on a trusted machine):

```sh
keytool -genkeypair -v -keystore upload.jks -alias upload -keyalg RSA -keysize 4096 -validity 10000
```

Keep `upload.jks` and both passwords in the owner's password manager, with a second copy offline. Then:

1. **Play Console.** Create the app (`ke.tajirika.lottery_mobile`) and enrol in Play App Signing. Upload the first
   bundle signed with this key; Play registers it as the upload key.
2. **GitHub secrets** (`docs/ci-secrets.md`):
   ```sh
   base64 -w0 upload.jks | gh secret set ANDROID_UPLOAD_KEYSTORE_BASE64
   gh secret set ANDROID_KEYSTORE_PASSWORD
   gh secret set ANDROID_KEY_ALIAS     # upload
   gh secret set ANDROID_KEY_PASSWORD
   ```
3. **Releasing.** Tag a release, for example `v1.0.0`. The `mobile` workflow builds the signed bundle (`app-release.aab`)
   and keeps it as the `android-release` artifact, for upload to Play by hand. Uploading automatically waits until the
   Play Console account exists (S10).

A build on a developer's machine reads the same four values from `apps/mobile/android/key.properties`, which git
ignores:

```properties
storeFile=/absolute/path/to/upload.jks
storePassword=…
keyAlias=upload
keyPassword=…
```

Without them, `flutter build appbundle --release` (or `apk --release`, `run --release`) stops with an error. That is
deliberate: no release build is ever debug-signed. Debug builds (`flutter run`) are unaffected.

## Pull requests

Pull requests have no secrets, so their release build is signed with a throwaway key generated in the job. This proves
the release build works. That bundle (the `android-ci-throwaway-key` artifact) can be installed for testing but never
uploaded to Play, which would refuse it.

## If the upload key leaks

Ask Google Play support to reset the upload key (Play Console → Setup → App signing). Generate a new key as above,
replace the secrets, and rotate both passwords. Players' installed apps are not affected, because they are signed with
Google's app signing key.

## Checked by tests

`apps/mobile/test/release/android_release_config_test.dart` fails if any of these regresses:

- a release is signed with the debug key;
- the SDK levels are no longer pinned;
- backup is allowed;
- CI no longer sets the build number.
