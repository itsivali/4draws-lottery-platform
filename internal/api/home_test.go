package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	spec "github.com/itsivali/4draws-lottery-platform/api"
	"github.com/itsivali/4draws-lottery-platform/internal/api"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/httpx"
)

func get(t *testing.T, path, accept string) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("RENDER_GIT_COMMIT", "0123456789abcdef")
	h := httpx.NewRouterWith(nil, api.Mount(api.Deps{}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Opening api.4draws.co.ke in a browser shows what the API is, not a bare 404.
func TestHome_ABrowserGetsAPageAboutTheAPI(t *testing.T) {
	rec := get(t, "/", "text/html,application/xhtml+xml,*/*;q=0.8")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, want := range []string{"4draws API", "https://4draws.co.ke", `href="/openapi.yaml"`, "Idempotency-Key", "Fair by design",
		"payments", "M-Pesa top-ups", "Build 0123456", "Status unavailable", "18+"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not mention %q", want)
		}
	}
	// still locked down: inline styles only, nothing loaded from elsewhere, never framed
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "style-src 'unsafe-inline'") ||
		strings.Contains(csp, "script-src") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP %q", csp)
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("the security headers were dropped")
	}
}

// An API client asking for JSON gets the same facts as data.
func TestHome_AClientAskingForJSONGetsTheFactsAsJSON(t *testing.T) {
	rec := get(t, "/", "application/json")
	var got struct {
		Name       string    `json:"name"`
		Contract   string    `json:"contract"`
		BaseURL    string    `json:"base_url"`
		Version    string    `json:"version"`
		Operations int       `json:"operations"`
		Status     *struct{} `json:"status"`
		Sections   []struct {
			Name string `json:"name"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s %v", rec.Code, rec.Body, err)
	}
	if got.Name != "4draws API" || got.Contract != "/openapi.yaml" || got.BaseURL != "/v1" || got.Version != "0123456" ||
		got.Operations < 50 || len(got.Sections) < 8 || got.Status != nil {
		t.Fatalf("%+v", got)
	}
}

// The contract the router is tested against is the one served, byte for byte.
func TestOpenAPI_TheContractIsServedAsWritten(t *testing.T) {
	rec := get(t, "/openapi.yaml", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/yaml") ||
		!bytes.Equal(rec.Body.Bytes(), spec.OpenAPI) || !bytes.HasPrefix(spec.OpenAPI, []byte("openapi: 3.1")) {
		t.Fatalf("status %d type %q, %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
}
