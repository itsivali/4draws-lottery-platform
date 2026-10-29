# Runbook: the web app in search

The player web app (`apps/mobile`, Flutter, served by Render at https://tajirika.co.ke) is a single page: crawlers get
`index.html` and little else, so everything search engines and link previews use lives in `apps/mobile/web/`.

## What is where

- **`index.html`:** the `<head>` carries the title, description, keywords, canonical and hreflang links, Open Graph and
  Twitter card tags, the theme colour, the manifest and icon links, a preconnect to the API, and JSON-LD structured
  data (`Organization`, `WebSite`, `WebApplication` with the KES 1 price, and a `FAQPage`). The `<body>` carries a
  **landing** (`#landing`): the headline, how it works, fairness, responsible play and the questions, in plain HTML,
  which is what the root address shows: the app is not started at `/` (Google's renderer runs JavaScript, and an app
  drawn on a canvas has no text to index). Every other path starts the app, and "Open Tajirika" leads to `/sign-in`;
  the installed app opens at `/home` (`manifest.json`'s `start_url`). Keep the landing's facts in step with the
  product (price, plays a day, draws, where prizes go): it is the one page search engines read.
- **`og-image.png`:** the 1200x630 share image, rendered by `scripts/gen-icons.sh` from
  `design-system/lottery-platform/logo/tajirika-og.svg`.
- **`robots.txt`** (allow all but the reset pages) and **`sitemap.xml`** (the root and `/sign-in`).
- **`manifest.json`:** the installable app's name, description, colours and categories.
- **Addresses:** the app uses path URLs (`/sign-in`, `/reset-password?token=…`), not `/#/…`, since 2026-10-30
  (`usePathUrlStrategy` in `main.dart`; Render rewrites every path to `index.html`). Each screen sets the document
  title ("Wallet · Tajirika") through `PageScaffold`.
- **The back office** (https://backoffice.tajirika.co.ke) stays out of search: `<meta name="robots" content="noindex">`,
  `public/robots.txt` disallows everything, and Render sends `X-Robots-Tag: noindex, nofollow`.

## After a change

- Changed the landing or the tags: a normal deploy; then **Google Search Console → URL inspection → Request indexing**
  for `https://tajirika.co.ke/`. (Search Console needs the site verified once: add the `google-site-verification`
  meta tag it gives you to `index.html`, or the DNS record at Safaricom.)
- Changed the share image: share the link once in WhatsApp or X to see the new card; Facebook's and LinkedIn's
  debuggers refresh their cache on request.
- Added a public page: add it to `sitemap.xml`.

## What to expect

Only the landing is indexable; every other screen is behind sign-in and should stay that way. Ranking for "lottery
Kenya" and "M-Pesa lottery" depends on links and reputation more than on tags; the tags make sure what appears is
right and the share cards look like the brand.
