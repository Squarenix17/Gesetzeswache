// Package httpx provides a shared HTTP client with timeouts, conditional GETs, rate limiting, and host allowlisting.
package httpx

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client wraps http.Client with per-host rate limiting, size caps, and SSRF controls.
type Client struct {
	hc       *http.Client
	minGap   time.Duration
	maxBytes int64
	allowed  map[string]struct{}
	mu       sync.Mutex
	last     map[string]time.Time
}

func New(timeout time.Duration, minGap time.Duration, maxBytes int64, allowHosts ...string) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if minGap <= 0 {
		minGap = 200 * time.Millisecond
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
	c.hc = &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return c.validateURL(req.URL)
		},
	}
	return c
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
	req.Header.Set("User-Agent", "gesetzeswache/0.1 (+https://github.com/Squarenix17/gesetzeswache)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if modSince != "" {
		req.Header.Set("If-Modified-Since", modSince)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
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
