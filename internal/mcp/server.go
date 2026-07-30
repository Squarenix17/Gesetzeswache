// Package mcp implements a minimal MCP stdio server (JSON-RPC 2.0) without an external SDK.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/clienterr"
	"github.com/Squarenix17/gesetzeswache/internal/service"
)

const (
	maxMessageSize  = 4 << 20 // 4 MiB
	idExtractPrefix = 4 << 10 // 4 KiB for best-effort id extraction on oversized lines
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      any     `json:"id"`
	Result  any     `json:"result,omitempty"`
	Error   *rpcErr `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// readLineBounded reads a single line up to max bytes. When exceeded, the remainder of the
// line is discarded so the reader stays aligned on line boundaries. prefix holds the first
// idExtractPrefix bytes of the line (for JSON-RPC id recovery on oversized messages).
func readLineBounded(r *bufio.Reader, max int) (line []byte, overLimit bool, prefix []byte, err error) {
	var buf bytes.Buffer
	var prefixBuf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				if buf.Len() == 0 && prefixBuf.Len() == 0 {
					return nil, false, nil, err
				}
				return buf.Bytes(), overLimit, prefixBuf.Bytes(), nil
			}
			return nil, false, nil, err
		}
		if b == '\n' {
			return buf.Bytes(), overLimit, prefixBuf.Bytes(), nil
		}
		if prefixBuf.Len() < idExtractPrefix {
			_ = prefixBuf.WriteByte(b)
		}
		if overLimit {
			continue
		}
		if buf.Len() >= max {
			overLimit = true
			continue
		}
		_ = buf.WriteByte(b)
	}
}

// ServeStdio runs until stdin closes.
func ServeStdio(ctx context.Context, svc *service.Service) error {
	in := bufio.NewReader(os.Stdin)
	for {
		line, overLimit, prefix, err := readLineBounded(in, maxMessageSize)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if overLimit {
			writeResp(rpcResp{
				JSONRPC: "2.0",
				ID:      extractID(prefix),
				Error:   &rpcErr{Code: -32700, Message: "parse error: message too large"},
			})
			continue
		}
		line = bytesTrim(line)
		if len(line) == 0 {
			continue
		}
		var req rpcReq
		if err := json.Unmarshal(line, &req); err != nil {
			writeResp(rpcResp{JSONRPC: "2.0", ID: extractID(line), Error: &rpcErr{Code: -32700, Message: "parse error"}})
			continue
		}
		resp := handle(ctx, svc, req)
		writeResp(resp)
	}
}

func extractID(line []byte) any {
	var partial struct {
		ID any `json:"id"`
	}
	if err := json.Unmarshal(line, &partial); err == nil {
		return partial.ID
	}
	// Best-effort on truncated oversized lines: trim suffix until JSON parses.
	for end := len(line); end > 0; end-- {
		if err := json.Unmarshal(line[:end], &partial); err == nil {
			return partial.ID
		}
	}
	return nil
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

func writeResp(r rpcResp) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(r)
}

func logError(svc *service.Service, msg string, err error) {
	if err == nil {
		return
	}
	var log *slog.Logger
	if svc != nil {
		log = svc.Log
	}
	if log == nil {
		return
	}
	log.Error(msg, "err", err)
}

func handle(ctx context.Context, svc *service.Service, req rpcReq) rpcResp {
	r := rpcResp{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		r.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "gesetzeswache", "version": "0.2.0"},
		}
	case "notifications/initialized", "initialized":
		r.Result = map[string]any{}
	case "tools/list":
		r.Result = map[string]any{"tools": tools()}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			logError(svc, "mcp tools/call params", err)
			r.Error = &rpcErr{Code: -32602, Message: clienterr.Internal}
			return r
		}
		out, err := callTool(ctx, svc, p.Name, p.Arguments)
		if err != nil {
			msg := clienterr.Sanitize(err)
			if msg == clienterr.Internal {
				logError(svc, "mcp tools/call", err)
			}
			r.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": msg}},
				"isError": true,
			}
			return r
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		r.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(b)}},
		}
	case "ping":
		r.Result = map[string]any{}
	default:
		r.Error = &rpcErr{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
	return r
}

func tools() []map[string]any {
	return []map[string]any{
		tool("resolve_law", "Resolve a German federal law by abbreviation or title (e.g. \"ArbZG\", \"BGB\") and attach BGBl freshness. Args: query (string, required) — law abbreviation, slug, or title fragment; include (optional string) — comma-separated past,linked to expand linked instruments and past edges.", map[string]any{
			"query":   map[string]any{"type": "string"},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past, linked"},
		}, []string{"query"}),
		tool("law_freshness", "Freshness status for a known law. Args: id (string, required) — canonical law id or abbreviation (e.g. \"bgb\", \"ArbZG\"); include (optional string) — comma-separated past,linked.", map[string]any{
			"id":      map[string]any{"type": "string"},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past, linked"},
		}, []string{"id"}),
		tool("list_stale_laws", "List laws currently in confirmed_stale state. No arguments.", map[string]any{}, nil),
		tool("force_recheck", "Force out-of-band re-verification (feeds + reconcile). Args: id (optional string) — target law id or abbreviation; omit to recheck all.", map[string]any{
			"id": map[string]any{"type": "string"},
		}, nil),
		tool("sync_status", "Sync and freshness readiness (catalog_ready, data_fresh, last feed timestamps). No arguments.", map[string]any{}, nil),
		tool("export_law_text", "Export law text in RAG-ready formats. Args: query (string, required) — law id, abbreviation, or title; formats (optional array of strings) — hierarchical, chunked, flat, normtext (default hierarchical); include (optional string) — comma-separated past,linked.", map[string]any{
			"query":   map[string]any{"type": "string"},
			"formats": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past, linked"},
		}, []string{"query"}),
		tool("export_law_bundle", "Export a Gesetz plus current linked Verordnungen as separate indexable members (parent + operative[]). Parent text is never merged with VO body. Args: query (required); formats (optional); include (optional past); compose (optional bool) — display-only hierarchical callout, do not embed.", map[string]any{
			"query":   map[string]any{"type": "string"},
			"formats": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past"},
			"compose": map[string]any{"type": "boolean", "description": "Emit display-only composed hierarchical; not for vector ingest"},
		}, []string{"query"}),
	}
}

func tool(name, desc string, props map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props}
	if required != nil {
		schema["required"] = required
	}
	return map[string]any{
		"name":        name,
		"description": desc,
		"inputSchema": schema,
	}
}

func callTool(ctx context.Context, svc *service.Service, name string, args map[string]any) (any, error) {
	opts := service.ParseInclude(str(args["include"]))
	switch name {
	case "resolve_law":
		return svc.Resolve(ctx, str(args["query"]), opts)
	case "law_freshness":
		return svc.Freshness(ctx, str(args["id"]), opts)
	case "list_stale_laws":
		return svc.ListStale(ctx)
	case "force_recheck":
		err := svc.ForceRecheck(ctx, str(args["id"]))
		return map[string]string{"status": "ok"}, err
	case "sync_status":
		return svc.SyncStatus(ctx)
	case "export_law_text":
		var formats []string
		if v, ok := args["formats"]; ok {
			switch t := v.(type) {
			case []any:
				for _, x := range t {
					formats = append(formats, fmt.Sprint(x))
				}
			case []string:
				formats = t
			case string:
				if t != "" {
					formats = strings.Split(t, ",")
				}
			}
		}
		return svc.ExportText(ctx, str(args["query"]), formats, opts)
	case "export_law_bundle":
		var formats []string
		if v, ok := args["formats"]; ok {
			switch t := v.(type) {
			case []any:
				for _, x := range t {
					formats = append(formats, fmt.Sprint(x))
				}
			case []string:
				formats = t
			case string:
				if t != "" {
					formats = strings.Split(t, ",")
				}
			}
		}
		bopts := service.BundleOpts{Past: opts.Past, Compose: boolArg(args["compose"])}
		return svc.ExportOperativeBundle(ctx, str(args["query"]), formats, bopts)
	default:
		return nil, fmt.Errorf("unknown tool %s", name)
	}
}

func boolArg(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true") || t == "1"
	default:
		return false
	}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
