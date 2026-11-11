package dataexport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"
)

// The readable copy (owner, 2026-11-11: "properly formatted, easy to read, as Google and Microsoft give people their
// data"): one self-contained web page, opened from the email in any browser and printable, with a contents list, a
// summary and a table per kind of record. No scripts and nothing fetched from the network. Dates are East Africa
// Time, money is whole shillings.

var eat = time.FixedZone("EAT", 3*60*60)

type colKind int

const (
	kText  colKind = iota
	kWords         // a code such as "self_exclusion", shown as "Self exclusion"
	kMoney
	kTime
	kDate
	kYesNo
)

type column struct {
	Key   string
	Label string
	Kind  colKind
}

type section struct {
	Key   string
	Title string
	// About says what the records are, in a sentence; Empty what to say when there are none.
	About string
	Empty string
	Cols  []column
}

// the report's sections, in reading order: who you are, your money, your play, your safety settings, your devices
var reportSections = []section{
	{"top_ups", "Top-ups", "Money you paid in from M-Pesa.", "You have made no top-ups.", []column{
		{"at", "When", kTime}, {"amount_requested_kes", "Asked for", kMoney}, {"amount_confirmed_kes", "Confirmed", kMoney},
		{"status", "Status", kWords}, {"channel", "Paid by", kWords}, {"mpesa_receipt", "M-Pesa receipt", kText},
		{"phone_number", "From number", kText}, {"failure_reason", "If it failed, why", kWords}, {"reversed_at", "Reversed", kTime}}},
	{"wallet", "Wallet movements", "Every amount added to or taken from your wallet.", "Your wallet has no movements.", []column{
		{"at", "When", kTime}, {"kind", "What", kWords}, {"amount_kes", "Amount", kMoney}, {"for", "For", kWords}}},
	{"plays", "Plays", "Each spin: its price, the slot the wheel landed on and whether it was lucky.", "You have not played.", []column{
		{"at", "When", kTime}, {"play_date", "Play day", kDate}, {"number_that_day", "Play no.", kText}, {"price_kes", "Price", kMoney},
		{"slot", "Slot", kText}, {"lucky", "Lucky", kYesNo}, {"status", "Status", kWords}}},
	{"draw_entries", "Draw entries", "The draws your plays entered you in.", "You have no draw entries.", []column{
		{"draw", "Draw", kWords}, {"period_start", "From", kDate}, {"period_end", "To", kDate}, {"entry_number", "Entry no.", kText},
		{"from_play_on", "From your play on", kDate}, {"draw_status", "Draw status", kWords}}},
	{"prizes", "Prizes", "What you won in the draws.", "You have won no prizes yet.", []column{
		{"at", "When", kTime}, {"draw", "Draw", kWords}, {"period_start", "From", kDate}, {"period_end", "To", kDate},
		{"rank", "Prize rank", kText}, {"amount_kes", "Amount", kMoney}}},
	{"withdrawals_and_payouts", "Withdrawals and payouts", "Money sent to your M-Pesa: withdrawals you asked for, and prizes.", "Nothing has been paid out to you.", []column{
		{"at", "When", kTime}, {"kind", "What", kWords}, {"gross_kes", "Amount", kMoney}, {"tax_kes", "Tax withheld", kMoney},
		{"net_kes", "Sent to you", kMoney}, {"status", "Status", kWords}, {"mpesa_receipt", "M-Pesa receipt", kText},
		{"phone_number", "To number", kText}, {"settled_at", "Completed", kTime}}},
	{"deposit_limits", "Deposit limits", "The limits you set on your top-ups.", "You have set no deposit limits.", []column{
		{"period", "Period", kWords}, {"amount_kes", "Limit", kMoney}, {"requested_at", "Set on", kTime},
		{"effective_from", "In force from", kTime}, {"cancelled_at", "Replaced on", kTime}}},
	{"breaks_and_exclusions", "Breaks and self-exclusions", "Times you paused or stopped your play.", "You have taken no breaks.", []column{
		{"kind", "What", kWords}, {"starts_at", "From", kTime}, {"ends_at", "Until", kTime}}},
	{"devices", "Devices", "Phones and browsers that have signed in to your account.", "No devices are recorded.", []column{
		{"platform", "Device type", kWords}, {"first_seen_at", "First used", kTime}, {"last_seen_at", "Last used", kTime}}},
	{"sign_ins", "Sign-ins", "Each time your account was signed in to.", "No sign-ins are recorded.", []column{
		{"at", "When", kTime}, {"platform", "On", kWords}, {"ended_at", "Signed out", kTime}}},
	{"sign_in_links", "Google and Apple sign-in", "Accounts you linked for signing in.", "You have linked no Google or Apple account.", []column{
		{"provider", "Provider", kWords}, {"linked_at", "Linked on", kTime}}},
	{"copies_of_this_data", "Earlier copies of your data", "When copies like this one were sent to you before.", "This is your first copy.", []column{
		{"sent_at", "Sent on", kTime}}},
}

type reportFact struct{ Label, Value string }

type reportTable struct {
	ID, Title, About, Empty string
	Head                    []string
	Rows                    [][]string
	Count                   int
}

type reportData struct {
	Operator  string
	Generated string
	Name      string
	Facts     []reportFact
	Tables    []reportTable
	Left      string
}

func words(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), "-", " "))
	if s == "" {
		return ""
	}
	switch strings.ToLower(s) {
	case "stk":
		return "M-Pesa prompt"
	case "c2b":
		return "Paybill"
	case "b2c":
		return "M-Pesa"
	case "kyc":
		return "Identity check"
	case "ios":
		return "iPhone"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func shillings(v any) string {
	f, ok := v.(float64)
	if !ok {
		return ""
	}
	n := int64(f)
	neg := n < 0
	if neg {
		n = -n
	}
	d := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range d {
		if i > 0 && (len(d)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "− KES " + b.String()
	}
	return "KES " + b.String()
}

func when(v any, layout string) string {
	s, ok := v.(string)
	if !ok || s == "" {
		return ""
	}
	for _, l := range []string{time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			if l == "2006-01-02" {
				return t.Format("2 Jan 2006")
			}
			return t.In(eat).Format(layout)
		}
	}
	return s
}

func cell(v any, k colKind) string {
	if v == nil {
		return ""
	}
	switch k {
	case kMoney:
		return shillings(v)
	case kTime:
		return when(v, "2 Jan 2006, 15:04")
	case kDate:
		return when(v, "2 Jan 2006")
	case kYesNo:
		if b, ok := v.(bool); ok {
			if b {
				return "Yes"
			}
			return "No"
		}
		return ""
	case kWords:
		if s, ok := v.(string); ok {
			return words(s)
		}
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// Report renders the file's JSON as the readable page.
func Report(file []byte) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(file, &doc); err != nil {
		return nil, err
	}
	var about, account, ident map[string]any
	_ = json.Unmarshal(doc["about"], &about)
	_ = json.Unmarshal(doc["account"], &account)
	_ = json.Unmarshal(doc["identity"], &ident)
	d := reportData{Operator: Operator, Generated: when(about["generated_at"], "2 Jan 2006, 15:04") + " EAT"}
	if s, _ := about["not_included"].(string); s != "" {
		d.Left = s
	}
	if n, _ := ident["full_name"].(string); n != "" {
		d.Name = n
	}
	add := func(label, value string) {
		if value != "" {
			d.Facts = append(d.Facts, reportFact{label, value})
		}
	}
	add("Name", d.Name)
	add("Phone number", cell(account["phone_number"], kText))
	add("Email address", cell(account["email"], kText))
	if e, _ := ident["id_number_ending"].(string); e != "" {
		add("ID number", "ends in "+e)
	}
	add("Identity check", cell(ident["status"], kWords))
	add("Account status", cell(account["status"], kWords))
	add("Account opened", cell(account["opened_at"], kTime))
	add("Phone number confirmed", cell(account["phone_confirmed_at"], kTime))
	add("Asked to close the account", cell(account["closure_requested_at"], kTime))
	add("Account reference", cell(account["user_id"], kText))

	for _, sec := range reportSections {
		var rows []map[string]any
		_ = json.Unmarshal(doc[sec.Key], &rows)
		t := reportTable{ID: strings.ReplaceAll(sec.Key, "_", "-"), Title: sec.Title, About: sec.About, Empty: sec.Empty, Count: len(rows)}
		// a column nobody has a value in is left out, so the table stays narrow
		for _, c := range sec.Cols {
			used := false
			for _, r := range rows {
				if cell(r[c.Key], c.Kind) != "" {
					used = true
					break
				}
			}
			if used {
				t.Head = append(t.Head, c.Label)
			}
		}
		for _, r := range rows {
			var line []string
			for _, c := range sec.Cols {
				for _, h := range t.Head {
					if h == c.Label {
						line = append(line, cell(r[c.Key], c.Kind))
					}
				}
			}
			t.Rows = append(t.Rows, line)
		}
		d.Tables = append(d.Tables, t)
	}
	var out bytes.Buffer
	if err := reportTemplate.Execute(&out, d); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

var reportTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Your 4draws data</title>
<style>
:root{--ink:#1b1a2e;--muted:#5a5870;--line:#e4e3f0;--bg:#f7f7fb;--card:#fff;--brand:#4338ca;--tint:#e4e2ff}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,Arial,sans-serif}
header{background:linear-gradient(135deg,#5b4fe9,#4338ca 50%,#2a2385);color:#fff;padding:28px 20px}
header div,main{max-width:60rem;margin:0 auto}
header p{margin:0}.brand{font-weight:800;font-size:1.25rem;letter-spacing:-.01em}
h1{margin:.35rem 0 .25rem;font-size:1.7rem;line-height:1.2}.sub{color:#dddbff}
main{padding:20px 16px 48px}
section,nav{background:var(--card);border-radius:16px;padding:18px 20px;margin:0 0 16px;box-shadow:0 1px 4px rgb(20 18 58/.08),0 14px 32px -6px rgb(20 18 58/.10)}
h2{margin:0 0 .15rem;font-size:1.15rem}.about{margin:0 0 .9rem;color:var(--muted)}
.count{display:inline-block;margin-left:.4rem;padding:.05rem .5rem;border-radius:999px;background:var(--tint);color:#211a78;font-size:.8rem;font-weight:600;vertical-align:middle}
dl{display:grid;grid-template-columns:minmax(9rem,14rem) 1fr;gap:.45rem 1rem;margin:0}
dt{color:var(--muted)}dd{margin:0;font-weight:600;overflow-wrap:anywhere}
nav ol{margin:.3rem 0 0;padding-left:1.3rem;columns:2;column-gap:2rem}nav a{color:var(--brand)}
.scroll{overflow-x:auto}
table{border-collapse:collapse;width:100%;font-size:.93rem;font-variant-numeric:tabular-nums}
th{text-align:left;color:var(--muted);font-weight:600;white-space:nowrap;border-bottom:2px solid var(--line);padding:.45rem .7rem .45rem 0}
td{border-bottom:1px solid var(--line);padding:.45rem .7rem .45rem 0;vertical-align:top;white-space:nowrap}
tr:last-child td{border-bottom:0}.none{margin:0;color:var(--muted)}
footer{color:var(--muted);font-size:.88rem;max-width:60rem;margin:0 auto;padding:0 16px 40px}
@media (max-width:36rem){dl{grid-template-columns:1fr;gap:0}dd{margin-bottom:.5rem}nav ol{columns:1}}
@media print{body{background:#fff}header{background:none;color:#000;padding:0 0 12px}.sub{color:#444}section,nav{box-shadow:none;border:1px solid #ccc;break-inside:avoid}td,th{white-space:normal}}
</style>
</head>
<body>
<header><div>
<p class="brand">4draws</p>
<h1>Your data{{if .Name}}, {{.Name}}{{end}}</h1>
<p class="sub">A copy of the personal data {{.Operator}} holds about you. Prepared {{.Generated}}.</p>
</div></header>
<main>
<section id="you">
<h2>You and your account</h2>
<p class="about">Who we have you down as. If anything here is wrong, tell us and we will correct it.</p>
<dl>{{range .Facts}}<dt>{{.Label}}</dt><dd>{{.Value}}</dd>{{end}}</dl>
</section>
<nav aria-label="Contents">
<h2>In this copy</h2>
<ol>{{range .Tables}}<li><a href="#{{.ID}}">{{.Title}}</a> ({{.Count}})</li>{{end}}</ol>
</nav>
{{range .Tables}}<section id="{{.ID}}">
<h2>{{.Title}}<span class="count">{{.Count}}</span></h2>
<p class="about">{{.About}}</p>
{{if .Rows}}<div class="scroll"><table>
<thead><tr>{{range .Head}}<th scope="col">{{.}}</th>{{end}}</tr></thead>
<tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody>
</table></div>{{else}}<p class="none">{{.Empty}}</p>{{end}}
</section>
{{end}}</main>
<footer>
<p><strong>About this copy.</strong> Times are East Africa Time. Amounts are in Kenya shillings. {{.Left}}</p>
<p>The same data is attached to the email as a second file ending in .json, for use by another service or program.</p>
<p>Questions, or something to correct: privacy@4draws.co.ke. {{.Operator}}, Nairobi, Kenya.</p>
</footer>
</body>
</html>
`))
