# Privacy manifests (iOS)

Apple requires a privacy manifest (`PrivacyInfo.xcprivacy`) from every app and from SDKs on its list. It must agree
with the App Store privacy label filled in App Store Connect (S10). **This is a draft for the owner and counsel to
confirm before submission.**

## The app's own manifest (`apps/mobile/ios/Runner/PrivacyInfo.xcprivacy`)

- **Tracking:** none (`NSPrivacyTracking` false, no tracking domains). The app has no ads, no analytics SDK and no
  cross-app tracking.
- **Required-reason APIs used by the app's own code:** `UserDefaults`, reason `CA92.1` (the app's own settings, through
  shared_preferences).
- **Data collected.** All of it is linked to the player and used for app functionality, including fraud prevention and
  the law's requirements. None of it is used for tracking.

  | Data | Why |
  |---|---|
  | Phone number | The account; M-Pesa top-ups and payouts go to it |
  | Name | Identity as on the ID (KYC) |
  | Email address | Password resets |
  | User ID | The account |
  | Device ID | The per-install fingerprint that limits devices and checks attestation |
  | Purchase history | Top-ups and plays |
  | Other financial info | Wallet balance, M-Pesa payments and payouts |
  | Coarse location | The country of the connection, from the IP address (ADR 0031) |
  | Other data types | Date of birth and national ID number (age and identity checks) |

  The Google account photo (ADR 0035) is not collected by the app: the app shows the link Google gives.

## The plugins' manifests

Each plugin that touches a required-reason API ships its own manifest, which Xcode merges into the app's privacy
report:

- `shared_preferences_foundation`: UserDefaults;
- `flutter_secure_storage`: the keychain, not a required-reason API;
- `google_sign_in_ios` (GoogleSignIn SDK);
- `sign_in_with_apple`.

Before each submission, generate the privacy report in Xcode (Product → Archive → Generate Privacy Report) and check it
against the App Store privacy label. A plugin upgrade can add an API.
