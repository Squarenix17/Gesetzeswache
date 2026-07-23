# gesetzeswache

BGBl-verified German federal statute resolution for API, CLI, and MCP consumers.

## Quick start

```bash
go build -o bin/gesetzeswache ./cmd/gesetzeswache
./bin/gesetzeswache serve
```

Environment variables use the `GEW_` prefix (see `internal/config`).

### Commands

- `serve` — HTTP API + background sync
- `resolve <query>` — resolve law + freshness
- `freshness <id>` — freshness only
- `stale` — list confirmed_stale laws
- `recheck [id]` — force re-verification
- `sync-status` — sync readiness
- `export <query> [hierarchical,chunked,flat]` — RAG text export
- `mcp` — MCP stdio server

Verify-before-serve: every resolve/export attaches BGBl freshness; never silently serves without freshness metadata.
