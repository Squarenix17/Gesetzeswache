# gesetzeswache

BGBl-verified German federal statute resolution for any integrator — REST API, CLI, or MCP (stdio).

## What it is

**gesetzeswache** is a single Go binary that resolves German federal laws (abbreviation, title, or informal variant) and attaches **BGBl freshness metadata** before returning results. It verifies against official sources (Gesetze im Internet, BGBl feeds, ELI) on a schedule and on demand.

**Interfaces:** HTTP REST, local CLI, MCP over stdio.

**Optional text export:** On-demand export in `hierarchical`, `chunked`, or `flat` formats for RAG and indexing pipelines. No durable full-text corpus is stored — export fetches and formats text when requested.

**Verify-before-serve:** Matched resolve and export responses attach freshness metadata. Consumers must honor it.

## What it is not

- **Not an LLM** — no language model, embeddings, or chat layer. Pure resolution, freshness, and optional text export.
- **Not a statute database** — it maintains a lightweight catalog and sync state (embedded bbolt), not a permanent mirror of all law full text.
- **Not a substitute for legal advice** — it helps locate and verify official statute references; interpretation is out of scope.

## Quick start (binary)

```bash
go build -o bin/gesetzeswache ./cmd/gesetzeswache
./bin/gesetzeswache serve
```

The server listens on `:8080` by default. Use `GET /healthz` and `GET /readyz` for liveness and readiness.

## Configuration

All settings use the `GEW_` environment prefix. Essential variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `GEW_HTTP_ADDR` | `:8080` | HTTP listen address |
| `GEW_STORE_PATH` | `gesetzeswache.db` | Embedded bbolt catalog and sync state |
| `GEW_MATCH_THRESHOLD` | `0.75` | Fuzzy match threshold (0–1) |
| `GEW_FRESHNESS_MAX_AGE` | `6h` | Max age before sync data is too stale to confirm current |
| `GEW_SHARED_SECRET` | *(empty)* | Required for HTTP recheck; empty = fail-closed (401) |
| `GEW_VARIANTS_PATH` | `variants/variants.tsv` | TSV of informal names → law IDs |
| `GEW_ENABLE_EXPORT` | `true` | Enable on-demand text export |
| `GEW_REFUSE_EXPORT_STALE` | `false` | When `true`, refuse export if law is `confirmed_stale` |

Additional sync intervals (`GEW_TOC_INTERVAL`, `GEW_GII_FEED_INTERVAL`, `GEW_BGBL_FEED_INTERVAL`, `GEW_ELI_PROBE_INTERVAL`, etc.), source URLs, and tuning knobs are defined in [`internal/config/config.go`](internal/config/config.go).

## Docker

Published image: `ghcr.io/squarenix17/gesetzeswache:latest`

The image sets `GEW_VARIANTS_PATH=/variants/variants.tsv` and `GEW_STORE_PATH=/tmp/gesetzeswache.db`. Mount a volume for persistent store data:

```bash
docker run --rm -p 8080:8080 \
  -v gesetzeswache-data:/tmp \
  ghcr.io/squarenix17/gesetzeswache:latest
```

Override any `GEW_*` variable with `-e` as needed.

## Consumer contract

REST and CLI wrap payloads in a JSON envelope:

```json
{ "success": true, "data": { }, "error": null }
```

Any client — RAG pipeline, search index, agent tool, or application — must treat freshness as part of the API contract:

1. **Always read freshness on matched results** — for resolve/export use `data.freshness.state`; for `GET /v1/freshness` use `data.state`.
2. **Embed or serve as current only when** that state is `confirmed_current`.
3. **Quarantine or flag for manual review** when state is `confirmed_stale` or `uncertain`.
4. **Never index or serve law text while ignoring freshness metadata.**

Freshness states:

| State | Meaning |
|-------|---------|
| `confirmed_current` | Verified current against BGBl/GII signals |
| `confirmed_stale` | Known newer publication; do not treat as current |
| `uncertain` | Insufficient or conflicting signals; do not auto-promote |

Optional belt-and-suspenders: set `GEW_REFUSE_EXPORT_STALE=true` so the server rejects export requests for `confirmed_stale` laws.

## Interface cheat sheet

### REST

| Method | Path | Notes |
|--------|------|-------|
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness (sync initialized) |
| GET, POST | `/v1/resolve?q=` | Resolve law + freshness |
| GET | `/v1/freshness?id=` | Freshness for a known ID (or `q=`) |
| GET | `/v1/stale` | List `confirmed_stale` laws |
| GET | `/v1/sync/status` | Sync and readiness status |
| GET, POST | `/v1/export?q=&format=hierarchical\|chunked\|flat` | On-demand text export |
| POST | `/v1/recheck` | Force re-verification (auth required) |

### CLI

```bash
gesetzeswache serve                              # HTTP API + background sync
gesetzeswache resolve <query>                  # Resolve law + freshness
gesetzeswache freshness <id>                   # Freshness only
gesetzeswache stale                            # List confirmed_stale laws
gesetzeswache recheck [id]                     # Force re-verification
gesetzeswache sync-status                      # Sync readiness
gesetzeswache export <query> [format]          # hierarchical | chunked | flat
gesetzeswache mcp                              # MCP stdio server
```

### MCP (stdio)

Run `gesetzeswache mcp` and connect your MCP client. Tools:

| Tool | Purpose |
|------|---------|
| `resolve_law` | Resolve by abbreviation/title + freshness |
| `export_law_text` | Export text (`hierarchical`, `chunked`, `flat`) |
| `law_freshness` | Freshness for a known law ID or abbreviation |
| `list_stale_laws` | List laws currently `confirmed_stale` |
| `force_recheck` | Force out-of-band re-verification |
| `sync_status` | Sync and freshness readiness |

## Authentication (recheck)

`POST /v1/recheck` is **fail-closed**: set `GEW_SHARED_SECRET` and send the same value in the `X-Gesetzeswache-Token` header. If the secret is empty or the header does not match, the API returns `401`.

CLI `recheck` and MCP `force_recheck` call the local service process directly (no HTTP token); protect process access in production deployments.

## Variants

Informal and alternate names (e.g. `Zivilgesetzbuch` → `bgb`) live in [`variants/variants.tsv`](variants/variants.tsv) as TSV: `variant<TAB>law_id`. Override the path with `GEW_VARIANTS_PATH` or extend the file for your deployment.

## License

MIT — see [LICENSE](LICENSE).
