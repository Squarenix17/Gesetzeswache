package metrics

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestIncCounterIncrements(t *testing.T) {
	tests := []struct {
		name   string
		delta  float64
		calls  int
		expect float64
	}{
		{name: "single increment", delta: 1, calls: 1, expect: 1},
		{name: "fractional delta", delta: 0.5, calls: 2, expect: 1},
		{name: "large delta", delta: 10, calls: 1, expect: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			r.RegisterHelp(MetricSyncJobsTotal, "sync jobs", "counter")
			labels := map[string]string{"status": "success"}

			for i := 0; i < tt.calls; i++ {
				if err := r.IncCounter(MetricSyncJobsTotal, labels, tt.delta); err != nil {
					t.Fatalf("IncCounter: %v", err)
				}
			}

			var buf bytes.Buffer
			if err := r.WritePrometheus(&buf); err != nil {
				t.Fatalf("WritePrometheus: %v", err)
			}
			body := buf.String()
			if !strings.Contains(body, MetricSyncJobsTotal+`{status="success"} `) {
				t.Fatalf("missing counter line in %q", body)
			}
			if !strings.Contains(body, MetricSyncJobsTotal+`{status="success"} `+formatFloat(tt.expect)) {
				t.Fatalf("expected value %g in %q", tt.expect, body)
			}
		})
	}
}

func TestSetGaugeOverwrites(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		expect float64
	}{
		{name: "overwrite single", values: []float64{1, 2}, expect: 2},
		{name: "overwrite zero", values: []float64{5, 0}, expect: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			r.RegisterHelp(MetricCatalogReady, "catalog ready", "gauge")
			labels := map[string]string{"source": "gii"}

			for _, v := range tt.values {
				if err := r.SetGauge(MetricCatalogReady, labels, v); err != nil {
					t.Fatalf("SetGauge: %v", err)
				}
			}

			var buf bytes.Buffer
			if err := r.WritePrometheus(&buf); err != nil {
				t.Fatalf("WritePrometheus: %v", err)
			}
			body := buf.String()
			if !strings.Contains(body, MetricCatalogReady+`{source="gii"} `+formatFloat(tt.expect)) {
				t.Fatalf("expected gauge value %g in %q", tt.expect, body)
			}
		})
	}
}

func TestWritePrometheusFormat(t *testing.T) {
	r := NewRegistry()
	r.RegisterHelp(MetricOutboundHTTP, "outbound HTTP requests", "counter")
	if err := r.IncCounter(MetricOutboundHTTP, map[string]string{"code": "200"}, 3); err != nil {
		t.Fatalf("IncCounter: %v", err)
	}

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	body := buf.String()

	checks := []string{
		"# HELP " + MetricOutboundHTTP,
		"# TYPE " + MetricOutboundHTTP + " counter",
		MetricOutboundHTTP + `{code="200"} 3`,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in output:\n%s", want, body)
		}
	}
}

func TestInvalidMetricNameRejected(t *testing.T) {
	tests := []struct {
		name   string
		metric string
	}{
		{name: "starts with digit", metric: "1invalid"},
		{name: "contains hyphen", metric: "bad-name"},
		{name: "empty", metric: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			if err := r.IncCounter(tt.metric, nil, 1); err == nil {
				t.Fatal("expected IncCounter error")
			}
			if err := r.SetGauge(tt.metric, nil, 1); err == nil {
				t.Fatal("expected SetGauge error")
			}
		})
	}
}

func TestInvalidLabelKeyRejected(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
	}{
		{name: "starts with digit", labels: map[string]string{"1bad": "x"}},
		{name: "contains hyphen", labels: map[string]string{"bad-key": "x"}},
		{name: "empty key", labels: map[string]string{"": "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			if err := r.IncCounter(MetricSyncJobsTotal, tt.labels, 1); err == nil {
				t.Fatal("expected IncCounter error")
			}
			if err := r.SetGauge(MetricDataFresh, tt.labels, 1); err == nil {
				t.Fatal("expected SetGauge error")
			}
		})
	}
}

func TestNilRegistryNoPanic(t *testing.T) {
	var r *Registry

	if err := r.IncCounter(MetricSyncJobsTotal, nil, 1); err != nil {
		t.Fatalf("IncCounter on nil: %v", err)
	}
	if err := r.SetGauge(MetricDataFresh, nil, 1); err != nil {
		t.Fatalf("SetGauge on nil: %v", err)
	}
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus on nil: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output from nil registry, got %q", buf.String())
	}
	r.RegisterHelp(MetricCatalogReady, "noop", "gauge")
}

func TestConcurrentIncCounter(t *testing.T) {
	r := NewRegistry()
	r.RegisterHelp(MetricExportCacheLookups, "cache lookups", "counter")
	labels := map[string]string{"result": "hit"}

	const goroutines = 16
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := r.IncCounter(MetricExportCacheLookups, labels, 1); err != nil {
					t.Errorf("IncCounter: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	want := MetricExportCacheLookups + `{result="hit"} ` + formatFloat(float64(goroutines*perGoroutine))
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("expected %q in %q", want, buf.String())
	}
}

func TestLabelValueEscaping(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "quote", value: `say "hi"`, want: `say \"hi\"`},
		{name: "newline", value: "line1\nline2", want: `line1\nline2`},
		{name: "backslash", value: `path\to`, want: `path\\to`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			r.RegisterHelp(MetricFreshnessLaws, "freshness", "gauge")
			labels := map[string]string{"note": tt.value}
			if err := r.SetGauge(MetricFreshnessLaws, labels, 1); err != nil {
				t.Fatalf("SetGauge: %v", err)
			}

			var buf bytes.Buffer
			if err := r.WritePrometheus(&buf); err != nil {
				t.Fatalf("WritePrometheus: %v", err)
			}
			body := buf.String()
			if !strings.Contains(body, `note="`+tt.want+`"`) {
				t.Fatalf("expected escaped value %q in %q", tt.want, body)
			}
		})
	}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
