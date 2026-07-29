package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/clienterr"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/httpx"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/service"
	"github.com/Squarenix17/gesetzeswache/internal/store"
	"github.com/Squarenix17/gesetzeswache/internal/sync"
	"github.com/Squarenix17/gesetzeswache/internal/test/httpmock"
)

func TestReadLineBounded_normal(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(`{"jsonrpc":"2.0","id":1}` + "\n"))
	line, over, _, err := readLineBounded(r, maxMessageSize)
	if err != nil || over {
		t.Fatalf("err=%v over=%v", err, over)
	}
	if !strings.Contains(string(line), "jsonrpc") {
		t.Fatalf("line %q", line)
	}
}

func TestReadLineBounded_overLimit(t *testing.T) {
	payload := strings.Repeat("x", maxMessageSize+1) + "\n"
	r := bufio.NewReader(strings.NewReader(payload))
	_, over, _, err := readLineBounded(r, maxMessageSize)
	if err != nil || !over {
		t.Fatalf("err=%v over=%v want overLimit", err, over)
	}
}

func TestReadLineBounded_overLimitExtractsID(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`{"jsonrpc":"2.0","id":42,"method":"ping"}`)
	buf.WriteString(strings.Repeat("x", maxMessageSize))
	buf.WriteByte('\n')

	r := bufio.NewReader(&buf)
	_, over, prefix, err := readLineBounded(r, maxMessageSize)
	if err != nil || !over {
		t.Fatalf("err=%v over=%v", err, over)
	}
	if extractID(prefix) != float64(42) {
		t.Fatalf("id from prefix=%v want 42", extractID(prefix))
	}
}

func TestReadLineBounded_followedByValidMessage(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(strings.Repeat("y", maxMessageSize+1))
	buf.WriteByte('\n')
	buf.WriteString(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	buf.WriteByte('\n')

	r := bufio.NewReader(&buf)
	_, over, _, err := readLineBounded(r, maxMessageSize)
	if err != nil || !over {
		t.Fatalf("first line: err=%v over=%v", err, over)
	}
	line, over, _, err := readLineBounded(r, maxMessageSize)
	if err != nil || over {
		t.Fatalf("second line: err=%v over=%v", err, over)
	}
	var msg map[string]any
	if err := json.Unmarshal(line, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["method"] != "ping" {
		t.Fatalf("msg %v", msg)
	}
}

func testMCPService(t *testing.T) *service.Service {
	t.Helper()
	reg := metrics.NewRegistry()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{
		FreshnessMaxAge: 6 * time.Hour,
		EnableExport:    true,
		ExportCacheMax:  8,
		MatchThreshold:  0.75,
		GIIBase:         "https://www.gesetze-im-internet.de",
		RequestMinGap:   time.Millisecond,
		HTTPTimeout:     5 * time.Second,
	}
	mt := httpmock.New()
	httpClient := httpx.NewWithTransport(cfg.HTTPTimeout, cfg.RequestMinGap, 1<<20, mt)
	eng := search.NewEngine()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	orch := &sync.Orchestrator{CFG: cfg, Store: st, HTTP: httpClient, Search: eng, Log: log, Metrics: reg}
	return &service.Service{
		CFG: cfg, Store: st, Search: eng, Sync: orch, HTTP: httpClient,
		Export: export.NewCache(8), Log: log, Metrics: reg,
	}
}

func toolCallResultText(resp rpcResp) string {
	m, ok := resp.Result.(map[string]any)
	if !ok {
		return ""
	}
	switch content := m["content"].(type) {
	case []map[string]any:
		if len(content) == 0 {
			return ""
		}
		text, _ := content[0]["text"].(string)
		return text
	case []any:
		if len(content) == 0 {
			return ""
		}
		item, _ := content[0].(map[string]any)
		text, _ := item["text"].(string)
		return text
	default:
		return ""
	}
}

func TestHandle_toolsCall_internalErrorMasked(t *testing.T) {
	svc := testMCPService(t)
	_ = svc.Store.Close()

	resp := handle(context.Background(), svc, rpcReq{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"list_stale_laws","arguments":{}}`),
	})
	text := toolCallResultText(resp)
	if text != clienterr.Internal {
		t.Fatalf("text=%q want %q", text, clienterr.Internal)
	}
	if strings.Contains(text, "bbolt") || strings.Contains(text, "closed") {
		t.Fatalf("leaked internal detail: %q", text)
	}
}

func TestHandle_toolsCall_intentionalErrors(t *testing.T) {
	svc := testMCPService(t)

	resp := handle(context.Background(), svc, rpcReq{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"law_freshness","arguments":{"id":"no-such-law-xyz"}}`),
	})
	if text := toolCallResultText(resp); text != "law not found" {
		t.Fatalf("freshness text=%q", text)
	}

	longQ := strings.Repeat("ä", service.MaxQueryRunes+1)
	resp = handle(context.Background(), svc, rpcReq{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"resolve_law","arguments":{"query":"` + longQ + `"}}`),
	})
	if text := toolCallResultText(resp); text != "query too long" {
		t.Fatalf("resolve text=%q", text)
	}
}

func TestHandle_toolsCall_invalidParamsGeneric(t *testing.T) {
	svc := testMCPService(t)
	resp := handle(context.Background(), svc, rpcReq{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  json.RawMessage(`not-json`),
	})
	if resp.Error == nil || resp.Error.Message != clienterr.Internal {
		t.Fatalf("error=%v", resp.Error)
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("code=%d", resp.Error.Code)
	}
}
