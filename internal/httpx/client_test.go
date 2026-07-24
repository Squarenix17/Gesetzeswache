package httpx

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestGetAllowlistedMock(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", []byte(`<items></items>`))
	c := NewWithTransport(5*time.Second, time.Millisecond, 1<<20, mt)

	body, _, status, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/gii-toc.xml", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	if string(body) != `<items></items>` {
		t.Fatalf("body %q", body)
	}
}

func TestGetRejectsLiteralIP(t *testing.T) {
	mt := httpmock.New()
	c := NewWithTransport(5*time.Second, time.Millisecond, 1<<20, mt, "127.0.0.1")
	_, _, _, err := c.Get(context.Background(), "http://127.0.0.1/x", "", "")
	if err == nil {
		t.Fatal("expected literal IP rejection")
	}
}

func TestGetRejectsUnknownHost(t *testing.T) {
	mt := httpmock.New()
	c := NewWithTransport(5*time.Second, time.Millisecond, 1<<20, mt)
	_, _, _, err := c.Get(context.Background(), "https://evil.example/x", "", "")
	if err == nil {
		t.Fatal("expected allowlist rejection")
	}
}

func TestGetTransportError(t *testing.T) {
	mt := httpmock.New()
	mt.Set("www.recht.bund.de", "/rss/feeds/rss_bgbl-1.xml", httpmock.Response{
		Err: fmt.Errorf("simulated TLS timeout"),
	})
	c := NewWithTransport(5*time.Second, time.Millisecond, 1<<20, mt)
	_, _, _, err := c.Get(context.Background(), "https://www.recht.bund.de/rss/feeds/rss_bgbl-1.xml", "", "")
	if err == nil {
		t.Fatal("expected transport error")
	}
}

func TestGetNotModified(t *testing.T) {
	mt := httpmock.New()
	mt.Set("www.gesetze-im-internet.de", "/x", httpmock.Response{
		Status: http.StatusNotModified,
		ETag:   `"abc"`,
	})
	c := NewWithTransport(5*time.Second, time.Millisecond, 1<<20, mt)
	body, etag, status, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/x", `"abc"`, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != 304 || body != nil || etag != `"abc"` {
		t.Fatalf("got status=%d etag=%q body=%v", status, etag, body)
	}
}

func TestHTTPUpgradedToHTTPS(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/gii-toc.xml", []byte("ok"))
	c := NewWithTransport(5*time.Second, time.Millisecond, 1<<20, mt)
	_, _, status, err := c.Get(context.Background(), "http://www.gesetze-im-internet.de/gii-toc.xml", "", "")
	if err != nil || status != 200 {
		t.Fatalf("err=%v status=%d", err, status)
	}
	hits := mt.Hits()
	if len(hits) != 1 || hits[0] != "www.gesetze-im-internet.de|/gii-toc.xml" {
		t.Fatalf("hits=%v", hits)
	}
}
