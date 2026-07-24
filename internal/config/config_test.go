package config

import (
	"os"
	"testing"
)

func TestLoad_DiscoveryDefaults(t *testing.T) {
	t.Setenv("GEW_DISCOVERY_ENABLED", "")
	t.Setenv("GEW_DISCOVERY_MAX_PER_CYCLE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DiscoveryEnabled {
		t.Fatal("DiscoveryEnabled default want true")
	}
	if cfg.DiscoveryMaxPerCycle != 50 {
		t.Fatalf("DiscoveryMaxPerCycle=%d want 50", cfg.DiscoveryMaxPerCycle)
	}
}

func TestLoad_DiscoveryEnv(t *testing.T) {
	t.Setenv("GEW_DISCOVERY_ENABLED", "false")
	t.Setenv("GEW_DISCOVERY_MAX_PER_CYCLE", "12")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DiscoveryEnabled {
		t.Fatal("DiscoveryEnabled want false from env")
	}
	if cfg.DiscoveryMaxPerCycle != 12 {
		t.Fatalf("DiscoveryMaxPerCycle=%d want 12", cfg.DiscoveryMaxPerCycle)
	}
}

func TestLoad_DiscoveryEnv_invalidBoolFallsBack(t *testing.T) {
	t.Setenv("GEW_DISCOVERY_ENABLED", "not-a-bool")
	t.Setenv("GEW_DISCOVERY_MAX_PER_CYCLE", "not-int")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DiscoveryEnabled {
		t.Fatal("invalid bool should fall back to default true")
	}
	if cfg.DiscoveryMaxPerCycle != 50 {
		t.Fatalf("invalid int should fall back to 50, got %d", cfg.DiscoveryMaxPerCycle)
	}
	_ = os.Unsetenv("GEW_DISCOVERY_ENABLED")
	_ = os.Unsetenv("GEW_DISCOVERY_MAX_PER_CYCLE")
}
