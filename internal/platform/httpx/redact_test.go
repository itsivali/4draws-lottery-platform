package httpx_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsivali/lottery-platform/internal/platform/httpx"
)

func TestRedactPath_HidesTheHookSecretAndLeavesOtherPathsAlone(t *testing.T) {
	for in, want := range map[string]string{
		"/hooks/mpesa/stk/s3cr3t":               "/hooks/mpesa/stk/{secret}",
		"/hooks/c2b/s3cr3t/validate":            "/hooks/c2b/{secret}/validate",
		"/hooks/c2b/s3cr3t/status-result":       "/hooks/c2b/{secret}/status-result",
		"/hooks/b2c/s3cr3t/timeout":             "/hooks/b2c/{secret}/timeout",
		"/hooks/b2c/s3cr3t/status-timeout/":     "/hooks/b2c/{secret}/status-timeout/",
		"/hooks/c2b/validate/confirm":           "/hooks/c2b/{secret}/confirm", // the secret position is redacted even if it looks like a word
		"/hooks/mpesa/stk/s3cr3t/anything/else": "/hooks/mpesa/stk/{secret}/{secret}/{secret}",
		"/hooks/unknown/s3cr3t":                 "/hooks/{secret}/{secret}",
		"/v1/topups/abc":                        "/v1/topups/abc",
		"/healthz":                              "/healthz",
		"/":                                     "/",
	} {
		if got := httpx.RedactPath(in); got != want {
			t.Errorf("RedactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogClientIPHeaders_NeverLogsTheHookSecret(t *testing.T) {
	var buf bytes.Buffer
	h := httpx.LogClientIPHeaders(slog.New(slog.NewJSONHandler(&buf, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/mpesa/stk/s3cr3t", http.NoBody))
	if out := buf.String(); strings.Contains(out, "s3cr3t") || !strings.Contains(out, `"path":"/hooks/mpesa/stk/{secret}"`) {
		t.Fatalf("log = %s", out)
	}
}
