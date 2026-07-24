<a id="readme-top"></a>
<br />
<div align="center">

  <h1 align="center">Gesetzeswache</h1>
  <h3 align="center"><strong><code>gew</code></strong> the CLI and container binary</h3>

  <p align="center">
    Bundesgesetzblatt-verified German federal statute resolution for any integrator.
    <br />
    REST API · CLI · MCP (stdio) · optional on-demand text export.
    <br />
    <br />
    <a href="#usage">Usage</a>
    &middot;
    <a href="#getting-started">Getting Started</a>
    &middot;
    <a href="https://github.com/Squarenix17/gesetzeswache/issues">Report Bug</a>
    &middot;
    <a href="https://github.com/Squarenix17/gesetzeswache/issues">Request Feature</a>
  </p>
</div>

<!-- TABLE OF CONTENTS -->
<details>
  <summary>Table of Contents</summary>
  <ol>
    <li><a href="#about-the-project">About The Project</a></li>
    <li><a href="#usage">Usage</a></li>
    <li>
      <a href="#getting-started">Getting Started</a>
      <ul>
        <li><a href="#prerequisites">Prerequisites</a></li>
        <li><a href="#installation">Installation</a></li>
        <li><a href="#configuration">Configuration</a></li>
        <li><a href="#docker">Docker</a></li>
      </ul>
    </li>
    <li><a href="#roadmap">Roadmap</a></li>
    <li><a href="#license">License</a></li>
    <li><a href="#contact">Contact</a></li>
  </ol>
</details>

<!-- ABOUT THE PROJECT -->
## About The Project

**gesetzeswache** (`gew`) is a single Go binary that resolves German federal laws by abbreviation, title, or informal variant, and attaches **BGBl freshness metadata** before returning results. It verifies against official sources (Gesetze im Internet, BGBl feeds, ELI) on a schedule and on demand.

**Verify-before-serve:** matched resolve and export responses attach freshness metadata. Consumers must honor it.

### What it is

- **Interfaces:** HTTP REST, local CLI (`gew`), MCP over stdio
- **Optional text export:** on-demand `hierarchical`, `chunked`, `flat`, or `normtext` formats for RAG and indexing pipelines, no durable full-text corpus is stored
- **Env prefix:** `GEW_*` (matches the `gew` binary nickname)

### What it is not

- **Not an LLM:** no language model, embeddings, or chat layer
- **Not a statute mirror:** lightweight catalog and sync state (embedded bbolt), not a permanent copy of all law full text
- **Not legal advice:** helps locate and verify official statute references; interpretation is out of scope

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- USAGE -->
## Usage

### Consumer contract

REST and CLI wrap payloads in a JSON envelope:

```json
{ "success": true, "data": { }, "error": null }
```

Any client: RAG pipeline, search index, agent tool, or application, must treat freshness as part of the API contract:

1. **Always read freshness on matched results**, resolve/export: `data.freshness.state`; `GET /v1/freshness`: `data.state`
2. **Embed or serve as current only when** state is `confirmed_current`, confidence is high, and Stand `parse_ok` is true (amendment-by-reference laws may otherwise look current while a linked Verordnung moved)
3. **Quarantine or flag for manual review** when state is `confirmed_stale` or `uncertain`
4. **Never index or serve law text while ignoring freshness metadata**

| State | Meaning | Consumer action |
|-------|---------|-----------------|
| `confirmed_current` | Verified current against BGBl/GII signals | Safe to treat as current |
| `confirmed_stale` | Known newer publication | Quarantine / manual review |
| `uncertain` | Insufficient or conflicting signals | Quarantine / manual review |

Optional: set `GEW_REFUSE_EXPORT_STALE=true` so the server rejects export for `confirmed_stale` laws.

### REST

| Method | Path | Notes |
|--------|------|-------|
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness (sync initialized) |
| GET | `/metrics` | Prometheus text metrics (unauthenticated) |
| GET, POST | `/v1/resolve?q=` | Resolve law + freshness |
| GET | `/v1/freshness?id=` | Freshness for a known ID (or `q=`) |
| GET | `/v1/stale` | List `confirmed_stale` laws |
| GET | `/v1/sync/status` | Sync and readiness status |
| GET, POST | `/v1/export?q=&format=hierarchical\|chunked\|flat\|normtext` | On-demand text export |
| POST | `/v1/recheck` | Force re-verification (auth required) |

```bash
curl 'http://127.0.0.1:8080/v1/resolve?q=BGB'
curl 'http://127.0.0.1:8080/v1/export?q=BGB&format=hierarchical'
curl 'http://127.0.0.1:8080/v1/sync/status'
curl -s 'http://127.0.0.1:8080/metrics' | head
```

### CLI

```bash
gew serve                    # HTTP API + background sync
gew resolve BGB              # Resolve law + freshness
gew freshness bgb            # Freshness only
gew stale                    # List confirmed_stale laws
gew recheck bgb              # Force re-verification
gew sync-status              # Sync readiness
gew export BGB hierarchical  # On-demand text export
gew mcp                      # MCP stdio server
```

### MCP (stdio)

Run `gew mcp` and connect your MCP client.

| Tool | Purpose |
|------|---------|
| `resolve_law` | Resolve by abbreviation/title + freshness |
| `export_law_text` | Export text (`hierarchical`, `chunked`, `flat`, `normtext`) |
| `law_freshness` | Freshness for a known law ID or abbreviation |
| `list_stale_laws` | List laws currently `confirmed_stale` |
| `force_recheck` | Force out-of-band re-verification |
| `sync_status` | Sync and freshness readiness |

### Authentication (recheck)

`POST /v1/recheck` is **fail-closed**: set `GEW_SHARED_SECRET` and send the same value in the `X-Gesetzeswache-Token` header. Empty secret or wrong header → `401`.

CLI `recheck` and MCP `force_recheck` call the local process directly (no HTTP token); protect process access in production.

### Variants

Informal names (e.g. `Zivilgesetzbuch` → `bgb`) live in [`variants/variants.tsv`](variants/variants.tsv) as TSV: `variant<TAB>law_id`. Override with `GEW_VARIANTS_PATH`.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- GETTING STARTED -->
## Getting Started

### Prerequisites

- **Go 1.21+:** to build from source
- **Docker:** (optional) to run the published container image
- **Network:** outbound HTTPS to GII and `recht.bund.de` for sync and export

### Installation

1. Clone the repo
   ```sh
   git clone https://github.com/Squarenix17/gesetzeswache.git
   cd gesetzeswache
   ```
2. Build the binary
   ```sh
   go build -o bin/gew ./cmd/gesetzeswache
   ```
3. Run the server
   ```sh
   ./bin/gew serve
   ```
4. Check health
   ```sh
   curl http://127.0.0.1:8080/healthz
   curl http://127.0.0.1:8080/readyz
   ```

The server listens on `:8080` by default.

### Configuration

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

Additional sync intervals, source URLs, and tuning knobs: [`internal/config/config.go`](internal/config/config.go).

### Docker

Image: `ghcr.io/squarenix17/gesetzeswache:latest` (pinned tags on [releases](https://github.com/Squarenix17/gesetzeswache/releases))

```bash
docker compose up -d
curl http://127.0.0.1:8080/readyz
```

Stop:

```bash
docker compose down
```

Override `GEW_*` in [`docker-compose.yml`](docker-compose.yml) under `environment:`, or run manually:

```bash
docker run --rm -p 8080:8080 -v gew-data:/tmp ghcr.io/squarenix17/gesetzeswache:latest
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- ROADMAP -->
## Roadmap

- [x] Core resolve + BGBl freshness evaluator
- [x] REST, CLI (`gew`), and MCP interfaces
- [x] On-demand text export (hierarchical / chunked / flat)
- [x] Docker image + GHCR publish
- [x] Export format quality (`kind` / `section_ref` / `normtext` for RAG)
- [x] Integration tests with mocked GII/BGBl fixtures
- [x] Bulk Stand refresh for full catalog
- [x] Metrics / observability endpoints

See the [open issues](https://github.com/Squarenix17/gesetzeswache/issues) for proposed features and known issues.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- LICENSE -->
## License

Distributed under the MIT License. See [`LICENSE`](LICENSE) for more information.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTACT -->
## Contact

Gabriele Ughetto - gabriele.ughetto@ovgu.de - gabriele.ughetto2000@gmail.com \
Telegram - [@Squarenix](https://t.me/squarenix)

Issues: [github.com/Squarenix17/gesetzeswache/issues](https://github.com/Squarenix17/gesetzeswache/issues)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- MARKDOWN LINKS & IMAGES -->
[contributors-shield]: https://img.shields.io/github/contributors/Squarenix17/gesetzeswache.svg?style=for-the-badge
[contributors-url]: https://github.com/Squarenix17/gesetzeswache/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/Squarenix17/gesetzeswache.svg?style=for-the-badge
[forks-url]: https://github.com/Squarenix17/gesetzeswache/network/members
[stars-shield]: https://img.shields.io/github/stars/Squarenix17/gesetzeswache.svg?style=for-the-badge
[stars-url]: https://github.com/Squarenix17/gesetzeswache/stargazers
[issues-shield]: https://img.shields.io/github/issues/Squarenix17/gesetzeswache.svg?style=for-the-badge
[issues-url]: https://github.com/Squarenix17/gesetzeswache/issues
[license-shield]: https://img.shields.io/github/license/Squarenix17/gesetzeswache.svg?style=for-the-badge
[license-url]: https://github.com/Squarenix17/gesetzeswache/blob/main/LICENSE
[go-shield]: https://img.shields.io/badge/Go-1.21-00ADD8?style=for-the-badge&logo=go&logoColor=white
[go-url]: https://go.dev/
