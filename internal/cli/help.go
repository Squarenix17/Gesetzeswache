package cli

import (
	"fmt"
	"os"
)

// PrintUsage writes command help with examples to stderr.
func PrintUsage() {
	fmt.Fprintln(os.Stderr, "usage: gew <command> [options] [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	for _, c := range commandHelp {
		fmt.Fprintf(os.Stderr, "  %s\n", c.name)
		fmt.Fprintf(os.Stderr, "    %s\n", c.summary)
		for _, ex := range c.examples {
			fmt.Fprintf(os.Stderr, "    example: %s\n", ex)
		}
		fmt.Fprintln(os.Stderr)
	}
}

type commandDoc struct {
	name     string
	summary  string
	examples []string
}

var commandHelp = []commandDoc{
	{
		name:    "serve",
		summary: "Start HTTP API, MCP is separate (gew mcp)",
		examples: []string{
			"gew serve",
			"GEW_HTTP_ADDR=:9090 gew serve",
			"GEW_SHARED_SECRET=secret gew serve",
		},
	},
	{
		name:    "resolve",
		summary: "Resolve abbreviation or title to a law with freshness",
		examples: []string{
			`gew resolve ArbZG`,
			`gew resolve --include=linked "Mindestlohngesetz"`,
			`gew resolve --include=past,linked BGB`,
		},
	},
	{
		name:    "freshness",
		summary: "Freshness metadata for a law id or abbreviation",
		examples: []string{
			`gew freshness bgb`,
			`gew freshness --include=linked ArbZG`,
			`gew freshness --include=past,linked MiLoG`,
		},
	},
	{
		name:    "stale",
		summary: "List laws in confirmed_stale state",
		examples: []string{
			`gew stale`,
			`gew stale | jq '.data | length'`,
			`gew stale > /tmp/stale.json`,
		},
	},
	{
		name:    "recheck",
		summary: "Force feed sync and reconcile (optional law id)",
		examples: []string{
			`gew recheck`,
			`gew recheck bgb`,
			`gew recheck arbzg`,
		},
	},
	{
		name:    "sync-status",
		summary: "Catalog readiness and feed freshness timestamps",
		examples: []string{
			`gew sync-status`,
			`gew sync-status | jq '.data.catalog_ready'`,
			`gew sync-status | jq '.data.data_fresh'`,
		},
	},
	{
		name:    "export",
		summary: "Export law text (hierarchical|chunked|flat|normtext)",
		examples: []string{
			`gew export BGB hierarchical`,
			`gew export --include=linked ArbZG hierarchical,chunked`,
			`gew export MiLoG normtext,flat`,
		},
	},
	{
		name:    "bundle",
		summary: "Export Gesetz + current linked Verordnungen (unmixed; optional --compose display)",
		examples: []string{
			`gew bundle MiLoG normtext`,
			`gew bundle --compose MiLoG hierarchical`,
			`gew bundle --include=past MiLoG hierarchical`,
		},
	},
	{
		name:    "index",
		summary: "Flat ingest-ready chunks for parent + current linked Verordnungen (optional --section)",
		examples: []string{
			`gew index MiLoG`,
			`gew index MiLoG --section='§ 1'`,
			`gew index --include=past MiLoG --section='§ 1,§ 2'`,
		},
	},
	{
		name:    "mcp",
		summary: "Run MCP server on stdio (JSON-RPC tools)",
		examples: []string{
			`gew mcp`,
			`gew mcp < mcp-request.jsonl`,
			`echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | gew mcp`,
		},
	},
	{
		name:    "health",
		summary: "Probe local /healthz (for container HEALTHCHECK)",
		examples: []string{
			`gew health`,
			`GEW_HTTP_ADDR=:9090 gew health`,
			`GEW_HTTP_ADDR=127.0.0.1:8080 gew health`,
		},
	},
	{
		name:    "version",
		summary: "Print binary version",
		examples: []string{
			`gew version`,
			`gew version | tee /tmp/gew-version.txt`,
			`docker run --rm ghcr.io/org/gesetzeswache gew version`,
		},
	},
}
