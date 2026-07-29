# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| Latest release on GitHub | Yes |
| `main` branch | Yes |
| Older releases | Best effort |

## Reporting a vulnerability

Please use **GitHub Private Vulnerability Reporting** (Security → Report a vulnerability) on this repository when available.

If that is not an option, email the maintainer listed in the README Contact section with:

- A short description of the issue
- Steps to reproduce
- Affected version / commit
- Impact assessment (if known)

Do **not** open a public issue for sensitive vulnerabilities until a fix or advisory is ready.

## Out of scope

- Legal interpretation or correctness of German statutory text from official sources
- Abuse of a deployment that was exposed to the internet without authentication, rate limiting, or a reverse proxy (resolve/export/metrics are open by design for local/trusted use)
- Social-engineering or physical attacks

## Deployment note

`gew serve` defaults to listening on `:8080` (all interfaces). Only `POST /v1/recheck` requires `GEW_SHARED_SECRET`. Do not expose the HTTP API on the public internet without TLS, network controls, and rate limits. Prefer binding to `127.0.0.1` or placing a reverse proxy in front.

## Thanks

We appreciate responsible disclosure and will credit reporters who wish to be named.
