package api

import (
	"bytes"
	"html/template"
	"net/http"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	spec "github.com/itsivali/4draws-lottery-platform/api"
	"github.com/itsivali/4draws-lottery-platform/internal/compliance"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/httpx"
)

// The home page (GET /) says what this API is to whoever opens api.4draws.co.ke: what 4draws is, how play and
// fairness work, the API's sections with links to the contract, and whether money is moving right now. Browsers get
// a page; a client that asks for JSON gets the same facts as JSON. It reads only public information.

const (
	siteURL     = "https://4draws.co.ke"
	sourceURL   = "https://github.com/itsivali/4draws-lottery-platform"
	verifierURL = "https://github.com/itsivali/4draws-fairness"
)

type apiSection struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

type apiDoc struct {
	Info struct {
		Title       string `yaml:"title"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
	} `yaml:"info"`
	Tags  []apiSection              `yaml:"tags"`
	Paths map[string]map[string]any `yaml:"paths"`
}

// contract is the embedded OpenAPI document, read once.
var contract = func() apiDoc {
	var d apiDoc
	_ = yaml.Unmarshal(spec.OpenAPI, &d) // the openapi test fails on a document that does not parse
	return d
}()

type homeStatus struct {
	OperatingMode string   `json:"operating_mode"`
	RealMoney     bool     `json:"real_money"`
	MoneyMoves    bool     `json:"money_moves"`
	Paused        []string `json:"paused"`
}

type homeInfo struct {
	Name       string       `json:"name"`
	About      string       `json:"about"`
	Website    string       `json:"website"`
	Contract   string       `json:"contract"`
	BaseURL    string       `json:"base_url"`
	Version    string       `json:"version,omitempty"`
	Status     *homeStatus  `json:"status,omitempty"`
	Sections   []apiSection `json:"sections"`
	Operations int          `json:"operations"`
	Fairness   string       `json:"fairness_verifier"`
	Source     string       `json:"source"`
}

func (h *handlers) homeInfo(r *http.Request) homeInfo {
	info := homeInfo{
		Name:     "4draws API",
		About:    "The API behind 4draws, Kenya's M-Pesa lottery: a spin for a shilling, provably fair draws, prizes paid back to M-Pesa.",
		Website:  siteURL,
		Contract: "/openapi.yaml",
		BaseURL:  "/v1",
		Version:  shortVersion(os.Getenv("RENDER_GIT_COMMIT")),
		Sections: contract.Tags,
		Fairness: verifierURL,
		Source:   sourceURL,
	}
	for _, ops := range contract.Paths {
		for m := range ops {
			switch m {
			case "get", "post", "put", "patch", "delete":
				info.Operations++
			}
		}
	}
	if h.Gate != nil {
		if st, err := h.Gate.Current(r.Context()); err == nil {
			hs := &homeStatus{OperatingMode: string(st.Mode), RealMoney: st.RealMoney, MoneyMoves: st.Mode != compliance.Disabled, Paused: []string{}}
			if sw, err := h.Gate.Switches(r.Context()); err == nil {
				for _, s := range sw {
					if s.Paused {
						hs.Paused = append(hs.Paused, string(s.Area))
					}
				}
			}
			info.Status = hs
		}
	}
	return info
}

func shortVersion(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// wantsJSON is true when the client prefers JSON to HTML (an API client, curl -H 'Accept: application/json').
func wantsJSON(r *http.Request) bool {
	a := r.Header.Get("Accept")
	return strings.Contains(a, "application/json") && !strings.Contains(a, "text/html")
}

func (h *handlers) home(w http.ResponseWriter, r *http.Request) {
	info := h.homeInfo(r)
	if wantsJSON(r) {
		httpx.WriteJSON(w, http.StatusOK, info)
		return
	}
	var b bytes.Buffer
	if err := homePage.Execute(&b, info); err != nil {
		internalError(w, err)
		return
	}
	hd := w.Header()
	// the page is self-contained: inline styles and an inline SVG, nothing fetched
	hd.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(b.Bytes())
}

// openAPI serves the contract itself.
func (h *handlers) openAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(spec.OpenAPI)
}

var homePage = template.Must(template.New("home").Funcs(template.FuncMap{
	"mode": func(s *homeStatus) string {
		switch {
		case s == nil:
			return "Status unavailable"
		case !s.MoneyMoves:
			return "Money operations are stopped"
		case len(s.Paused) > 0:
			return "Running, with some operations paused"
		case s.RealMoney:
			return "Live with real money"
		default:
			return "Running (test money)"
		}
	},
	"ok":   func(s *homeStatus) bool { return s != nil && s.MoneyMoves && len(s.Paused) == 0 },
	"join": func(xs []string) string { return strings.Join(xs, ", ") },
}).Parse(homeHTML))

const homeHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>4draws API</title>
<meta name="description" content="{{.About}}">
<meta name="robots" content="noindex">
<style>
:root{--bg:#F7F7FB;--card:#fff;--ink:#1B1A2E;--muted:#5A5870;--line:#E4E2F0;--primary:#4338CA;--tint:#E4E2FF;--gold:#B47A12;--ok:#1B7A43;--warn:#9A5600}
@media (prefers-color-scheme:dark){:root{--bg:#121120;--card:#1B1A2E;--ink:#EEEDF7;--muted:#A9A7C0;--line:#2E2C46;--primary:#8B83FF;--tint:#2A2763;--ok:#5BD38E;--warn:#F2B35B}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
a{color:var(--primary)}
.hero{background:linear-gradient(135deg,#4338CA,#2A1F8F);color:#fff;padding:48px 20px 56px}
.wrap{max-width:960px;margin:0 auto;padding:0 4px}
.brand{display:flex;align-items:center;gap:12px;font-weight:600;font-size:20px}
.brand svg{width:44px;height:44px;border-radius:12px;background:rgba(255,255,255,.12)}
h1{font-size:clamp(30px,5vw,44px);line-height:1.1;margin:28px 0 12px;letter-spacing:-.02em}
.lede{max-width:60ch;opacity:.9;margin:0 0 24px}
.pill{display:inline-flex;align-items:center;gap:8px;background:rgba(255,255,255,.14);border-radius:999px;padding:6px 14px;font-size:14px}
.dot{width:9px;height:9px;border-radius:50%;background:#FFC53D}.dot.ok{background:#5BD38E}
.links{display:flex;flex-wrap:wrap;gap:10px;margin-top:22px}
.links a{color:#fff;text-decoration:none;border:1px solid rgba(255,255,255,.35);border-radius:999px;padding:8px 16px;font-size:14px}
.links a.primary{background:#fff;color:#4338CA;border-color:#fff;font-weight:600}
main{padding:0 20px 56px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:16px;margin-top:-28px}
.card{background:var(--card);border:1px solid var(--line);border-radius:20px;padding:22px}
.card h2{font-size:17px;margin:0 0 8px}
.card p,.card li{color:var(--muted);margin:0}
ol,ul{padding-left:20px;margin:8px 0 0}
li{margin:4px 0}
h3{font-size:22px;margin:40px 0 12px;letter-spacing:-.01em}
table{width:100%;border-collapse:collapse;background:var(--card);border:1px solid var(--line);border-radius:16px;overflow:hidden}
td{padding:12px 16px;border-top:1px solid var(--line);vertical-align:top}
tr:first-child td{border-top:0}
td:first-child{white-space:nowrap;font-weight:600}
td:last-child{color:var(--muted)}
code{background:var(--tint);border-radius:6px;padding:1px 6px;font-size:.92em}
footer{color:var(--muted);font-size:14px;margin-top:40px;display:flex;flex-wrap:wrap;gap:8px 20px;justify-content:space-between}
</style>
</head>
<body>
<header class="hero"><div class="wrap">
  <div class="brand"><svg viewBox="0 0 64 64" aria-hidden="true"><g fill="none" stroke-linecap="round" stroke-width="4.5"><path d="M36.68 9.99 A22.5 22.5 0 0 1 54.01 27.32" stroke="#FFC53D"/><path d="M54.01 36.68 A22.5 22.5 0 0 1 36.68 54.01" stroke="#fff" stroke-opacity=".92"/><path d="M27.32 54.01 A22.5 22.5 0 0 1 9.99 36.68" stroke="#fff" stroke-opacity=".92"/><path d="M9.99 27.32 A22.5 22.5 0 0 1 27.32 9.99" stroke="#fff" stroke-opacity=".92"/></g><path d="M36.5 44.5 V19.5 L22 38 H43" fill="none" stroke="#fff" stroke-width="5.5" stroke-linecap="round" stroke-linejoin="round"/></svg>4draws API</div>
  <h1>The API behind Kenya's M-Pesa lottery</h1>
  <p class="lede">A spin for a shilling, four draws to win in, and every prize paid back to M-Pesa. This service runs the wallets, plays, draws and payments for the 4draws web, Android and iOS apps and the staff back office.</p>
  <span class="pill"><span class="dot{{if ok .Status}} ok{{end}}"></span>{{mode .Status}}{{with .Status}}{{if .Paused}}: {{join .Paused}} paused{{end}}{{end}}</span>
  <div class="links">
    <a class="primary" href="{{.Website}}">Play at 4draws.co.ke</a>
    <a href="{{.Contract}}">API contract (OpenAPI)</a>
    <a href="{{.Fairness}}">Fairness verifier</a>
    <a href="{{.Source}}">Source code</a>
  </div>
</div></header>
<main><div class="wrap">
  <div class="grid">
    <section class="card"><h2>How it works</h2><ol>
      <li>Sign up with the M-Pesa number you will play with.</li>
      <li>Top up from KES 1 with an M-Pesa prompt on your phone.</li>
      <li>Spin up to four times a day, KES 1 a spin. Every spin enters the daily, weekly, monthly and quarterly draws.</li>
      <li>Prizes land in your wallet and are withdrawn to M-Pesa.</li>
    </ol></section>
    <section class="card"><h2>Fair by design</h2><p>Each spin is committed to before it happens and revealed after. Draws are seeded from the public drand randomness beacon and signed off by two officials before payout. The protocol and a verifier anyone can run are published.</p></section>
    <section class="card"><h2>Money you can trust</h2><p>Balances are kept in a double-entry ledger in whole shillings. M-Pesa callbacks are checked against M-Pesa before anything is credited, and every payout is recorded with its receipt.</p></section>
  </div>

  <h3>Using the API</h3>
  <table>
    <tr><td>Base path</td><td><code>{{.BaseURL}}</code> on this host, JSON over HTTPS.</td></tr>
    <tr><td>Contract</td><td><a href="{{.Contract}}">openapi.yaml</a> (OpenAPI 3.1, {{.Operations}} operations): the source of truth, tested against the router on every change.</td></tr>
    <tr><td>Authentication</td><td>Players sign in for a bearer token; staff use the back office's own sign-in. Public reads (status, draws and their certificates) need no token.</td></tr>
    <tr><td>Writes</td><td>Every POST, PUT, PATCH and DELETE needs an <code>Idempotency-Key</code> header; a retry returns the stored answer.</td></tr>
    <tr><td>Errors</td><td>RFC 7807 <code>application/problem+json</code>.</td></tr>
    <tr><td>Status</td><td><a href="/v1/status">/v1/status</a> (operating mode), <a href="/healthz">/healthz</a> and <a href="/readyz">/readyz</a> (probes).</td></tr>
  </table>

  <h3>Sections</h3>
  <table>{{range .Sections}}
    <tr><td>{{.Name}}</td><td>{{.Description}}</td></tr>{{end}}
  </table>

  <footer>
    <span>18+ only. Play responsibly: deposit limits, breaks and self-exclusion are in the app.</span>
    <span>{{if .Version}}Build {{.Version}}{{end}}</span>
  </footer>
</div></main>
</body>
</html>
`
