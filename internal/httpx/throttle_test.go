package httpx

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestGetThrottle_sameHostSerializes(t *testing.T) {
	const gap = 50 * time.Millisecond
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/a", []byte("a"))
	mt.SetBytes("www.gesetze-im-internet.de", "/b", []byte("b"))
	c := NewWithTransport(5*time.Second, gap, 1<<20, mt)

	start := time.Now()
	if _, _, _, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/a", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/b", "", ""); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < gap {
		t.Fatalf("elapsed %v want >= %v", elapsed, gap)
	}
}

func TestGetThrottle_differentHostsNotSerialized(t *testing.T) {
	const gap = 80 * time.Millisecond
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/a", []byte("a"))
	mt.SetBytes("www.recht.bund.de", "/b", []byte("b"))
	c := NewWithTransport(5*time.Second, gap, 1<<20, mt)

	var wg sync.WaitGroup
	wg.Add(2)
	start := time.Now()
	go func() {
		defer wg.Done()
		_, _, _, _ = c.Get(context.Background(), "https://www.gesetze-im-internet.de/a", "", "")
	}()
	go func() {
		defer wg.Done()
		_, _, _, _ = c.Get(context.Background(), "https://www.recht.bund.de/b", "", "")
	}()
	wg.Wait()
	elapsed := time.Since(start)
	if elapsed >= gap {
		t.Fatalf("different hosts serialized: elapsed %v want < %v", elapsed, gap)
	}
}

func TestGetThrottle_zeroMinGapNoSleep(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/a", []byte("a"))
	mt.SetBytes("www.gesetze-im-internet.de", "/b", []byte("b"))
	c := NewWithTransport(5*time.Second, 0, 1<<20, mt)

	start := time.Now()
	if _, _, _, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/a", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/b", "", ""); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("zero minGap slept: elapsed %v", elapsed)
	}
}

func TestGetThrottle_negativeMinGapNoSleep(t *testing.T) {
	mt := httpmock.New()
	mt.SetBytes("www.gesetze-im-internet.de", "/a", []byte("a"))
	mt.SetBytes("www.gesetze-im-internet.de", "/b", []byte("b"))
	c := NewWithTransport(5*time.Second, -time.Millisecond, 1<<20, mt)

	start := time.Now()
	if _, _, _, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/a", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.Get(context.Background(), "https://www.gesetze-im-internet.de/b", "", ""); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("negative minGap slept: elapsed %v", elapsed)
	}
}
