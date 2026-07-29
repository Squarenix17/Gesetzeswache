# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| Latest release on GitHub | Yes |
| `main` branch | Yes |
| Older releases | Best effort |

## Reporting a vulnerability

Please use **GitHub Private Vulnerability Reporting** (Security → Report a vulnerability) on this repository when available.

For non-sensitive issues, use GitHub Issues. Do **not** open a public issue for sensitive vulnerabilities until a fix or advisory is ready.

## Out of scope

- Legal interpretation or correctness of German statutory text from official sources
- Abuse of a deployment that was exposed to the internet without authentication, rate limiting, or a reverse proxy (resolve/export/metrics are open by design for local/trusted use)
- Social-engineering or physical attacks

## Deployment note

`gew serve` defaults to listening on `:8080` (all interfaces). Only `POST /v1/recheck` requires `GEW_SHARED_SECRET`. Do not expose the HTTP API on the public internet without TLS, network controls, and rate limits. Prefer binding to `127.0.0.1` or placing a reverse proxy in front.

## Threat model and controls

### Outbound SSRF

Outbound HTTPS to official sources uses a shared HTTP client with:

- **Host allowlist** — only configured government hosts (`gesetze-im-internet.de`, `recht.bund.de`, etc.) are permitted
- **Literal-IP rejection** — URLs with numeric hostnames are blocked at validation
- **Dial-time IP pinning** — after DNS resolution, connections to loopback, private, link-local, unspecified, CGNAT (`100.64.0.0/10`), and NAT64 (`64:ff9b::/96` with embedded-IPv4 re-check) addresses are refused

**PROXY caveat (deferred review finding M3):** if `HTTP_PROXY` or `HTTPS_PROXY` is set, outbound connections go through the proxy and dial-time IP pinning applies only to the proxy connection. Deployments behind a proxy **must** enforce egress policy at the proxy. Recommend unsetting proxy environment variables for this service otherwise.

### Denial-of-service bounds

- **HTTP server timeouts:** read header 10s, read 30s, idle 120s, write 270s (write timeout covers the 240s recheck bound with margin)
- **Query/id cap:** 512 runes → HTTP 400
- **MCP stdio:** 4 MiB inbound message cap; 1 MiB response/body drain cap (pre-existing)
- **Bounded request IDs:** client `X-Request-ID` echoed only when ≤128 alphanumeric/`._-` chars; otherwise a server-generated ID is used
- **XML entity guard:** shared `xmlsafe` rejects internal entity declarations in untrusted upstream XML

### Authentication

`POST /v1/recheck` compares `X-Gesetzeswache-Token` to `GEW_SHARED_SECRET` via **constant-time** compare over fixed-width SHA-256 digests. Empty secret is **fail-closed** (401). Wrong or missing token → 401 with stable contract message.

### Error handling

Unexpected failures return HTTP 5xx with generic **"internal error"**; structured details are logged server-side with `request_id`. Intentional 4xx messages are a stable **allowlist** (query caps, not found, unauthorized, recheck timeout, etc.). MCP tool errors use the same sanitization via `clienterr.Sanitize`.

### Supply chain

- **Go 1.24** toolchain-pinned (`go1.24.5` in `go.mod`)
- **Digest-pinned base images** in `Dockerfile` (as of 2026-07-29)
- **CI gates (blocking):** `govulncheck`, `gosec` v2.22.0, `staticcheck`, race detector, per-package coverage floors (`service`/`apihttp`/`sync`/`store` ≥80%)
- **Release workflow:** verify → cross-platform binaries with `-ldflags -X main.version=…` → SPDX SBOM → GitHub release with `SHA256SUMS` artifacts

### Operational semantics (freshness)

`DataFresh` in `/v1/sync/status` and the `gew_data_fresh` gauge is an **ops gauge**: true when any usable sync evidence (including probe-only) is within max age. It is deliberately distinct from per-law freshness verdicts (`confirmed_current` / `uncertain`). See `docs/adr/0001-fail-closed-freshness.md`.

**Roadmap:** per-law sync evidence timestamps on `/v1/sync/status` are not yet exposed.

## Thanks

We appreciate responsible disclosure and will credit reporters who wish to be named.
