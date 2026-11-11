package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/lottery-platform/internal/platform/httpx"
)

func corsRouter(origins ...string) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.CORS(origins))
	r.Get("/v1/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	r.Post("/v1/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	return r
}

func corsDo(h http.Handler, method, origin string, extra map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, "/v1/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCORS_AnswersAListedOriginsPreflightWithoutReachingTheHandler(t *testing.T) {
	h := corsRouter("https://play.example.co.ke")
	rec := corsDo(h, http.MethodOptions, "https://play.example.co.ke", map[string]string{
		"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "authorization, idempotency-key, content-type",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d", rec.Code)
	}
	hd := rec.Header()
	if hd.Get("Access-Control-Allow-Origin") != "https://play.example.co.ke" || hd.Get("Vary") == "" {
		t.Fatalf("allow-origin %q vary %q", hd.Get("Access-Control-Allow-Origin"), hd.Get("Vary"))
	}
	for _, want := range []string{"Authorization", "Idempotency-Key", "Content-Type"} {
		if !containsToken(hd.Get("Access-Control-Allow-Headers"), want) {
			t.Errorf("allow-headers %q lacks %s", hd.Get("Access-Control-Allow-Headers"), want)
		}
	}
	if hd.Get("Access-Control-Allow-Credentials") != "" {
		t.Error("credentials must not be allowed: tokens travel in the Authorization header, never in cookies")
	}
}

func TestCORS_ExposesTheHeadersClientsNeedOnRealRequests(t *testing.T) {
	rec := corsDo(corsRouter("https://play.example.co.ke"), http.MethodGet, "https://play.example.co.ke", nil)
	if rec.Code != http.StatusTeapot || rec.Header().Get("Access-Control-Allow-Origin") != "https://play.example.co.ke" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	for _, want := range []string{"Idempotent-Replay", "Retry-After", "X-Certificate-SHA256", "X-File-SHA256", "X-File-Rows"} {
		if !containsToken(rec.Header().Get("Access-Control-Expose-Headers"), want) {
			t.Errorf("expose-headers lacks %s", want)
		}
	}
}

func TestCORS_IgnoresUnlistedOriginsAndIsOffWithoutAList(t *testing.T) {
	for name, h := range map[string]http.Handler{
		"unlisted origin": corsRouter("https://play.example.co.ke"),
		"no list":         corsRouter(),
	} {
		rec := corsDo(h, http.MethodGet, "https://evil.example", nil)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: allowed %q", name, rec.Header().Get("Access-Control-Allow-Origin"))
		}
		pre := corsDo(h, http.MethodOptions, "https://evil.example", map[string]string{"Access-Control-Request-Method": "POST"})
		if pre.Header().Get("Access-Control-Allow-Origin") != "" || pre.Code == http.StatusNoContent {
			t.Errorf("%s: preflight %d %v", name, pre.Code, pre.Header())
		}
	}
	// same-origin and server-to-server requests carry no Origin and pass through untouched
	if rec := corsDo(corsRouter("https://play.example.co.ke"), http.MethodPost, "", nil); rec.Code != http.StatusTeapot {
		t.Fatalf("no origin: %d", rec.Code)
	}
}

func containsToken(list, tok string) bool {
	for part := range strings.SplitSeq(list, ",") {
		if http.CanonicalHeaderKey(strings.TrimSpace(part)) == http.CanonicalHeaderKey(tok) {
			return true
		}
	}
	return false
}
