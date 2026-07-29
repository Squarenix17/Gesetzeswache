package service

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestCollectMetrics_gauges(t *testing.T) {
	mt := httpmock.New()
	reg := metrics.NewRegistry()
	metrics.RegisterDefaults(reg)
	svc := newTestService(t, mt)
	svc.Metrics = reg
	svc.Sync.Metrics = reg
	svc.HTTP.Metrics = reg

	svc.CollectMetrics(reg)
	if reg.GaugeValue(metrics.MetricCatalogReady, nil) != 0 {
		t.Fatal("expected catalog_ready 0 before TOC")
	}

	seedCatalog(t, svc, mt)
	seedSyncFreshMeta(t, svc, time.Now().UTC())
	svc.CollectMetrics(reg)

	if reg.GaugeValue(metrics.MetricCatalogReady, nil) != 1 {
		t.Fatal("expected catalog_ready 1 after TOC")
	}
	if reg.GaugeValue(metrics.MetricDataFresh, nil) != 1 {
		t.Fatal("expected data_fresh 1")
	}
	tocTS := reg.GaugeValue(metrics.MetricSyncLastSuccess, map[string]string{"source": "toc"})
	if tocTS <= 0 {
		t.Fatalf("toc timestamp gauge=%v", tocTS)
	}

	var buf bytes.Buffer
	if err := reg.WritePrometheus(&buf); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	if !strings.Contains(body, metrics.MetricCatalogReady+" 1") {
		t.Fatalf("missing catalog_ready sample:\n%s", body)
	}
}

func TestCollectMetrics_freshnessLaws(t *testing.T) {
	mt := httpmock.New()
	reg := metrics.NewRegistry()
	svc := newTestService(t, mt)
	svc.Metrics = reg

	now := time.Now().UTC()
	_ = svc.Store.PutFreshness(domain.FreshnessRecord{
		LawID: "x", State: domain.FreshnessConfirmedStale, EvaluatedAt: now,
	})
	_ = svc.Store.PutFreshness(domain.FreshnessRecord{
		LawID: "y", State: domain.FreshnessUncertain, EvaluatedAt: now,
	})
	svc.CollectMetrics(reg)

	if reg.GaugeValue(metrics.MetricFreshnessLaws, map[string]string{"state": "confirmed_stale"}) != 1 {
		t.Fatal("expected stale count 1")
	}
	if reg.GaugeValue(metrics.MetricFreshnessLaws, map[string]string{"state": "uncertain"}) != 1 {
		t.Fatal("expected uncertain count 1")
	}
}

func TestCollectMetrics_freshnessGaugesClearedOnStoreError(t *testing.T) {
	mt := httpmock.New()
	reg := metrics.NewRegistry()
	svc := newTestService(t, mt)
	svc.Metrics = reg

	_ = svc.Store.PutFreshness(domain.FreshnessRecord{
		LawID: "x", State: domain.FreshnessConfirmedStale, EvaluatedAt: time.Now().UTC(),
	})
	svc.CollectMetrics(reg)
	if reg.GaugeValue(metrics.MetricFreshnessLaws, map[string]string{"state": "confirmed_stale"}) != 1 {
		t.Fatal("setup: expected stale=1")
	}

	_ = svc.Store.Close()
	svc.CollectMetrics(reg)
	if reg.GaugeValue(metrics.MetricFreshnessLaws, map[string]string{"state": "confirmed_stale"}) != 0 {
		t.Fatal("expected freshness gauges cleared to 0 after collect failure")
	}
	if reg.GaugeValue(metrics.MetricFreshnessLaws, map[string]string{"state": "uncertain"}) != 0 {
		t.Fatal("expected uncertain gauge cleared to 0")
	}
}

func TestExportCacheLookupMetrics(t *testing.T) {
	mt := httpmock.New()
	reg := metrics.NewRegistry()
	svc := newTestService(t, mt)
	svc.Metrics = reg
	svc.Sync.Metrics = reg
	svc.HTTP.Metrics = reg
	seedCatalog(t, svc, mt)

	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	_, err := svc.ExportText(context.Background(), "ArbZG", []string{export.FormatFlat}, IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	miss := reg.CounterValue(metrics.MetricExportCacheLookups, map[string]string{"result": "miss"})
	if miss < 1 {
		t.Fatalf("miss=%v want >=1", miss)
	}

	_, err = svc.ExportText(context.Background(), "ArbZG", []string{export.FormatFlat}, IncludeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	hit := reg.CounterValue(metrics.MetricExportCacheLookups, map[string]string{"result": "hit"})
	if hit < 1 {
		t.Fatalf("hit=%v want >=1", hit)
	}
}
