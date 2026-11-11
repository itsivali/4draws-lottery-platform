# Player sign-in with Google and Apple

Players sign up and sign in with a phone number and password (ADR 0005), or with Google or Apple (ADR 0020): the
sign-in screen shows those buttons when the API lists them, a token for an identity that has an account signs in, and
one that has none is asked for the player's M-Pesa number (proved by the first top-up, as always) and nothing else.
Nothing works until the client ids below are set: unset, the buttons do not appear and the API refuses every token.

The back office's own Google and Apple sign-in (staff) is a separate set of client ids: `docs/runbooks/back-office.md`.

## Setting up Google (once, by the owner)

Google Cloud console → APIs & Services. One project (it may be the back office's).

1. **OAuth consent screen:** External; app name Tajirika, support email, authorised domain `tajirika.co.ke`, the
   privacy policy address. Scopes: only the default `openid`, `email`, `profile`. **Publish** it: in testing, only
   listed test users can sign in. (Non-sensitive scopes need no verification.)
2. **Credentials → Create OAuth client ID**, three times:
   - **Web application** "Tajirika player web": authorised JavaScript origins `https://tajirika.co.ke`,
     `https://www.tajirika.co.ke` and `http://localhost:8088` (local web app). No redirect URI (popup). Its client id
     is `GOOGLE_CLIENT_ID`. The phone apps name it too, as the server the token is for.
   - **Android** "Tajirika Android": package `ke.tajirika.lottery_mobile` and the SHA-1 of every signing certificate
     (the debug keystore, the upload key and Play App Signing's key, from Play Console → App integrity). No setting
     holds this id; it exists so Google lets the app sign in.
   - **iOS** "Tajirika iOS": bundle `ke.tajirika.lotteryMobile`. Its client id is `GOOGLE_IOS_CLIENT_ID`, and its
     *reversed* form (`com.googleusercontent.apps.…`) goes into `apps/mobile/ios/Runner/Info.plist` as a URL scheme
     (`CFBundleURLTypes` → `CFBundleURLSchemes`), so Google can return to the app.

## Setting up Apple (once, by the owner; needs the Apple Developer Program)

Apple Developer → Certificates, Identifiers & Profiles → Identifiers.

1. **App ID** `ke.tajirika.lotteryMobile` with the *Sign in with Apple* capability (the app's entitlements file,
   `apps/mobile/ios/Runner/Runner.entitlements`, already asks for it; Xcode's automatic signing adds it to the profile).
2. **Services ID** `ke.tajirika.web` with *Sign in with Apple* configured: primary App ID the one above; domains
   `tajirika.co.ke` and `api.tajirika.co.ke`; return URLs `https://tajirika.co.ke/` (the web app's popup) and
   `https://api.tajirika.co.ke/auth/apple/android` (the Android app's way back). Its identifier is `APPLE_CLIENT_ID`;
   the App ID's bundle is `APPLE_BUNDLE_ID`. Apple needs https, so it cannot be tried on localhost.

## Configuration

API (`render.yaml`, `lottery-api`; set in the dashboard, none is a secret):

```
GOOGLE_CLIENT_ID=<web client id>            # Google on; also the phone apps' server client id
GOOGLE_IOS_CLIENT_ID=<ios client id>        # optional: Google in the iOS app
APPLE_CLIENT_ID=<services id>               # Apple on the web and Android
APPLE_BUNDLE_ID=ke.tajirika.lotteryMobile   # Apple in the iOS app (natively)
ANDROID_PACKAGE=ke.tajirika.lottery_mobile  # where Apple's Android return sends the browser
```

The apps need no build setting: they ask `GET /v1/auth/providers` at the sign-in screen and offer what it lists. A
change in the dashboard shows in the apps within five minutes (the answer is cached that long) without a release;
the iOS URL scheme above is the one thing that needs a build.

## How it works

1. The app gets an ID token from Google's or Apple's own sign-in, bound to a random nonce (Apple is given its SHA-256).
2. `POST /v1/auth/sso/{provider}` with the token, the nonce and the device. The API checks signature, issuer,
   audience, expiry and nonce. Bound to an account: signed in. Not bound: 409 `sso-not-linked`.
3. The app asks for the M-Pesa number and the 18+ confirmation; `POST /v1/auth/sso/{provider}/signup` with the same
   token creates the account (no password; the number a claim until the first top-up proves it). The provider's tokens
   are short-lived (Apple's ten minutes): a player who dawdles is sent back to start again.
4. Android and Apple: Apple answers in the browser to `POST /auth/apple/android`, which redirects into the app
   (`intent://callback…` for the `sign_in_with_apple` plugin's callback activity). Nothing is verified or stored there.

## Forgotten passwords (ADR 0021)

Players give an email address at sign-up. "Forgotten your password?" on the sign-in screen emails a link to
`WEB_APP_URL/reset-password?token=…` (30 minutes, once) through the same SMTP as the staff notices; the answer is
the same whether or not the address has an account. `WEB_APP_URL` (`https://tajirika.co.ke`) must be set on
`lottery-api`, else the app says reset by email is not configured. A player with no address (an account from before,
or a Google or Apple one) adds one under Account → Email address; until then staff reset them with a temporary
password (below). The back office's Players page shows the address and searches by it.

## Operations

- A player who lost access to their Google or Apple account signs in again only with a password: the Players page →
  *Temporary password* (ADR 0019); the app then makes them choose their own, and the number and password work from
  then on, alongside the provider.
- The audit log shows `user.created` with `provider` and `sso.linked` for these accounts. The API's start-up log line
  says whether `google` and `apple` are on.
- Rotating a client id: set the new value in the dashboard; tokens for the old audience are refused from the next
  deploy. The web and Android apps pick the new id up without a release; the iOS Google client also needs the URL scheme.

## Local

The compose api passes `GOOGLE_CLIENT_ID` and `APPLE_CLIENT_ID` through from the shell: export the web client id (with
`http://localhost:8088` among its origins) before `scripts/dev-backend.sh`, then `scripts/dev-frontend.sh` and the
Google button appears on the local web app. Apple cannot be tried locally (https). The widget tests use a fake
provider (`test/support/app_harness.dart`), the Go tests a fake verifier.

## Photos and passwords for Google and Apple accounts (ADR 0035)

- **Photo.** A player or staff member who signs in with Google sees that account's photo, in the app's account header
  and the back office's account menu. It is refreshed at each Google sign-in and kept only when Google serves it from
  its own image host (https on googleusercontent.com). Apple gives no photo, so initials show instead.
- **Password.** An account made with Google or Apple has no password. It can set one under Account → Sign-in methods
  → Password (players) or the account menu → *Set a password* (staff), within 10 minutes of signing in. A player can
  then also sign in with their phone number on the web, and staff with their email address. If it is refused ("sign in
  again"), they sign out, sign in with Google or Apple again, and go straight back.
- **Phone number.** A Google or Apple sign-up gives the M-Pesa number the player will use. The first top-up's prompt
  goes to that number, and paying it proves the number (ADR 0005, 0020). Nothing changed here.
