package apihttp

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Squarenix17/gesetzeswache/internal/clienterr"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/service"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/sync"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func testServer(t *testing.T, secret string) (*Server, *httpmock.Transport) {
	t.Helper()
	reg := metrics.NewRegistry()
	metrics.RegisterDefaults(reg)
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{
		FreshnessMaxAge: 6 * time.Hour,
		EnableExport:    true,
		ExportCacheMax:  8,
		MatchThreshold:  0.75,
		GIIBase:         "https://www.gesetze-im-internet.de",
		RequestMinGap:   time.Millisecond,
		HTTPTimeout:     5 * time.Second,
	}
	mt := httpmock.New()
	httpClient := httpx.NewWithTransport(cfg.HTTPTimeout, cfg.RequestMinGap, 1<<20, mt)
	httpClient.Metrics = reg
	eng := search.NewEngine()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	orch := &sync.Orchestrator{CFG: cfg, Store: st, HTTP: httpClient, Search: eng, Log: log, Metrics: reg}
	svc := &service.Service{
		CFG:     cfg,
		Store:   st,
		Search:  eng,
		Sync:    orch,
		HTTP:    httpClient,
		Export:  export.NewCache(8),
		Log:     log,
		Metrics: reg,
	}
	return &Server{Svc: svc, SharedSecret: secret, Metrics: reg, Log: log}, mt
}

func longQuery(n int) string {
	return strings.Repeat("ä", n)
}

func TestResolve_QueryTooLong(t *testing.T) {
	srv, _ := testServer(t, "")
	q := longQuery(513)
	req := httptest.NewRequest(http.MethodGet, "/v1/resolve?q="+q, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d want 400", rec.Code)
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || *env.Error != "query too long" {
		t.Fatalf("error %v", env.Error)
	}
}

func TestResolve_NormalQueryUnaffected(t *testing.T) {
	srv, _ := testServer(t, "")
	q := longQuery(512)
	if utf8.RuneCountInString(q) != 512 {
		t.Fatal("test setup")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/resolve?q="+q, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("512-rune query should not be rejected")
	}
}

func TestAuthorize(t *testing.T) {
	const secret = "super-secret-token"

	cases := []struct {
		name       string
		secret     string
		token      string
		headerName string
		want       int
	}{
		{name: "empty secret", secret: "", token: secret, want: 401},
		{name: "wrong token", secret: secret, token: "wrong", want: 401},
		{name: "correct token", secret: secret, token: secret, want: 200},
		{name: "different length token", secret: secret, token: secret + "x", want: 401},
		{name: "header case insensitive", secret: secret, token: secret, headerName: "x-gesetzeswache-token", want: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := testServer(t, tc.secret)
			req := httptest.NewRequest(http.MethodPost, "/v1/recheck", nil)
			h := "X-Gesetzeswache-Token"
			if tc.headerName != "" {
				h = tc.headerName
			}
			req.Header.Set(h, tc.token)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status %d want %d body %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestInternalErrorMasked(t *testing.T) {
	srv, _ := testServer(t, "")
	_ = srv.Svc.Store.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/stale", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || *env.Error != clienterr.Internal {
		t.Fatalf("error %v want %q", env.Error, clienterr.Internal)
	}
	if strings.Contains(rec.Body.String(), "bbolt") || strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("leaked internal error: %s", rec.Body.String())
	}
}

func TestMiddleware_SecurityHeaders(t *testing.T) {
	srv, _ := testServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "trace-abc")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff=%q", got)
	}
	if got := rec.Header().Get("X-Request-ID"); got != "trace-abc" {
		t.Fatalf("request id=%q", got)
	}
}

func TestResolve_PostQueryStringDrainsBody(t *testing.T) {
	srv, _ := testServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve?q=ArbZG", strings.NewReader("{not-json"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusBadRequest && strings.Contains(rec.Body.String(), "invalid json") {
		t.Fatalf("body should be drained when q is in query string, got %s", rec.Body.String())
	}
}

func TestRecheck_UnknownLawID404NoOutbound(t *testing.T) {
	srv, mt := testServer(t, "sekret")
	req := httptest.NewRequest(http.MethodPost, "/v1/recheck?id=unknown-law-xyz", nil)
	req.Header.Set("X-Gesetzeswache-Token", "sekret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if len(mt.Hits()) != 0 {
		t.Fatalf("outbound requests fired: %v", mt.Hits())
	}
}

func TestRecheck_TimesOut504(t *testing.T) {
	srv, mt := testServer(t, "sekret")
	// Handler recheck bound is 4*CFG.HTTPTimeout; client timeout must exceed that so blocking waits on recheck ctx.
	srv.Svc.CFG.HTTPTimeout = 25 * time.Millisecond
	srv.Svc.CFG.GIIFeedURL = "https://www.gesetze-im-internet.de/aktuDienst-rss-feed.xml"
	srv.Svc.CFG.BGBlFeed1URL = "https://www.recht.bund.de/rss/feeds/rss_bgbl-1.xml"
	srv.Svc.CFG.BGBlFeed2URL = "https://www.recht.bund.de/rss/feeds/rss_bgbl-2.xml"
	srv.Svc.Sync.CFG = srv.Svc.CFG
	httpClient := httpx.NewWithTransport(2*time.Second, time.Millisecond, 1<<20, mt)
	srv.Svc.HTTP = httpClient
	srv.Svc.Sync.HTTP = httpClient
	block := httpmock.Response{BlockUntilContext: true}
	mt.Set("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", block)
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", block)
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", block)

	req := httptest.NewRequest(http.MethodPost, "/v1/recheck", nil)
	req.Header.Set("X-Gesetzeswache-Token", "sekret")
	rec := httptest.NewRecorder()

	start := time.Now()
	srv.Handler().ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || *env.Error != "recheck timed out" {
		t.Fatalf("error %v", env.Error)
	}
	maxWait := 4*srv.Svc.CFG.HTTPTimeout + 200*time.Millisecond
	if elapsed > maxWait {
		t.Fatalf("recheck took %v, want bound near %v", elapsed, 4*srv.Svc.CFG.HTTPTimeout)
	}
}

func TestRecheck_FastPathUnaffected(t *testing.T) {
	srv, mt := testServer(t, "sekret")
	mt.SetBytes("www.gesetze-im-internet.de", "/aktuDienst-rss-feed.xml", []byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", []byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	mt.SetBytes("www.recht.bund.de", "/rss/feeds/rss_bgbl-2.xml", []byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))

	req := httptest.NewRequest(http.MethodPost, "/v1/recheck", nil)
	req.Header.Set("X-Gesetzeswache-Token", "sekret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestRecheck_QueryTooLong400NoOutbound(t *testing.T) {
	srv, mt := testServer(t, "sekret")
	q := longQuery(513)
	req := httptest.NewRequest(http.MethodPost, "/v1/recheck?id="+q, nil)
	req.Header.Set("X-Gesetzeswache-Token", "sekret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || *env.Error != "query too long" {
		t.Fatalf("error %v", env.Error)
	}
	if len(mt.Hits()) != 0 {
		t.Fatalf("outbound requests fired: %v", mt.Hits())
	}
}

func TestClientError_masksUnexpectedInternal(t *testing.T) {
	srv, _ := testServer(t, "")
	if got := srv.clientError(errors.New("bbolt: /tmp/db closed")); got != clienterr.Internal {
		t.Fatalf("got %q want %q", got, clienterr.Internal)
	}
	if got := srv.clientError(service.ErrLawNotFound); got != "law not found" {
		t.Fatalf("got %q", got)
	}
}

func TestMiddleware_RequestIDValidation(t *testing.T) {
	srv, _ := testServer(t, "")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "trace-abc_01")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "trace-abc_01" {
		t.Fatalf("valid id=%q", got)
	}

	longID := strings.Repeat("a", 500)
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", longID)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got == longID || len(got) != 32 {
		t.Fatalf("long id should be replaced, got len=%d", len(got))
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "bad\nid")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got == "bad\nid" {
		t.Fatalf("newline id should be replaced")
	}
}

func TestMetricsEndpoint(t *testing.T) {
	reg := metrics.NewRegistry()
	metrics.RegisterDefaults(reg)

	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := config.Config{
		FreshnessMaxAge: 6 * time.Hour,
		EnableExport:    true,
		ExportCacheMax:  8,
		MatchThreshold:  0.75,
		GIIBase:         "https://www.gesetze-im-internet.de",
		RequestMinGap:   time.Millisecond,
		HTTPTimeout:     5 * time.Second,
	}
	mt := httpmock.New()
	httpClient := httpx.NewWithTransport(cfg.HTTPTimeout, cfg.RequestMinGap, 1<<20, mt)
	httpClient.Metrics = reg
	eng := search.NewEngine()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	orch := &sync.Orchestrator{CFG: cfg, Store: st, HTTP: httpClient, Search: eng, Log: log, Metrics: reg}
	svc := &service.Service{
		CFG:     cfg,
		Store:   st,
		Search:  eng,
		Sync:    orch,
		HTTP:    httpClient,
		Export:  export.NewCache(8),
		Log:     log,
		Metrics: reg,
	}
	srv := &Server{Svc: svc, Metrics: reg}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") || !strings.Contains(ct, "version=0.0.4") {
		t.Fatalf("Content-Type %q", ct)
	}
	body := rec.Body.String()
	for _, name := range []string{
		metrics.MetricCatalogReady,
		metrics.MetricDataFresh,
		metrics.MetricSyncLastSuccess,
		metrics.MetricSyncJobsTotal,
		metrics.MetricFreshnessLaws,
		metrics.MetricExportCacheLookups,
		metrics.MetricOutboundHTTP,
	} {
		if !strings.Contains(body, name) && !strings.Contains(body, "# TYPE "+name) && !strings.Contains(body, "# HELP "+name) {
			if !strings.Contains(body, "# HELP "+name) {
				t.Fatalf("missing metric family %s in body:\n%s", name, body)
			}
		}
	}
	for _, bad := range []string{"query=", `"error"`, "law_id", "ArbZG"} {
		if strings.Contains(body, bad) {
			t.Fatalf("unexpected sensitive fragment %q in metrics body", bad)
		}
	}
}

func TestMetricsEndpoint_noAuthRequired(t *testing.T) {
	reg := metrics.NewRegistry()
	metrics.RegisterDefaults(reg)
	srv := &Server{Metrics: reg}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}
