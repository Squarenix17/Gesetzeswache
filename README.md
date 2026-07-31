
<!-- PROJECT LOGO / TITLE -->
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
    <li><a href="#license">License</a></li>
    <li><a href="#contact">Contact</a></li>
  </ol>
</details>

<!-- ABOUT THE PROJECT -->
## About The Project

**gesetzeswache** (`gew`) is a single Go binary that finds German federal laws by abbreviation, title, or informal name, and attaches **freshness metadata** from official sources before returning a result.

Typical use: a RAG pipeline, chatbot, or internal tool needs to know *which* law a user meant (e.g. `MiLoG`, `Mindestlohngesetz`, `BGB`) and whether that text is still safe to treat as current.

It syncs a lightweight catalog from [Gesetze im Internet](https://www.gesetze-im-internet.de/) (GII) and Bundesgesetzblatt (BGBl) feeds, then exposes three interfaces:

| Interface | When to use it |
|-----------|----------------|
| **REST** (`gew serve`) | HTTP clients, Docker, production services |
| **CLI** (`gew …`) | Scripts, local debugging, one-off exports |
| **MCP** (`gew mcp`) | AI agents over stdio JSON-RPC |

### What it does

- Resolves a query to a canonical law id (e.g. `milog`) with optional suggestions on weak matches
- Attaches **fail-closed freshness** (`confirmed_current` / `confirmed_stale` / `uncertain`)
- On demand, exports statute text in several formats (no full-text corpus is stored permanently)
- For amendment-by-reference laws (e.g. MiLoG + Mindestlohn-Verordnung), can return **parent + current linked ordinances** in one call (`bundle` / `index`)

### What it is not

- **Not an LLM** — no chat, embeddings, or answer generation
- **Not a full statute archive** — text is fetched on demand from GII; only catalog + sync state live in a local bbolt DB
- **Not legal advice** — it helps locate and check references; interpretation is your responsibility

### Important limitations (read before integrating)

These are intentional or known behaviours — they are easy to misread as bugs:

1. **`uncertain` is normal for some laws.** Parents with linked ordinances (e.g. MiLoG) stay `uncertain` until each operative citation is proven against a current linked child that is itself `confirmed_current`. Bare editorial `Bek.` cites without a section hint are ignored; section-scoped `Bek.` and Kind `V` still require proof. That is fail-closed behaviour, not a failed lookup. Sync can still be healthy while a law is `uncertain`.
2. **Freshness is a contract, not decoration.** Do not index or serve text as “current” unless freshness (or for bundles, `bundle_freshness.safe_to_serve`) allows it.
3. **Linked Verordnungen are not the same as parent §§.** MiLoV5 § 2 is not MiLoG § 2. Use `section_hint` / `parent_section_hint` as attachment metadata, not as a 1:1 section map.
4. **CLI and `serve` share one database file.** Do not run both at once against the same `gesetzeswache.db` (lock / timeout errors).
5. **First sync needs network and time.** Until `/readyz` reports ready, resolve/export may be incomplete. Outbound HTTPS to GII and `recht.bund.de` is required.
6. **Auto-discovery of linked instruments is best-effort.** High-confidence Ermächtigung links are stored automatically; seeded TSV overrides win on collision. Coverage is incomplete for the full federal corpus.
7. **HTTP is open by design for trusted networks.** Only `POST /v1/recheck` is authenticated. Do not expose the API on the public internet without TLS, network controls, and rate limits — see [`SECURITY.md`](SECURITY.md).

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- USAGE -->
## Usage

All JSON responses use the same envelope:

```json
{ "success": true, "data": { }, "error": null }
```

### Freshness (consumer contract)

| State | Meaning | What you should do |
|-------|---------|-------------------|
| `confirmed_current` | Verified against BGBl/GII signals | Safe to treat as current (also check confidence + Stand `parse_ok`) |
| `confirmed_stale` | A newer publication is known | Quarantine / manual review |
| `uncertain` | Evidence missing or conflicting | Quarantine / manual review — **do not** treat as proven current |

Rules of thumb:

1. Always read freshness on matched results (`data.freshness.state`, or for bundles/index: `data.bundle_freshness`).
2. For bundles/index, serve as current only when `bundle_freshness.safe_to_serve` is `true` (fail-closed over **all** members).
3. For vector ingest of amendment-by-reference parents, index **parent and linked ordinances as separate chunks** linked by `parent_section_hint` — do not embed optional `--compose` hierarchical text.
4. BGBl issue identity is the triple `(teil, year, number)` — never compare issue numbers alone.

Optional: `GEW_REFUSE_EXPORT_STALE=true` makes the server refuse export when a law is `confirmed_stale`.

### Which command / endpoint for which job?

| Goal | CLI | REST |
|------|-----|------|
| Find a law + freshness | `gew resolve MiLoG` | `GET /v1/resolve?q=MiLoG` |
| Freshness only | `gew freshness milog` | `GET /v1/freshness?id=milog` |
| Full readable text | `gew export MiLoG hierarchical` | `GET /v1/export?q=MiLoG&format=hierarchical` |
| Parent + linked VO (unmixed, for display/API) | `gew bundle MiLoG normtext` | `GET /v1/bundle?q=MiLoG&format=normtext` |
| Combined markdown for humans | `gew bundle --compose MiLoG hierarchical` | `…/v1/bundle?…&compose=true` |
| Flat chunks for a vector DB | `gew index MiLoG` | `GET /v1/index?q=MiLoG` |
| Only one parent section + matching VO | `gew index MiLoG --section='§ 1'` | `…/v1/index?q=MiLoG&section=%C2%A7%201` |

### Export formats

| Format | Shape | Best for |
|--------|-------|----------|
| `hierarchical` | Markdown-like sections | Reading / display |
| `chunked` / `normtext` | One payload per Abs./unit | RAG / embeddings (TOC and editorial `(+++ … +++ )` omitted) |
| `flat` | Full IR with markers | Diffing / debugging (includes chrome that vector formats drop) |

**`index`** is a dedicated ingest projection: flat `chunks[]` with `law_id`, `law_name`, `instrument_kind`, `section_ref`, `section_name`, and on linked ordinances only `parent_law_id` / `parent_section_hint`. Formulary chrome such as `Eingangsformel` is omitted from index output.

### REST API

Default listen address: `:8080` (Compose maps host **8081** → container 8080 — see [Docker](#docker)).

| Method | Path | Notes |
|--------|------|-------|
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness (catalog / sync initialized) |
| GET | `/metrics` | Prometheus text (unauthenticated) |
| GET, POST | `/v1/resolve?q=` | Resolve + freshness (`include=past`, `include=linked`) |
| GET | `/v1/freshness?id=` | Freshness by id or `q=` |
| GET | `/v1/stale` | List `confirmed_stale` laws |
| GET | `/v1/sync/status` | Sync and readiness timestamps |
| GET, POST | `/v1/export?q=&format=` | `hierarchical` \| `chunked` \| `flat` \| `normtext` |
| GET, POST | `/v1/bundle?q=&format=` | Parent + current linked Verordnungen (`include=past`, `compose=true`) |
| GET, POST | `/v1/index?q=` | Flat ingest chunks (`section=§1,§2`, `include=past`) |
| POST | `/v1/recheck` | Force re-verification (**auth required**) |

```bash
# Replace 8080 with 8081 if using the default docker-compose.yml mapping
curl -s 'http://127.0.0.1:8080/healthz'
curl -s 'http://127.0.0.1:8080/readyz'
curl -s 'http://127.0.0.1:8080/v1/resolve?q=BGB' | jq .
curl -s 'http://127.0.0.1:8080/v1/resolve?q=MiLoG&include=linked' | jq .
curl -s 'http://127.0.0.1:8080/v1/export?q=MiLoG&format=hierarchical' | jq -r '.data.formats.hierarchical' | head
curl -s 'http://127.0.0.1:8080/v1/bundle?q=MiLoG&format=hierarchical&compose=true' | jq -r '.data.formats.hierarchical' | head
curl -s 'http://127.0.0.1:8080/v1/index?q=MiLoG' | jq '.data.chunks[0]'
curl -s 'http://127.0.0.1:8080/v1/sync/status' | jq .
```

### CLI

From the repo root after build (`./bin/gew`) or with `bin` on your `PATH`:

```bash
gew serve                         # HTTP API + background sync
gew resolve BGB                   # Resolve law + freshness
gew resolve --include=linked MiLoG
gew freshness bgb
gew stale
gew recheck bgb                   # Local process call (no HTTP token)
gew sync-status
gew export BGB hierarchical
gew bundle MiLoG normtext
gew bundle --compose MiLoG hierarchical
gew index MiLoG
gew index MiLoG --section='§ 1'
gew index --include=past MiLoG --section='§ 1,§ 2'
gew mcp                           # MCP stdio server
gew health                        # Probe /healthz (container HEALTHCHECK)
gew version
```

`gew help` prints the same command list with examples.

### MCP (stdio)

Run `gew mcp` and connect your MCP client (stdio JSON-RPC).

| Tool | Purpose |
|------|---------|
| `resolve_law` | Resolve by abbreviation/title + freshness |
| `export_law_text` | Export text (`hierarchical`, `chunked`, `flat`, `normtext`) |
| `export_law_bundle` | Parent + current linked Verordnungen (optional `compose`) |
| `law_freshness` | Freshness for a known id or abbreviation |
| `list_stale_laws` | List `confirmed_stale` laws |
| `force_recheck` | Force re-verification (in-process; no HTTP token) |
| `sync_status` | Sync and readiness |

There is **no** MCP tool for `/v1/index` yet — use CLI or REST for vector ingest.

### Authentication (recheck only)

`POST /v1/recheck` is fail-closed:

- Set `GEW_SHARED_SECRET` to a long random value
- Send the same value in the `X-Gesetzeswache-Token` header
- Empty secret or wrong header → `401`

CLI `recheck` and MCP `force_recheck` talk to the local process directly (no HTTP token). Protect process access in production.

### Linked instruments and variants

Some statutes (e.g. MiLoG) do not carry every operative rate in the Gesetz text itself; a linked Verordnung does. Those links appear under `freshness.linked_instruments` when you use `include=linked`, or as separate members via `bundle` / `index`.

| File | Role |
|------|------|
| [`variants/variants.tsv`](variants/variants.tsv) | Informal names → law id (e.g. `Zivilgesetzbuch` → `bgb`) |
| [`variants/linked_instruments.tsv`](variants/linked_instruments.tsv) | Manual parent → Verordnung overrides (wins on collision) |
| [`variants/fortschreibung_families.tsv`](variants/fortschreibung_families.tsv) | Yearly Fortschreibung families (latest catalog year) |

When discovery is enabled (`GEW_DISCOVERY_ENABLED=true`), high-confidence parent→child links can also be stored from Verordnung Ermächtigung text (`source=discovered`). Treat discovery as helpful coverage, not a complete legal graph.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- GETTING STARTED -->
## Getting Started

### Prerequisites

- **Go 1.26+** to build from source
- **Docker** (optional) for the container image / Compose
- **Network** — outbound HTTPS to GII and `recht.bund.de` for sync and export
- **`jq`** (optional) — handy for inspecting JSON from curl/CLI

### Installation

1. Clone the repository
   ```sh
   git clone https://github.com/Squarenix17/gesetzeswache.git
   cd gesetzeswache
   ```

2. Build the binary
   ```sh
   go build -o bin/gew ./cmd/gesetzeswache
   ```
   Or download a release asset into `bin/` from [Releases](https://github.com/Squarenix17/gesetzeswache/releases) (e.g. `gew_linux_amd64` → `bin/gew`), then `chmod +x bin/gew`.

3. Put `bin` on your `PATH` (optional)
   ```sh
   export PATH="$(pwd)/bin:$PATH"
   ```

4. Start the server **or** use CLI commands (not both against the same DB at once)
   ```sh
   ./bin/gew serve
   ```

5. Wait for readiness, then probe
   ```sh
   curl http://127.0.0.1:8080/healthz
   curl http://127.0.0.1:8080/readyz
   ```

The first sync can take one to several minutes depending on network and discovery settings.

### Configuration

All settings use the `GEW_` prefix. Copy [`.env.example`](.env.example) as a starting point.

| Variable | Default | Description |
|----------|---------|-------------|
| `GEW_HTTP_ADDR` | `:8080` | HTTP listen address |
| `GEW_STORE_PATH` | `gesetzeswache.db` | Embedded bbolt catalog and sync state |
| `GEW_MATCH_THRESHOLD` | `0.75` | Fuzzy match threshold (0–1) |
| `GEW_FRESHNESS_MAX_AGE` | `6h` | Max age of sync evidence before confirmation fails |
| `GEW_SHARED_SECRET` | *(empty)* | Required for HTTP recheck; empty → fail-closed `401` |
| `GEW_VARIANTS_PATH` | `variants/variants.tsv` | Informal name → law id |
| `GEW_LINKED_INSTRUMENTS_PATH` | `variants/linked_instruments.tsv` | Seeded parent→VO links |
| `GEW_ENABLE_EXPORT` | `true` | Enable on-demand text export / bundle / index |
| `GEW_REFUSE_EXPORT_STALE` | `false` | Refuse export when law is `confirmed_stale` |
| `GEW_DISCOVERY_ENABLED` | `true` | Auto-discover parent→Verordnung links |
| `GEW_DISCOVERY_MAX_PER_CYCLE` | `50` | Max Verordnungen ingested per discovery pass |
| `GEW_FORTSCHREIBUNG_FAMILIES_PATH` | `variants/fortschreibung_families.tsv` | Fortschreibung slug-prefix families |

More sync intervals and source URLs: [`.env.example`](.env.example), [`internal/config/config.go`](internal/config/config.go).

### Docker

Published image: `ghcr.io/squarenix17/gesetzeswache:latest` (pinned tags on [releases](https://github.com/Squarenix17/gesetzeswache/releases)).

The image includes a `HEALTHCHECK` via `gew health`. Kubernetes can keep using `/healthz` and `/readyz`.

**Compose** (from the repo; `GEW_SHARED_SECRET` is required):

```bash
export GEW_SHARED_SECRET="$(openssl rand -hex 32)"
# Persist the secret so restarts keep the same value:
#   echo "GEW_SHARED_SECRET=$GEW_SHARED_SECRET" > .env

docker compose up -d --build
curl http://127.0.0.1:8081/readyz
```

The checked-in [`docker-compose.yml`](docker-compose.yml) maps **host `8081` → container `8080`** so it does not collide with another service already on 8080. Adjust the left-hand port if needed.

```bash
docker compose logs -f gew
docker compose down
```

**One-off run:**

```bash
docker run --rm -p 8081:8080 \
  -e GEW_SHARED_SECRET="$(openssl rand -hex 32)" \
  -v gew-data:/tmp \
  ghcr.io/squarenix17/gesetzeswache:latest
```

For vulnerability reporting and deployment hardening, see [`SECURITY.md`](SECURITY.md).

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- LICENSE -->
## License

Distributed under the MIT License. See [`LICENSE`](LICENSE) for more information.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTACT -->
## Contact

Gabriele Ughetto · Telegram [@Squarenix](https://t.me/squarenix)

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
[go-shield]: https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white
[go-url]: https://go.dev/
