package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/service"
)

// Run executes a CLI command against the in-process service.
func Run(ctx context.Context, svc *service.Service, args []string) int {
	if len(args) < 1 {
		PrintUsage()
		return 2
	}
	switch args[0] {
	case "resolve":
		opts, rest := peelIncludeFlags(args[1:])
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: resolve [--include=past,linked,proof] <query>")
			return 2
		}
		res, err := svc.Resolve(ctx, strings.Join(rest, " "), opts)
		return printJSON(res, err)
	case "freshness":
		opts, rest := peelIncludeFlags(args[1:])
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: freshness [--include=past,linked,proof] <id-or-abbr>")
			return 2
		}
		res, err := svc.Freshness(ctx, rest[0], opts)
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
		opts, rest := peelIncludeFlags(args[1:])
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: export [--include=past,linked] <query> [formats: hierarchical|chunked|flat|normtext]")
			return 2
		}
		var formats []string
		q := rest[0]
		if len(rest) > 1 {
			formats = strings.Split(rest[1], ",")
		}
		res, err := svc.ExportText(ctx, q, formats, opts)
		return printJSON(res, err)
	case "bundle":
		opts, rest := peelBundleFlags(args[1:])
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: bundle [--include=past] [--compose] <query> [formats: hierarchical|chunked|flat|normtext]")
			return 2
		}
		var formats []string
		q := rest[0]
		if len(rest) > 1 {
			formats = strings.Split(rest[1], ",")
		}
		res, err := svc.ExportOperativeBundle(ctx, q, formats, opts)
		return printJSON(res, err)
	case "index":
		opts, rest := peelIndexFlags(args[1:])
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: index [--include=past] [--section=§1,§2] <query>")
			return 2
		}
		res, err := svc.ExportIndexChunks(ctx, strings.Join(rest, " "), opts)
		return printJSON(res, err)
	default:
		if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
			PrintUsage()
			return 0
		}
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		PrintUsage()
		return 2
	}
}

func peelIncludeFlags(args []string) (service.IncludeOpts, []string) {
	var opts service.IncludeOpts
	var rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "--include=") {
			part := service.ParseInclude(strings.TrimPrefix(a, "--include="))
			opts.Past = opts.Past || part.Past
			opts.Linked = opts.Linked || part.Linked
			opts.Proof = opts.Proof || part.Proof
			continue
		}
		if a == "--include" {
			continue
		}
		rest = append(rest, a)
	}
	return opts, rest
}

func peelBundleFlags(args []string) (service.BundleOpts, []string) {
	var opts service.BundleOpts
	var rest []string
	for _, a := range args {
		switch {
		case a == "--compose":
			opts.Compose = true
		case strings.HasPrefix(a, "--include="):
			part := service.ParseInclude(strings.TrimPrefix(a, "--include="))
			opts.Past = opts.Past || part.Past
		case a == "--include":
			continue
		default:
			rest = append(rest, a)
		}
	}
	return opts, rest
}

func peelIndexFlags(args []string) (service.IndexOpts, []string) {
	var opts service.IndexOpts
	var rest []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--section="):
			refs := export.ParseSectionRefs(strings.TrimPrefix(a, "--section="))
			opts.Sections = append(opts.Sections, refs...)
		case a == "--section":
			continue
		case strings.HasPrefix(a, "--include="):
			part := service.ParseInclude(strings.TrimPrefix(a, "--include="))
			opts.Past = opts.Past || part.Past
		case a == "--include":
			continue
		default:
			rest = append(rest, a)
		}
	}
	return opts, rest
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
