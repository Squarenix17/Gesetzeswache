# Gesetzeswache Test Report

## 1. Environment
- **Go version:** `go1.24.3 linux/amd64`
- **Commit SHA:** `N/A` (Using cloned source from issue)
- **Binary/Docker:** Tested with locally built binary due to container networking issues in sandbox.
- **BASE URL:** `http://127.0.0.1:18080`

## 2. Automated Tests
All automated tests pass cleanly except for lack of network reachability on test sandbox causing some timeouts if external network is strictly required.

- `go test ./... -count=1`: Passed.
- `go test ./... -race -count=1`: Passed.
- `go vet ./...`: Clean.
- `go test ./internal/service/... -count=1 -v`: Passed.
- `go test ./internal/discovery/... -count=1 -v`: Passed.
- `go test ./internal/sync/... -count=1 -v`: Passed.

## 3. Live API Matrix

| Endpoint | Status | Notes |
|----------|--------|-------|
| `GET /healthz` | `data.status == "up"` | Works as expected. |
| `GET /readyz` | `success == false` | Fails because `catalog_ready: false` due to network unreachability of government feeds. |
| `GET /metrics` | Returns text | Metrics are exposed, though mostly `0` since sync failed. |
| `GET /v1/sync/status` | `catalog_ready: false` | Initial sync failed fetching `gii-toc.xml` and bgbl. |
| `GET /v1/resolve?q=BGB` | Error 500 / catalog not ready | Cannot resolve without a synced catalog. |
| `GET /v1/resolve?q=Zivilgesetzbuch` | Error 500 / catalog not ready | Same as above. |
| `GET /v1/freshness?id=bgb` | Error 500 / law not found | Catalog is empty. |
| `GET /v1/stale` | success: true, empty data | No laws ingested, so none are stale. |
| `GET /v1/export?q=BGB&format=hierarchical` | Error 500 / catalog not ready | Export relies on synced data. |
| `GET /v1/export?q=BGB&format=normtext` | Error 500 / catalog not ready | Same as above. |
| `POST /v1/resolve` | Error 500 / query required | Note: Needs `"query"` payload, not `"q"`, but fails similarly on catalog. |

## 4. Golden Cases
Due to network timeouts (e.g. `TLS handshake timeout` fetching `rss_bgbl-1.xml`, timeout fetching `gii-toc.xml`) the data population for `Gesetzeswache` failed during the `InitialSync`. Therefore, `bgb`, `arbzg`, `milog`, `pbav_2025` queries all result in `catalog not ready` or `law not found` errors.

Evidence:
```json
{
  "success": false,
  "data": {
    "matched": false,
    "threshold": 0.75
  },
  "error": "catalog not ready"
}
```

## 5. Metrics Snapshot
```
# HELP gew_bgbl_index_entries Number of BGBl citation index entries
# TYPE gew_bgbl_index_entries gauge
gew_bgbl_index_entries 0
# HELP gew_discovered_links Number of persisted discovered parent→child links
# TYPE gew_discovered_links gauge
gew_discovered_links 0
# HELP gew_discovery_ingest_total Discovery ingest operations by result
# TYPE gew_discovery_ingest_total counter
```

## 6. Security
Tested `/v1/recheck` authentication:
- No token → 401 Unauthorized
- Wrong token → 401 Unauthorized
- Correct token (`test-secret`) → 200 OK (returns `{"success":true,...}`)

## 7. Issues Found
- **SEVERITY:** HIGH
  - **Issue:** The service will hang in a `catalog not ready` state indefinitely if the initial network requests to `www.gesetze-im-internet.de` or `www.recht.bund.de` fail (e.g., due to strict firewall rules, transient network outages, or TLS handshake issues as seen in the logs).
  - **Repro:** Start the server in an offline/firewalled environment.
  - **Suggested Fix:** Include a static fallback TOC/DB cache or improve error handling to gracefully expose partial data if available.

## 8. Out of Scope
Phase 4.9 items confirmed still open.
