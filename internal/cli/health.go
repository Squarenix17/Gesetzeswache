package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const healthClientTimeout = 2 * time.Second

// HealthProbeURL derives the localhost URL for GET /healthz from GEW_HTTP_ADDR.
// Empty host, 0.0.0.0, or [::] bind addresses map to 127.0.0.1; explicit hosts are preserved.
func HealthProbeURL(httpAddr string) (string, error) {
	httpAddr = strings.TrimSpace(httpAddr)
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		if strings.Contains(httpAddr, ":") {
			return "", fmt.Errorf("invalid GEW_HTTP_ADDR %q: %w", httpAddr, err)
		}
		host, port = "", httpAddr
	}
	if port == "" {
		port = "8080"
	}
	switch host {
	case "", "0.0.0.0", "[::]", "::":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s/healthz", host, port), nil
}

// ProbeHealth performs one GET to url with a 2s timeout. Returns nil on HTTP 200.
func ProbeHealth(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), healthClientTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check status %d", resp.StatusCode)
	}
	return nil
}

// HealthCheck probes /healthz using addr (typically GEW_HTTP_ADDR). Exits silently on success.
func HealthCheck(httpAddr string) error {
	url, err := HealthProbeURL(httpAddr)
	if err != nil {
		return err
	}
	return ProbeHealth(url)
}
