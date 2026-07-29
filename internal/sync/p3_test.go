package sync

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestLoop_exitsOnCtxCancel(t *testing.T) {
	mt := httpmock.New()
	mt.Set("www.gesetze-im-internet.de", "/gii-toc.xml", httpmock.Response{
		BlockUntilContext: true,
	})
	cfg := testCFG()
	cfg.HTTPTimeout = 2 * time.Second
	cfg.TOCInterval = 20 * time.Millisecond
	st := openTestStore(t)
	httpClient := httpx.NewWithTransport(cfg.HTTPTimeout, time.Millisecond, 1<<20, mt)
	o := &Orchestrator{
		CFG:   cfg,
		Store: st,
		HTTP:  httpClient,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	o.loop(ctx, cfg.TOCInterval, o.RunTOC)

	time.Sleep(30 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		o.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not exit within 1s after ctx cancel")
	}
}

func TestSourceTimeout_cancelsBlockingRun(t *testing.T) {
	mt := httpmock.New()
	mt.Set("www.gesetze-im-internet.de", "/gii-toc.xml", httpmock.Response{
		BlockUntilContext: true,
	})
	cfg := testCFG()
	cfg.HTTPTimeout = 50 * time.Millisecond
	o := newTestOrchestrator(t, mt)
	o.CFG = cfg
	o.HTTP = httpx.NewWithTransport(cfg.HTTPTimeout, time.Millisecond, 1<<20, mt)

	ctx := context.Background()
	cctx, ccancel := o.sourceTimeout(ctx)
	defer ccancel()

	start := time.Now()
	err := o.RunTOC(cctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected RunTOC to fail on source timeout")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("RunTOC took %v, want prompt cancel near 2*HTTPTimeout", elapsed)
	}
}

func TestReconcile_respectsCtxCancel(t *testing.T) {
	o := newTestOrchestrator(t, httpmock.New())
	laws := make([]domain.Law, 0, 32)
	for i := 0; i < 32; i++ {
		laws = append(laws, domain.Law{
			ID:           "law" + string(rune('a'+i)),
			Abbreviation: "L",
			Title:        "Law",
			GIIPath:      "x",
		})
	}
	if err := o.Store.UpsertLaws(laws); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := o.Reconcile(ctx)
	if err == nil {
		t.Fatal("expected reconcile to stop on canceled ctx")
	}
	if err != context.Canceled {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}
