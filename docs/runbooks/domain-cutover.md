# Moving to 4draws.co.ke

The product was renamed 4draws on 2026-11-11 and `4draws.co.ke` was registered with Safaricom the same day. This is
the one-time move from `tajirika.co.ke`. The change that carries this file switches every address in the repository;
**merge it only after steps 1 to 3**, or the web app is built against an API address that does not answer yet.

Nothing here touches player data, the ledger or M-Pesa money. The old addresses keep working throughout.

## 1. Render: add the new custom domains (owner, dashboard)

For each service, *Settings → Custom Domains → Add*:

| Service | Add | Keep |
|---|---|---|
| `lottery-web` | `4draws.co.ke`, `www.4draws.co.ke` | `tajirika.co.ke`, `www.tajirika.co.ke` |
| `lottery-api` | `api.4draws.co.ke` | `api.tajirika.co.ke` |
| `lottery-backoffice` | `backoffice.4draws.co.ke` | `backoffice.tajirika.co.ke` |

Render shows the DNS target for each (a CNAME to the service's `onrender.com` name; an A record for the bare domain).

## 2. Safaricom DNS: point the names (owner, domains.safaricom.co.ke)

Create exactly the records Render shows in step 1:

| Name | Type | Value |
|---|---|---|
| `4draws.co.ke` | A (or ALIAS/ANAME) | the address Render gives for `lottery-web` |
| `www` | CNAME | `lottery-web`'s `onrender.com` name |
| `api` | CNAME | `lottery-api`'s `onrender.com` name |
| `backoffice` | CNAME | `lottery-backoffice`'s `onrender.com` name |

## 3. Wait for the certificates, then check

Render issues a certificate for each name once DNS answers (minutes to an hour).

```sh
curl -fsS https://api.4draws.co.ke/readyz
curl -fsSI https://4draws.co.ke/ | head -1
curl -fsSI https://backoffice.4draws.co.ke/ | head -1
```

## 4. Render: allow the new origins (owner, dashboard)

`lottery-api` → Environment → `CORS_ALLOWED_ORIGINS`: add `https://4draws.co.ke` and
`https://backoffice.4draws.co.ke`, **keeping** the tajirika ones for now. Save (the API restarts).

## 5. Merge the change

It sets, in `render.yaml`: `PUBLIC_BASE_URL` (the M-Pesa callback address), the web app's and the back office's
`API_BASE_URL`, the links in mail (`WEB_APP_URL`, `BACKOFFICE_URL`); and in the web app: the canonical address, the
sitemap, robots.txt, the share image and the policy pages' addresses. Render rebuilds all three services.

Then check: sign in at https://4draws.co.ke, make a sandbox top-up (the STK prompt names 4draws and its callback now
arrives at `api.4draws.co.ke`), and open the back office.

## 6. M-Pesa: register the paybill addresses again

STK callbacks follow `PUBLIC_BASE_URL` on their own. The paybill (C2B) validation and confirmation addresses are
registered with Safaricom once, so register them again on the new host:

```sh
go run ./cmd/lotteryctl payments c2b-register   # see docs/runbooks/payments-operations.md
```

Until then paybill payments still arrive on the old host, which keeps working.

## 7. Search engines and sign-in providers

- Google Search Console: add `4draws.co.ke`, submit `https://4draws.co.ke/sitemap.xml`, and use *Change of address*
  from the `tajirika.co.ke` property (`docs/runbooks/web-seo.md`).
- Google sign-in: add `https://4draws.co.ke` and `https://backoffice.4draws.co.ke` to the web clients' authorised
  JavaScript origins. Apple sign-in (when set up): the new domains and return URLs.
- Mail: when a sender on `@4draws.co.ke` exists, change `SMTP_FROM` in the dashboard. Staff accounts keep the email
  addresses they were created with.

## 8. Retire the old domain (later)

Keep `tajirika.co.ke` serving for as long as links and bookmarks exist. When ready, replace it with redirects to the
same path on `4draws.co.ke` (Render: a redirect rule on `lottery-web`; the API and the back office can simply drop the
old custom domain once nothing calls them), then remove the tajirika origins from `CORS_ALLOWED_ORIGINS`. Do not let
the old domain lapse while the paybill addresses of step 6 still point at it.
