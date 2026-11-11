# 0035. The Google account's photo is shown; Google and Apple accounts may set a password right after signing in

Status: accepted (amends 0019, 0020)
Date: 2026-11-11

## Context
The owner asked on 2026-11-09 for two things:

- a player or staff member who signs in with Google sees that account's photo;
- someone who signed up with Google or Apple can set a password, for when they do not want to sign in to the back
  office or the web player app with Google.

The third point of the request was already in place (ADR 0020): a Google or Apple sign-up gives an M-Pesa number, and
the first top-up proves it.

Player accounts made with Google or Apple had no password and no way to set one, because changing a password needs the
current one. Staff could set a first password at any time while signed in. That meant anyone holding a stolen session
could quietly add a password and keep access after the session was ended.

## Decision
- **The photo is a Google-hosted link, and only that.** At each Google sign-in (players: sign-in, sign-up and linking;
  staff: sign-in) the verified ID token's `picture` claim is kept, as long as it is an https URL on
  `*.googleusercontent.com` and no longer than 1,600 characters. A different link clears it, and Apple changes nothing.
  - **Where it is stored:** `identity.users.photo_url` and `staff.members.photo_url` (migration 00054), each with a
    `CHECK` on the same rule.
  - **Where it is returned:** `GET /v1/account` and `GET /v1/staff/me`.
  - **How it is shown:** the app and the back office load it straight from Google (`referrerPolicy=no-referrer` in the
    back office), over the initials, which show if the photo fails to load. Neither client accepts a URL from another
    host either.
- **A first password must follow a fresh sign-in.**
  - **Players:** `POST /v1/auth/password/set` works only for an account without a password (409 `password-exists`)
    and only within 10 minutes of the session's sign-in (`identity.FreshSignIn`; 403 `sign-in-again`).
  - **Staff:** `PUT /v1/staff/password` without a current password has the same 10-minute rule (`staff.FreshSignIn`).
  - **In both cases** every other session ends and the change is audited (`auth.password_set`,
    `staff.password_changed`).
  - **Clients:** the app's Sign-in methods screen and the back office's account menu offer *Set a password* when there
    is none.

## Consequences
- **Privacy.** The photo is personal data, kept as Google serves it. The URL points at Google, which sees the request
  when the image loads: the player's or member's own device asks Google for their own photo. The privacy data
  inventory (S8) lists it.
- **Fresh sign-in.** A player who wants a password more than 10 minutes after signing in signs in again first. The
  screens say so.
