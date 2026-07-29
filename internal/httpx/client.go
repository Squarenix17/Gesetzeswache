// Package httpx provides a shared HTTP client with timeouts, conditional GETs, rate limiting, and host allowlisting.
package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/metrics"
)

// Client wraps http.Client with per-host rate limiting, size caps, and SSRF controls.
type Client struct {
	hc       *http.Client
	minGap   time.Duration
	maxBytes int64
	allowed  map[string]struct{}
	mu       sync.Mutex
	last     map[string]time.Time
	Metrics  *metrics.Registry
}

func New(timeout time.Duration, minGap time.Duration, maxBytes int64, allowHosts ...string) *Client {
	return NewWithTransport(timeout, minGap, maxBytes, nil, allowHosts...)
}

// NewWithTransport is like New but uses rt as the underlying RoundTripper when non-nil.
// Production callers pass nil (default transport). Tests inject a path-routed mock that
// never dials the network while still exercising allowlisting and SSRF checks.
// Injected transports must honor the validated req.URL destination and must not dial elsewhere.
func NewWithTransport(timeout time.Duration, minGap time.Duration, maxBytes int64, rt http.RoundTripper, allowHosts ...string) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if minGap < 0 {
		minGap = 0
	}
	if maxBytes <= 0 {
		maxBytes = 32 << 20
	}
	allowed := map[string]struct{}{}
	for _, h := range allowHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			allowed[h] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		allowed["www.gesetze-im-internet.de"] = struct{}{}
		allowed["gesetze-im-internet.de"] = struct{}{}
		allowed["www.recht.bund.de"] = struct{}{}
		allowed["recht.bund.de"] = struct{}{}
	}
	c := &Client{
		minGap:   minGap,
		maxBytes: maxBytes,
		allowed:  allowed,
		last:     map[string]time.Time{},
	}
	transport := rt
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           c.dialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		}
	}
	c.hc = &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return c.validateURL(req.URL)
		},
	}
	return c
}

// ValidateURL checks that rawURL satisfies the same SSRF rules as outbound GET.
func ValidateURL(rawURL string, allowHosts ...string) error {
	c := New(30*time.Second, 200*time.Millisecond, 1<<20, allowHosts...)
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	return c.validateURL(u)
}

// IsBlockedIP reports whether ip must not be dialed (loopback, private, link-local, unspecified).
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// NAT64 well-known prefix 64:ff9b::/96 — decode embedded IPv4 and re-check.
	if len(ip) == net.IPv6len && ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b {
		embedded := net.IP(append([]byte(nil), ip[12:16]...))
		if embedded.To4() != nil && IsBlockedIP(embedded) {
			return true
		}
	}
	ip = ip.To16()
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// CGNAT 100.64.0.0/10
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
		// Explicit metadata / link-local IPv4 range 169.254.0.0/16.
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
	}
	return false
}

func (c *Client) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses for host %q", host)
	}
	var blocked error
	for _, ipa := range ips {
		if IsBlockedIP(ipa.IP) {
			blocked = fmt.Errorf("resolved to blocked IP for host %q", host)
			continue
		}
		d := net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		if blocked == nil {
			blocked = err
		}
	}
	if blocked != nil {
		return nil, blocked
	}
	return nil, fmt.Errorf("dial failed for %s", addr)
}

func (c *Client) validateURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("nil url")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("disallowed scheme %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if _, ok := c.allowed[host]; !ok {
		return fmt.Errorf("host not allowlisted: %s", host)
	}
	// Block literal IPs (SSRF to link-local/metadata)
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("literal IP hosts disallowed")
	}
	return nil
}

// Get fetches url. etag/mod may be empty. Returns body, new etag, status.
func (c *Client) Get(ctx context.Context, rawURL, etag, modSince string) (body []byte, newETag string, status int, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", 0, err
	}
	if err := c.validateURL(u); err != nil {
		return nil, "", 0, err
	}
	host := strings.ToLower(u.Hostname())
	defer func() { c.recordOutbound(host, status, err) }()

	// Prefer https for allowlisted government hosts
	if u.Scheme == "http" {
		u.Scheme = "https"
		rawURL = u.String()
	}
	if err := c.waitHost(ctx, u.Host); err != nil {
		return nil, "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("User-Agent", "gew/0.2.0 (+https://github.com/Squarenix17/gesetzeswache)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modSince != "" {
		req.Header.Set("If-Modified-Since", modSince)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, resp.StatusCode, nil
	}
	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", resp.StatusCode, err
	}
	if int64(len(data)) > c.maxBytes {
		return nil, "", resp.StatusCode, fmt.Errorf("response exceeds size cap")
	}
	return data, resp.Header.Get("ETag"), resp.StatusCode, nil
}

func (c *Client) recordOutbound(host string, status int, err error) {
	if host == "" {
		return
	}
	result := "success"
	switch {
	case err != nil && isTimeout(err):
		result = "timeout"
	case err != nil || status >= 400:
		result = "error"
	}
	_ = c.Metrics.IncCounter(metrics.MetricOutboundHTTP, map[string]string{
		"host":   host,
		"result": result,
	}, 1)
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// Exists probes URL existence via GET (HEAD is unreliable on some gov hosts).
func (c *Client) Exists(ctx context.Context, rawURL string) (bool, int, error) {
	_, _, st, err := c.Get(ctx, rawURL, "", "")
	if err != nil {
		return false, st, err
	}
	return st >= 200 && st < 400, st, nil
}

func (c *Client) waitHost(ctx context.Context, host string) error {
	for {
		c.mu.Lock()
		last := c.last[host]
		wait := c.minGap - time.Since(last)
		if wait <= 0 {
			c.last[host] = time.Now()
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
