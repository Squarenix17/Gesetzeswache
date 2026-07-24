package apihttp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			// HELP/TYPE registered even with zero series; at least HELP must appear
			if !strings.Contains(body, "# HELP "+name) {
				t.Fatalf("missing metric family %s in body:\n%s", name, body)
			}
		}
	}
	// Security: no query/law PII leakage patterns from sync attempt errors
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
