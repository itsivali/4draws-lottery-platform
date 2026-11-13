package mail_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

func notice() mail.Notice {
	return mail.Notice{
		Preheader:  "One tap confirms that this address is yours.",
		Heading:    "Confirm your email address",
		Intro:      []string{"This address was given for a 4draws account."},
		Action:     &mail.Action{Label: "Confirm my email address", URL: "https://4draws.co.ke/verify-email?token=abc_DEF-123"},
		ActionNote: "The link works for 24 hours.",
		Steps:      []string{"Open the link.", "Sign in."},
		Details:    []mail.Detail{{Label: "Asked for", Value: "13 Nov 2026"}},
		Outro:      []string{"Then Google can sign you in."},
		Safety:     "Do not open the link.",
		Reason:     "You are receiving this email because this address was entered on a 4draws account.",
		SiteURL:    "https://4draws.co.ke/",
	}
}

func TestNotice_SaysTheSameThingAsTextAndAsABrandedPage(t *testing.T) {
	n := notice()
	text := n.Text()
	html, err := n.HTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Confirm your email address", "This address was given for a 4draws account.", "https://4draws.co.ke/verify-email?token=abc_DEF-123",
		"The link works for 24 hours.", "Open the link.", "Asked for", "13 Nov 2026", "Then Google can sign you in.", "Do not open the link.",
		"support@4draws.co.ke", "https://4draws.co.ke/privacy", "https://4draws.co.ke/terms", "https://4draws.co.ke/responsible-gambling",
		"adults aged 18 and over", "You are receiving this email because", "4 DRAWS TICKET LTD"} {
		if !strings.Contains(text, want) {
			t.Errorf("the text lacks %q", want)
		}
		if !strings.Contains(html, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// the page shows the logo from inside the message and fetches nothing from the web: no remote image, no script
	if !strings.Contains(html, `<img src="cid:logo.png"`) || !strings.Contains(html, `alt="4draws"`) {
		t.Error("the page does not show the logo")
	}
	if strings.Contains(html, "<script") || strings.Contains(html, `src="http`) {
		t.Error("the page carries a script or a remote image")
	}
	if !strings.Contains(html, "One tap confirms that this address is yours.") {
		t.Error("the page lacks the inbox preview line")
	}
	if strings.Contains(text, "<") {
		t.Errorf("the text carries markup:\n%s", text)
	}
}

func TestNotice_EscapesWhatItIsGivenAndLeavesOutWhatIsEmpty(t *testing.T) {
	html, err := mail.Notice{Heading: `<b>Hi</b> & "you"`, Intro: []string{`<script>alert(1)</script>`},
		Action: &mail.Action{Label: "Go", URL: "javascript:alert(1)"}}.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>alert") || strings.Contains(html, "<b>Hi</b>") || strings.Contains(html, `href="javascript:`) {
		t.Fatalf("unescaped:\n%s", html)
	}
	for _, absent := range []string{"Was this not you?", "/privacy", "You are receiving"} {
		if strings.Contains(html, absent) {
			t.Errorf("an empty part is shown: %q", absent)
		}
	}
	// an email to staff names the back office, not the player pages
	staff := mail.Notice{Heading: "Reset", Staff: true, SiteURL: "https://4draws.co.ke"}
	sh, _ := staff.HTML()
	if !strings.Contains(sh, "back office") || strings.Contains(sh, "/responsible-gambling") || strings.Contains(staff.Text(), "Play responsibly") {
		t.Error("the staff footer is the players' one")
	}
}

func TestRenderMessage_NestsTextPageLogoAndFiles(t *testing.T) {
	at := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	m := notice().Message([]string{"jane@example.com"}, "4draws: confirm your email address")
	if m.HTML == "" || len(m.Inline) != 1 || m.Inline[0].Name != "logo.png" || len(m.Inline[0].Data) < 1000 {
		t.Fatalf("message: html %d bytes, inline %+v", len(m.HTML), len(m.Inline))
	}
	got, err := mail.RenderMessage("4draws <noreply@example.com>", m.To, m, at)
	if err != nil {
		t.Fatal(err)
	}
	// related(alternative(text, html), logo): in that order, the page last among the alternatives so it is preferred
	order := []string{"Content-Type: multipart/related;", "Content-Type: multipart/alternative;", "Content-Type: text/plain; charset=utf-8",
		"Content-Type: text/html; charset=utf-8", "Content-Type: image/png; name=\"logo.png\"", "Content-ID: <logo.png>", "Content-Disposition: inline;"}
	at0 := 0
	for _, want := range order {
		i := strings.Index(got[at0:], want)
		if i < 0 {
			t.Fatalf("missing or out of order: %q in\n%.1500s", want, got)
		}
		at0 += i
	}
	if strings.Contains(got, "multipart/mixed") {
		t.Fatal("mixed without a file")
	}
	// the page comes back whole from its base64, no line longer than 76
	body := got[strings.Index(got, "Content-Type: text/html"):]
	body = body[strings.Index(body, "\r\n\r\n")+4:]
	body = body[:strings.Index(body, "\r\n--")]
	for _, line := range strings.Split(body, "\r\n") {
		if len(line) > 76 {
			t.Fatalf("a line of %d characters", len(line))
		}
	}
	if back, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", "")); err != nil || string(back) != m.HTML {
		t.Fatalf("the page did not survive: %v", err)
	}
	// with a file, all of that is the first part of a mixed message
	m.Attachments = []mail.Attachment{{Name: "data.json", ContentType: "application/json", Data: []byte(`{}`)}}
	got, err = mail.RenderMessage("a@b.c", m.To, m, at)
	if err != nil {
		t.Fatal(err)
	}
	mixed, related, file := strings.Index(got, "multipart/mixed"), strings.Index(got, "multipart/related"), strings.Index(got, `filename="data.json"`)
	if mixed < 0 || related < mixed || file < related {
		t.Fatalf("mixed %d, related %d, file %d", mixed, related, file)
	}
	// an inline image with a name that could break its header is refused
	m.Inline[0].Name = "a\r\nBcc: x@y.z"
	if _, err := mail.RenderMessage("a@b.c", m.To, m, at); err == nil {
		t.Fatal("a bad inline name was accepted")
	}
}
