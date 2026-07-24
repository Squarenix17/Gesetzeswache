package metrics

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	MetricCatalogReady       = "gew_catalog_ready"
	MetricDataFresh          = "gew_data_fresh"
	MetricSyncLastSuccess    = "gew_sync_last_success_timestamp_seconds"
	MetricSyncJobsTotal      = "gew_sync_jobs_total"
	MetricFreshnessLaws      = "gew_freshness_laws"
	MetricExportCacheLookups = "gew_export_cache_lookups_total"
	MetricOutboundHTTP       = "gew_outbound_http_requests_total"
)

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelKeyRE   = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

type metricMeta struct {
	help string
	typ  string
}

type labeledValue struct {
	labels map[string]string
	value  float64
}

// Registry is a thread-safe in-memory Prometheus text exposition registry.
type Registry struct {
	mu       sync.RWMutex
	meta     map[string]metricMeta
	counters map[string]map[string]float64
	gauges   map[string]map[string]float64
}

// NewRegistry returns an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		meta:     make(map[string]metricMeta),
		counters: make(map[string]map[string]float64),
		gauges:   make(map[string]map[string]float64),
	}
}

// RegisterDefaults registers HELP/TYPE for the Phase 4 MVP metric families.
func RegisterDefaults(r *Registry) {
	if r == nil {
		return
	}
	r.RegisterHelp(MetricCatalogReady, "1 if at least one TOC sync succeeded", "gauge")
	r.RegisterHelp(MetricDataFresh, "1 if sync data is within freshness max age", "gauge")
	r.RegisterHelp(MetricSyncLastSuccess, "Unix timestamp of last successful sync by source", "gauge")
	r.RegisterHelp(MetricSyncJobsTotal, "Total sync job completions by source and result", "counter")
	r.RegisterHelp(MetricFreshnessLaws, "Number of laws by freshness state", "gauge")
	r.RegisterHelp(MetricExportCacheLookups, "Export IR cache lookups by result", "counter")
	r.RegisterHelp(MetricOutboundHTTP, "Outbound HTTP requests by host and result", "counter")
}

// CounterValue returns the current counter value for name+labels (0 if missing).
func (r *Registry) CounterValue(name string, labels map[string]string) float64 {
	if r == nil {
		return 0
	}
	key, err := canonicalLabelKey(labels)
	if err != nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.counters[name][key]
}

// GaugeValue returns the current gauge value for name+labels (0 if missing).
func (r *Registry) GaugeValue(name string, labels map[string]string) float64 {
	if r == nil {
		return 0
	}
	key, err := canonicalLabelKey(labels)
	if err != nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.gauges[name][key]
}

// RegisterHelp records HELP and TYPE metadata for a metric.
// typ must be "counter" or "gauge".
func (r *Registry) RegisterHelp(name, help, typ string) {
	if r == nil {
		return
	}
	if !metricNameRE.MatchString(name) {
		return
	}
	if typ != "counter" && typ != "gauge" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta[name] = metricMeta{help: help, typ: typ}
}

// IncCounter increments a counter metric by delta.
func (r *Registry) IncCounter(name string, labels map[string]string, delta float64) error {
	if r == nil {
		return nil
	}
	if err := validateMetricName(name); err != nil {
		return err
	}
	if err := r.validateLabels(name, labels); err != nil {
		return err
	}

	key, err := canonicalLabelKey(labels)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counters[name] == nil {
		r.counters[name] = make(map[string]float64)
	}
	r.counters[name][key] += delta
	return nil
}

// SetGauge sets a gauge metric to value.
func (r *Registry) SetGauge(name string, labels map[string]string, value float64) error {
	if r == nil {
		return nil
	}
	if err := validateMetricName(name); err != nil {
		return err
	}
	if err := r.validateLabels(name, labels); err != nil {
		return err
	}

	key, err := canonicalLabelKey(labels)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gauges[name] == nil {
		r.gauges[name] = make(map[string]float64)
	}
	r.gauges[name][key] = value
	return nil
}

// WritePrometheus writes all metrics in Prometheus text exposition format.
func (r *Registry) WritePrometheus(w io.Writer) error {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	names := make([]string, 0, len(r.meta)+len(r.counters)+len(r.gauges))
	seen := make(map[string]struct{})
	for name := range r.meta {
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for name := range r.counters {
		if _, ok := seen[name]; !ok {
			names = append(names, name)
			seen[name] = struct{}{}
		}
	}
	for name := range r.gauges {
		if _, ok := seen[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		meta, hasMeta := r.meta[name]
		if hasMeta {
			if meta.help != "" {
				fmt.Fprintf(&b, "# HELP %s %s\n", name, meta.help)
			}
			if meta.typ != "" {
				fmt.Fprintf(&b, "# TYPE %s %s\n", name, meta.typ)
			}
		}

		series := r.collectSeries(name, r.counters[name], r.gauges[name])
		for _, s := range series {
			labelPart := formatLabels(s.labels)
			if labelPart != "" {
				fmt.Fprintf(&b, "%s{%s} %g\n", name, labelPart, s.value)
			} else {
				fmt.Fprintf(&b, "%s %g\n", name, s.value)
			}
		}
	}
	r.mu.RUnlock()

	_, err := io.WriteString(w, b.String())
	return err
}

// Handler returns an http.Handler that runs collect callbacks then writes metrics.
func (r *Registry) Handler(collect ...func(*Registry)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if r != nil {
			for _, fn := range collect {
				if fn != nil {
					fn(r)
				}
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if r != nil {
			_ = r.WritePrometheus(w)
		}
	})
}

func (r *Registry) validateLabels(_ string, labels map[string]string) error {
	for key := range labels {
		if err := validateLabelKey(key); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) collectSeries(name string, counters, gauges map[string]float64) []labeledValue {
	keys := make(map[string]struct{})
	for key := range counters {
		keys[key] = struct{}{}
	}
	for key := range gauges {
		keys[key] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	series := make([]labeledValue, 0, len(sortedKeys))
	for _, key := range sortedKeys {
		labels := decodeLabelKey(key)
		value, ok := gauges[key]
		if !ok {
			value = counters[key]
		}
		series = append(series, labeledValue{labels: labels, value: value})
	}
	_ = name
	return series
}

func validateMetricName(name string) error {
	if !metricNameRE.MatchString(name) {
		return fmt.Errorf("invalid metric name %q", name)
	}
	return nil
}

func validateLabelKey(key string) error {
	if !labelKeyRE.MatchString(key) {
		return fmt.Errorf("invalid label key %q", key)
	}
	return nil
}

func canonicalLabelKey(labels map[string]string) (string, error) {
	if len(labels) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if err := validateLabelKey(key); err != nil {
			return "", err
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(labels[key])
	}
	return b.String(), nil
}

func decodeLabelKey(key string) map[string]string {
	if key == "" {
		return nil
	}
	labels := make(map[string]string)
	for _, part := range strings.Split(key, "|") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		labels[k] = v
	}
	return labels
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+`="`+escapeLabelValue(labels[key])+`"`)
	}
	return strings.Join(parts, ",")
}

func escapeLabelValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
