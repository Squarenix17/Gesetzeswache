// Package httpmock provides a path-routed http.RoundTripper for offline integration tests.
// Requests keep allowlisted government hostnames; nothing is dialed on the network.
package httpmock

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Response is a canned reply or transport-level error for a host+path route.
type Response struct {
	Status  int
	Body    []byte
	Header  http.Header
	Err     error // if set, RoundTrip returns Err (no *http.Response)
	ETag    string
}

// Transport maps "host|path" keys to canned responses. Unmatched routes return 404.
type Transport struct {
	mu     sync.Mutex
	routes map[string]Response
	hits   []string
}

// New returns an empty mock transport.
func New() *Transport {
	return &Transport{routes: map[string]Response{}}
}

func routeKey(host, path string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return host + "|" + path
}

// Set registers a response for host+path (path should include leading slash).
func (t *Transport) Set(host, path string, resp Response) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if resp.Status == 0 && resp.Err == nil {
		resp.Status = http.StatusOK
	}
	if len(resp.Body) > 0 {
		resp.Body = append([]byte(nil), resp.Body...)
	}
	t.routes[routeKey(host, path)] = resp
}

// SetBytes is a convenience for 200 OK with a body.
func (t *Transport) SetBytes(host, path string, body []byte) {
	t.Set(host, path, Response{Status: http.StatusOK, Body: append([]byte(nil), body...)})
}

// Hits returns request keys in order (host|path) for assertions.
func (t *Transport) Hits() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.hits))
	copy(out, t.hits)
	return out
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("nil request")
	}
	host := req.URL.Hostname()
	path := req.URL.EscapedPath()
	key := routeKey(host, path)

	t.mu.Lock()
	t.hits = append(t.hits, key)
	resp, ok := t.routes[key]
	t.mu.Unlock()

	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	if resp.Err != nil {
		return nil, resp.Err
	}
	hdr := http.Header{}
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	if resp.ETag != "" {
		hdr.Set("ETag", resp.ETag)
	}
	status := resp.Status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(bytes.NewReader(resp.Body)),
		Header:     hdr,
		Request:    req,
		ContentLength: int64(len(resp.Body)),
	}, nil
}
