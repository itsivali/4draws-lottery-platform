package mail

import (
	_ "embed"
	"html/template"
	"strconv"
	"strings"
)

// logoPNG is the app's logo at 144 px (shown at 48), rendered by scripts/gen-icons.sh from the brand mark. It rides
// inside each branded email, so it shows without the mail program fetching anything from the web.
//
//go:embed logo.png
var logoPNG []byte

// logoName is the logo's file name and the id the page refers to it by (cid:logo.png).
const logoName = "logo.png"

// Operator is the company named at the foot of every branded email.
const Operator = "4 DRAWS TICKET LTD"

// SupportAddress is where a reader is told to write for help.
const SupportAddress = "support@4draws.co.ke"

// Notice is the content of one branded email: the same words rendered twice, as plain text and as a page in the
// 4draws layout (the logo on the brand's indigo, a heading, the message, one button, facts, a safety note and a
// footer naming the company). Every field but Heading is optional; what is empty is left out.
type Notice struct {
	// Preheader is the line mail programs show beside the subject in the inbox list.
	Preheader string
	// Heading is the message in a few words.
	Heading string
	// Intro is the paragraphs before the button.
	Intro []string
	// Action is the one thing the reader is asked to do.
	Action *Action
	// ActionNote sits under the button: how long the link works, that it works once.
	ActionNote string
	// Steps is a numbered list (what the attached files are, what to do next).
	Steps []string
	// Details are facts shown as a small table (label, value).
	Details []Detail
	// Outro is the paragraphs after the button and the facts.
	Outro []string
	// Safety is what to do if the reader did not ask for this email.
	Safety string
	// Reason says why this email was sent ("You are receiving this email because …").
	Reason string
	// Staff marks an email to the operator's own staff: the footer names the back office instead of the player
	// pages.
	Staff bool
	// SiteURL is the player web app's origin (https://4draws.co.ke), for the footer's links; empty leaves them out.
	SiteURL string
}

// Action is a button: its words and where it leads.
type Action struct{ Label, URL string }

// Detail is one fact: a label and its value.
type Detail struct{ Label, Value string }

// Message is the notice as an email to send: the text, the page and the logo the page shows.
func (n Notice) Message(to []string, subject string) Message {
	m := Message{To: to, Subject: subject, Text: n.Text()}
	if html, err := n.HTML(); err == nil {
		m.HTML = html
		m.Inline = []Attachment{{Name: logoName, ContentType: "image/png", Data: logoPNG}}
	}
	return m
}

// Text is the notice in plain text, for mail programs that show no HTML and for the reader who prefers it.
func (n Notice) Text() string {
	var b strings.Builder
	para := func(lines ...string) {
		for _, l := range lines {
			if l != "" {
				b.WriteString(l + "\n\n")
			}
		}
	}
	para(n.Heading)
	para(n.Intro...)
	if n.Action != nil {
		para(n.Action.Label + ":\n" + n.Action.URL)
	}
	para(n.ActionNote)
	for i, s := range n.Steps {
		b.WriteString(strconv.Itoa(i+1) + ". " + s + "\n")
	}
	if len(n.Steps) > 0 {
		b.WriteString("\n")
	}
	for _, d := range n.Details {
		b.WriteString(d.Label + ": " + d.Value + "\n")
	}
	if len(n.Details) > 0 {
		b.WriteString("\n")
	}
	para(n.Outro...)
	para(n.Safety)
	b.WriteString("--\n")
	if n.Staff {
		b.WriteString("4draws back office. This message is for staff of " + Operator + ".\n")
	} else {
		b.WriteString("Help: " + SupportAddress + "\n")
		if site := n.site(); site != "" {
			b.WriteString("Privacy: " + site + "/privacy\nTerms: " + site + "/terms\nResponsible gambling: " + site + "/responsible-gambling\n")
		}
		b.WriteString("4draws is for adults aged 18 and over. Play responsibly.\n")
	}
	if n.Reason != "" {
		b.WriteString(n.Reason + "\n")
	}
	b.WriteString(Operator + ", Kenya\n")
	return b.String()
}

// HTML is the notice as a page. Every value is escaped; the layout is tables with inline styles, which is what mail
// programs render alike.
func (n Notice) HTML() (string, error) {
	var b strings.Builder
	err := noticePage.Execute(&b, struct {
		Notice
		Logo, Site, Operator, Support string
	}{n, logoName, n.site(), Operator, SupportAddress})
	return b.String(), err
}

func (n Notice) site() string { return strings.TrimRight(n.SiteURL, "/") }

var noticePage = template.Must(template.New("notice").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>{{.Heading}}</title>
<style>
  body { margin: 0; padding: 0; background: #F3F2FB; }
  a { color: #4338CA; }
  @media (max-width: 620px) {
    .card { width: 100% !important; border-radius: 0 !important; }
    .pad { padding-left: 24px !important; padding-right: 24px !important; }
    .button a { display: block !important; }
  }
  @media (prefers-color-scheme: dark) {
    body, .page { background: #14132B !important; }
    .card { background: #1E1D3A !important; border-color: #34325C !important; }
    .ink { color: #F2F1FF !important; }
    .soft, .foot { color: #B9B7DA !important; }
    .panel { background: #28264A !important; border-color: #3A3866 !important; }
    .rule { border-color: #34325C !important; }
    a { color: #A5A0FF !important; }
    .button a { color: #FFFFFF !important; }
  }
</style>
</head>
<body>
{{if .Preheader}}<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;mso-hide:all;">{{.Preheader}}</div>{{end}}
<table role="presentation" class="page" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#F3F2FB;">
<tr><td align="center" style="padding:32px 12px;">

<table role="presentation" class="card" width="600" cellpadding="0" cellspacing="0" border="0" style="width:600px;max-width:600px;background:#FFFFFF;border:1px solid #E4E2F5;border-radius:20px;overflow:hidden;">

  <!-- the brand: the logo and the name on the brand's indigo, with the gold of the logo's fourth segment beneath -->
  <tr><td class="pad" bgcolor="#4338CA" style="background:#4338CA;background-image:linear-gradient(135deg,#5B4FE9 0%,#4338CA 55%,#2A2385 100%);padding:28px 40px;">
    <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
      <td style="padding-right:14px;"><img src="cid:{{.Logo}}" width="48" height="48" alt="4draws" style="display:block;width:48px;height:48px;border:0;border-radius:12px;"></td>
      <td style="font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:26px;line-height:32px;font-weight:800;letter-spacing:-0.4px;color:#FFFFFF;">4draws</td>
    </tr></table>
  </td></tr>
  <tr><td height="4" bgcolor="#FFC53D" style="background:#FFC53D;height:4px;line-height:4px;font-size:0;">&nbsp;</td></tr>

  <!-- the message -->
  <tr><td class="pad" style="padding:36px 40px 8px 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <h1 class="ink" style="margin:0 0 16px 0;font-size:24px;line-height:32px;font-weight:700;letter-spacing:-0.2px;color:#17163A;">{{.Heading}}</h1>
    {{range .Intro}}<p class="ink" style="margin:0 0 16px 0;font-size:16px;line-height:26px;color:#33325A;">{{.}}</p>
    {{end}}
  </td></tr>

  {{with .Action}}
  <tr><td class="pad" style="padding:8px 40px 8px 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <table role="presentation" class="button" cellpadding="0" cellspacing="0" border="0"><tr>
      <td align="center" bgcolor="#4338CA" style="background:#4338CA;border-radius:999px;">
        <a href="{{.URL}}" target="_blank" style="display:inline-block;padding:15px 36px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:16px;line-height:20px;font-weight:700;color:#FFFFFF;text-decoration:none;border-radius:999px;">{{.Label}}</a>
      </td>
    </tr></table>
  </td></tr>
  {{end}}

  {{if .ActionNote}}
  <tr><td class="pad" style="padding:8px 40px 0 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <p class="soft" style="margin:0;font-size:14px;line-height:22px;color:#6B6A8C;">{{.ActionNote}}</p>
  </td></tr>
  {{end}}

  {{with .Action}}
  <tr><td class="pad" style="padding:20px 40px 0 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <table role="presentation" class="panel" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#F7F6FF;border:1px solid #E4E2F5;border-radius:12px;"><tr>
      <td style="padding:14px 16px;">
        <p class="soft" style="margin:0 0 6px 0;font-size:13px;line-height:18px;color:#6B6A8C;">If the button does not open, copy this address into your browser:</p>
        <p style="margin:0;font-family:Consolas,Menlo,'Courier New',monospace;font-size:13px;line-height:20px;word-break:break-all;"><a href="{{.URL}}" target="_blank" style="color:#4338CA;text-decoration:none;">{{.URL}}</a></p>
      </td>
    </tr></table>
  </td></tr>
  {{end}}

  {{if .Steps}}
  <tr><td class="pad" style="padding:20px 40px 0 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
      {{range $i, $s := .Steps}}<tr>
        <td valign="top" width="40" style="padding:0 0 14px 0;"><div style="width:28px;height:28px;border-radius:14px;background:#4338CA;color:#FFFFFF;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:14px;line-height:28px;font-weight:700;text-align:center;">{{inc $i}}</div></td>
        <td valign="top" class="ink" style="padding:2px 0 14px 0;font-size:16px;line-height:24px;color:#33325A;">{{$s}}</td>
      </tr>{{end}}
    </table>
  </td></tr>
  {{end}}

  {{if .Details}}
  <tr><td class="pad" style="padding:20px 40px 0 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <table role="presentation" class="panel" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#F7F6FF;border:1px solid #E4E2F5;border-radius:12px;">
      {{range .Details}}<tr>
        <td class="soft" width="40%" valign="top" style="padding:12px 16px;font-size:14px;line-height:20px;color:#6B6A8C;">{{.Label}}</td>
        <td class="ink" valign="top" style="padding:12px 16px;font-size:14px;line-height:20px;font-weight:600;color:#17163A;">{{.Value}}</td>
      </tr>{{end}}
    </table>
  </td></tr>
  {{end}}

  {{if .Outro}}
  <tr><td class="pad" style="padding:20px 40px 0 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    {{range .Outro}}<p class="ink" style="margin:0 0 16px 0;font-size:16px;line-height:26px;color:#33325A;">{{.}}</p>
    {{end}}
  </td></tr>
  {{end}}

  {{if .Safety}}
  <tr><td class="pad" style="padding:12px 40px 0 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <table role="presentation" class="panel" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#FFF8E6;border:1px solid #FFE08A;border-radius:12px;"><tr>
      <td style="padding:14px 16px;">
        <p class="ink" style="margin:0 0 4px 0;font-size:14px;line-height:20px;font-weight:700;color:#17163A;">Was this not you?</p>
        <p class="ink" style="margin:0;font-size:14px;line-height:22px;color:#33325A;">{{.Safety}}</p>
      </td>
    </tr></table>
  </td></tr>
  {{end}}

  <!-- the footer -->
  <tr><td class="pad" style="padding:32px 40px 0 40px;"><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr><td class="rule" style="border-top:1px solid #E4E2F5;font-size:0;line-height:0;">&nbsp;</td></tr></table></td></tr>
  <tr><td class="pad foot" style="padding:20px 40px 32px 40px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;line-height:21px;color:#6B6A8C;">
    {{if .Staff}}
    <p style="margin:0 0 10px 0;">4draws back office. This message is for staff of {{.Operator}}.</p>
    {{else}}
    <p style="margin:0 0 10px 0;">Need help? Write to <a href="mailto:{{.Support}}" style="color:#4338CA;text-decoration:none;">{{.Support}}</a>.</p>
    {{if .Site}}<p style="margin:0 0 10px 0;">
      <a href="{{.Site}}/privacy" target="_blank" style="color:#4338CA;text-decoration:none;">Privacy</a> &nbsp;·&nbsp;
      <a href="{{.Site}}/terms" target="_blank" style="color:#4338CA;text-decoration:none;">Terms</a> &nbsp;·&nbsp;
      <a href="{{.Site}}/responsible-gambling" target="_blank" style="color:#4338CA;text-decoration:none;">Responsible gambling</a>
    </p>{{end}}
    <p style="margin:0 0 10px 0;"><strong>18+</strong> &nbsp;4draws is for adults aged 18 and over. Play responsibly.</p>
    {{end}}
    {{if .Reason}}<p style="margin:0 0 10px 0;">{{.Reason}}</p>{{end}}
    <p style="margin:0;">{{.Operator}}, Kenya</p>
  </td></tr>

</table>

</td></tr>
</table>
</body>
</html>
`))
