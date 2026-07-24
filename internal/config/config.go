package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration (env-based).
type Config struct {
	HTTPAddr           string
	StorePath          string
	MatchThreshold     float64
	FreshnessMaxAge    time.Duration
	TOCInterval        time.Duration
	GIIFeedInterval    time.Duration
	BGBlFeedInterval   time.Duration
	ELIProbeInterval   time.Duration
	UnmatchedGrace     time.Duration
	EnableHeuristic    bool
	EnableExport       bool
	ExportCacheMax     int
	HTTPTimeout        time.Duration
	RequestMinGap      time.Duration
	GIIBase            string
	GIITOCURL          string
	GIIFeedURL         string
	BGBlFeed1URL       string
	BGBlFeed2URL       string
	ELIBase            string
	SharedSecret       string // optional; empty = disabled
	VariantsPath       string
	LinkedInstrumentsPath string
	RefuseExportStale  bool
	StandRefreshMax    int // max laws missing Stand to refresh at InitialSync (0 = skip bulk)
	DiscoveryEnabled   bool
	DiscoveryMaxPerCycle int
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:         env("GEW_HTTP_ADDR", ":8080"),
		StorePath:        env("GEW_STORE_PATH", "gesetzeswache.db"),
		MatchThreshold:   envFloat("GEW_MATCH_THRESHOLD", 0.75),
		FreshnessMaxAge:  envDur("GEW_FRESHNESS_MAX_AGE", 6*time.Hour),
		TOCInterval:      envDur("GEW_TOC_INTERVAL", 6*time.Hour),
		GIIFeedInterval:  envDur("GEW_GII_FEED_INTERVAL", 15*time.Minute),
		BGBlFeedInterval: envDur("GEW_BGBL_FEED_INTERVAL", 15*time.Minute),
		ELIProbeInterval: envDur("GEW_ELI_PROBE_INTERVAL", 30*time.Minute),
		UnmatchedGrace:   envDur("GEW_UNMATCHED_GRACE", 72*time.Hour),
		EnableHeuristic:  envBool("GEW_ENABLE_HEURISTIC", true),
		EnableExport:     envBool("GEW_ENABLE_EXPORT", true),
		ExportCacheMax:   envInt("GEW_EXPORT_CACHE_MAX", 64),
		HTTPTimeout:      envDur("GEW_HTTP_TIMEOUT", 60*time.Second),
		RequestMinGap:    envDur("GEW_REQUEST_MIN_GAP", 250*time.Millisecond),
		GIIBase:          env("GEW_GII_BASE", "https://www.gesetze-im-internet.de"),
		GIITOCURL:        env("GEW_GII_TOC_URL", "https://www.gesetze-im-internet.de/gii-toc.xml"),
		GIIFeedURL:       env("GEW_GII_FEED_URL", "https://www.gesetze-im-internet.de/aktuDienst-rss-feed.xml"),
		BGBlFeed1URL:     env("GEW_BGBL_FEED1_URL", "https://www.recht.bund.de/rss/feeds/rss_bgbl-1.xml"),
		BGBlFeed2URL:     env("GEW_BGBL_FEED2_URL", "https://www.recht.bund.de/rss/feeds/rss_bgbl-2.xml"),
		ELIBase:          env("GEW_ELI_BASE", "https://www.recht.bund.de/eli/bund"),
		SharedSecret:     os.Getenv("GEW_SHARED_SECRET"),
		VariantsPath:     env("GEW_VARIANTS_PATH", "variants/variants.tsv"),
		LinkedInstrumentsPath: env("GEW_LINKED_INSTRUMENTS_PATH", "variants/linked_instruments.tsv"),
		RefuseExportStale: envBool("GEW_REFUSE_EXPORT_STALE", false),
		StandRefreshMax:        envInt("GEW_STAND_REFRESH_MAX", 10),
		DiscoveryEnabled:       envBool("GEW_DISCOVERY_ENABLED", true),
		DiscoveryMaxPerCycle:   envInt("GEW_DISCOVERY_MAX_PER_CYCLE", 50),
	}
	if c.MatchThreshold <= 0 || c.MatchThreshold > 1 {
		return c, fmt.Errorf("invalid match threshold")
	}
	if c.StorePath == "" {
		return c, fmt.Errorf("store path required")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}
