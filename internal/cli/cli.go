package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

// Run executes a CLI command against the in-process service.
func Run(ctx context.Context, svc *service.Service, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: gew <serve|resolve|freshness|stale|recheck|sync-status|export|mcp> ...")
		return 2
	}
	switch args[0] {
	case "resolve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: resolve <query>")
			return 2
		}
		res, err := svc.Resolve(ctx, strings.Join(args[1:], " "))
		return printJSON(res, err)
	case "freshness":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: freshness <id-or-abbr>")
			return 2
		}
		res, err := svc.Freshness(ctx, args[1])
		return printJSON(res, err)
	case "stale":
		res, err := svc.ListStale(ctx)
		return printJSON(res, err)
	case "recheck":
		id := ""
		if len(args) > 1 {
			id = args[1]
		}
		err := svc.ForceRecheck(ctx, id)
		return printJSON(map[string]string{"status": "ok"}, err)
	case "sync-status":
		res, err := svc.SyncStatus(ctx)
		return printJSON(res, err)
	case "export":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: export <query> [formats comma-separated]")
			return 2
		}
		var formats []string
		q := args[1]
		if len(args) > 2 {
			formats = strings.Split(args[2], ",")
		}
		res, err := svc.ExportText(ctx, q, formats)
		return printJSON(res, err)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		return 2
	}
}

func printJSON(v any, err error) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err != nil {
		_ = enc.Encode(service.Envelope{Success: false, Data: v, Error: strPtr(err.Error())})
		return 1
	}
	_ = enc.Encode(service.Envelope{Success: true, Data: v, Error: nil})
	return 0
}

func strPtr(s string) *string { return &s }
