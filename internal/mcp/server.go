// Package mcp implements a minimal MCP stdio server (JSON-RPC 2.0) without an external SDK.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcErr `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeStdio runs until stdin closes.
func ServeStdio(ctx context.Context, svc *service.Service) error {
	in := bufio.NewReader(os.Stdin)
	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = bytesTrim(line)
		if len(line) == 0 {
			continue
		}
		var req rpcReq
		if err := json.Unmarshal(line, &req); err != nil {
			writeResp(rpcResp{JSONRPC: "2.0", ID: nil, Error: &rpcErr{Code: -32700, Message: "parse error"}})
			continue
		}
		resp := handle(ctx, svc, req)
		writeResp(resp)
	}
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
			r.Error = &rpcErr{Code: -32602, Message: err.Error()}
			return r
		}
		out, err := callTool(ctx, svc, p.Name, p.Arguments)
		if err != nil {
			r.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
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
		tool("resolve_law", "Resolve a German federal law by abbreviation/title and attach BGBl freshness", map[string]any{
			"query":   map[string]any{"type": "string"},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past, linked"},
		}, []string{"query"}),
		tool("law_freshness", "Freshness status for a known law id or abbreviation", map[string]any{
			"id":      map[string]any{"type": "string"},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past, linked"},
		}, []string{"id"}),
		tool("list_stale_laws", "List laws currently confirmed_stale", map[string]any{}, nil),
		tool("force_recheck", "Force out-of-band re-verification", map[string]any{
			"id": map[string]any{"type": "string"},
		}, nil),
		tool("sync_status", "Sync and freshness readiness status", map[string]any{}, nil),
		tool("export_law_text", "Export law text in RAG-ready formats (hierarchical|chunked|flat|normtext)", map[string]any{
			"query":   map[string]any{"type": "string"},
			"formats": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"include": map[string]any{"type": "string", "description": "Comma-separated: past, linked"},
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
	default:
		return nil, fmt.Errorf("unknown tool %s", name)
	}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
