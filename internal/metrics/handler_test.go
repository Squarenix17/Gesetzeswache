package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHandlerReturnsPrometheusResponse(t *testing.T) {
	r := NewRegistry()
	r.RegisterHelp(MetricSyncJobsTotal, "sync jobs", "counter")
	if err := r.IncCounter(MetricSyncJobsTotal, map[string]string{"status": "ok"}, 1); err != nil {
		t.Fatalf("IncCounter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("Content-Type %q missing text/plain", ct)
	}
	if !strings.Contains(ct, "version=0.0.4") {
		t.Fatalf("Content-Type %q missing version=0.0.4", ct)
	}
	if !strings.Contains(ct, "charset=utf-8") {
		t.Fatalf("Content-Type %q missing charset=utf-8", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, MetricSyncJobsTotal) {
		t.Fatalf("body missing metric %q: %s", MetricSyncJobsTotal, body)
	}
}

func TestHandlerCollectCallbackRunsBeforeWrite(t *testing.T) {
	r := NewRegistry()
	r.RegisterHelp(MetricCatalogReady, "catalog ready", "gauge")

	var called atomic.Bool
	collect := func(reg *Registry) {
		called.Store(true)
		if err := reg.SetGauge(MetricCatalogReady, nil, 1); err != nil {
			t.Errorf("SetGauge in collect: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.Handler(collect).ServeHTTP(rec, req)

	if !called.Load() {
		t.Fatal("collect callback was not invoked")
	}
	body := rec.Body.String()
	if !strings.Contains(body, MetricCatalogReady+" 1") {
		t.Fatalf("expected metric written by collect callback in %q", body)
	}
}
