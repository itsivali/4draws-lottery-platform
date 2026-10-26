# Back office (staff)

React + TypeScript (Vite) over the `/v1/staff` API (ADR 0013). Staff sign in with the staff realm (OIDC, Authorization
Code + PKCE, `oidc-client-ts`); the app asks the API for the member's roles and shows only what they allow (the API
checks again on every call). Runbook: [`docs/runbooks/back-office.md`](../../docs/runbooks/back-office.md).

```sh
npm ci
cp .env.example .env.local      # local stack: make dev
npm run dev                     # http://localhost:5173
npm run lint && npm run typecheck && npm test && npm run build
```

Every write carries an idempotency key; a lost answer is retried once with the same key and otherwise shown as
*unknown* with *Check again* (same key), never as a failure. Refusals are shown in the API's own words with "Nothing
was changed". Tokens live in the tab's session storage only.
