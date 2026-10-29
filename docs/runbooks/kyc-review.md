# Runbook: manual KYC review

Identities are checked by staff (ADR 0009). A player submits national ID number, full name and date of birth; the
account then waits in `manual_review` and cannot withdraw or be paid a prize until a reviewer decides (since ADR 0022
it may top up and play meanwhile). Under-18s and IDs already verified on another account are refused before they
reach the queue.

## Be told when a submission arrives

Reviewers are told two ways:

- **In the back office**, the sidebar's Identity review count and a banner update every 30 seconds while the tab
  is open (and whenever it regains focus).
- **By email**, the moment a submission starts waiting (a manual submission, or a registry mismatch): the api queues
  a `kyc.review_notice` job with the submission, and the job emails `KYC_REVIEW_NOTIFY_TO` (on Render
  `itsivali@outlook.com`) a link that opens the back office at that submission,
  `${BACKOFFICE_URL}/kyc?user=<account id>`. The email gives the name on the submitted ID (owner, 2026-10-29), when the submission arrived and how
  many are waiting. If the submission was decided before the job ran, nothing is sent. A failed send is retried by
  the queue (8 attempts, backing off).

Email goes through the SMTP server in `SMTP_HOST`, `SMTP_PORT` (587 with STARTTLS, or 465 with TLS), `SMTP_USERNAME`,
`SMTP_PASSWORD` and `SMTP_FROM` (the sender address, which the provider must have verified). Use a transactional
provider rather than a personal mailbox's SMTP (Outlook and Gmail refuse or throttle password sign-ins from servers):

| Provider | `SMTP_HOST` | `SMTP_PORT` | `SMTP_USERNAME` | `SMTP_PASSWORD` |
|---|---|---|---|---|
| Resend | `smtp.resend.com` | `465` | `resend` | an API key |
| Brevo | `smtp-relay.brevo.com` | `587` | the account's SMTP login | its SMTP key |

Set them in the Render dashboard on `lottery-api` (`render.yaml` lists the keys; `SMTP_PASSWORD` is a secret). With
no `SMTP_HOST` the api logs `email not sent (no SMTP_HOST: logged only)` with the recipient and subject, and the
notice goes no further. With no `KYC_REVIEW_NOTIFY_TO` no notice job is queued at all.

The link opens the Identity review page after sign-in, with the submission tinted and marked "From your email". If
it is no longer waiting (decided already, or the player submitted again), the page says so.

## List what is waiting

Listing decrypts personal data, so it needs the api's configuration (copy the api's environment from the Render
dashboard into your shell; never commit it or save it in a file in the repository):

```sh
go run ./cmd/lotteryctl kyc pending -operator <your staff id>
```

Every submission shown is audited as `kyc.unmask` under `staff:<your staff id>`. Show it only when you are about to
review it.

## Decide

Confirm the identity by the operator's procedure (owner to define: for example a photo of the ID card sent by the
player, checked against the submitted number, name and date of birth). Then:

```sh
go run ./cmd/lotteryctl kyc approve -user <user id> -reason "<how it was checked>" -operator <your staff id>
go run ./cmd/lotteryctl kyc reject  -user <user id> -reason "<why>" -operator <your staff id>
```

Approve and reject need no keys. A reason is required and is stored with the decision. A rejected player may submit
again. Approval fails if the national ID was meanwhile verified for another account: reject instead.
