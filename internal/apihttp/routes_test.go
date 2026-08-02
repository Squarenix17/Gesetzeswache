package apihttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/service"
	"github.com/Squarenix17/gesetzeswache/internal/test/fixtures"
)

func seedCatalogForHTTP(t *testing.T, srv *Server) {
	t.Helper()
	law := domain.Law{ID: "arbzg", Abbreviation: "ArbZG", Title: "Arbeitszeitgesetz", GIIPath: "arbzg"}
	if err := srv.Svc.Store.UpsertLaws([]domain.Law{law}); err != nil {
		t.Fatal(err)
	}
	srv.Svc.Search.Swap([]domain.Law{law}, nil)
	now := time.Now().UTC()
	_ = srv.Svc.Store.SetMetaTime("last_toc_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_gii_feed_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_bgbl_feed_success", now)
}

func seedManyLinkedParentHTTP(t *testing.T, srv *Server) {
	t.Helper()
	tsvPath := filepath.Join(t.TempDir(), "links.tsv")
	var b strings.Builder
	b.WriteString("# parent\tkind\tslug\tnotes\teffective_from\tsection_hint\n")
	for i := 0; i < service.MaxOperativeBundleMembers+1; i++ {
		slug := fmt.Sprintf("capv_%d", i)
		hint := fmt.Sprintf("§ %d", i+1)
		fmt.Fprintf(&b, "p\tverordnung\t%s\tBGBl 2024 I Nr. %d\t2024-01-01\t%s\n", slug, i+1, hint)
	}
	if err := os.WriteFile(tsvPath, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := instruments.LoadTSV(tsvPath)
	if err != nil {
		t.Fatal(err)
	}
	srv.Svc.Instruments = cat
	parent := domain.Law{ID: "p", Abbreviation: "P", Title: "Parent", GIIPath: "p", GIIURL: "https://www.gesetze-im-internet.de/p/"}
	if err := srv.Svc.Store.UpsertLaws([]domain.Law{parent}); err != nil {
		t.Fatal(err)
	}
	srv.Svc.Search.Swap([]domain.Law{parent}, nil)
	now := time.Now().UTC()
	_ = srv.Svc.Store.SetMetaTime("last_toc_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_gii_feed_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_bgbl_feed_success", now)
}

func TestFreshness_endpoint(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/v1/freshness?id=arbzg", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if !env.Success {
		t.Fatalf("success=false err=%v", env.Error)
	}
}

func TestFreshness_lawNotFound(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/v1/freshness?id=missing-law", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestReadyz_catalogReady(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestReadyz_notReady(t *testing.T) {
	srv, _ := testServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestSyncStatus_endpoint(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/v1/sync/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestResolve_postJSONBody(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	body := `{"query":"ArbZG"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestResolve_invalidJSON(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestExport_disabled(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	srv.Svc.CFG.EnableExport = false

	req := httptest.NewRequest(http.MethodGet, "/v1/export?q=ArbZG", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExport_unknownFormat(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/v1/export?q=ArbZG&format=invalid", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestRecheck_methodNotAllowed(t *testing.T) {
	srv, _ := testServer(t, "sekret")
	req := httptest.NewRequest(http.MethodGet, "/v1/recheck", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestExport_HTTP_success(t *testing.T) {
	srv, mt := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	stand := domain.StandCitation{LawID: "arbzg", Raw: "BGBl", ParseOK: true, Year: 2022, Teil: 1, Number: "1"}
	_ = srv.Svc.Store.UpsertStand(stand)
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	req := httptest.NewRequest(http.MethodGet, "/v1/export?q=ArbZG&format=hierarchical", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExport_postJSON(t *testing.T) {
	srv, mt := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	body := `{"query":"ArbZG","formats":["hierarchical"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestBundle_HTTP_parentOnly(t *testing.T) {
	srv, mt := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=ArbZG&format=hierarchical", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if !env.Success {
		t.Fatalf("envelope %+v", env)
	}
}

func TestBundle_HTTP_composeAndPost(t *testing.T) {
	srv, mt := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=ArbZG&format=hierarchical&compose=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	body := `{"query":"ArbZG","formats":["hierarchical"],"compose":true}`
	req = httptest.NewRequest(http.MethodPost, "/v1/bundle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestBundle_HTTP_unknownFormat(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=ArbZG&format=invalid", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestBundle_HTTP_exportDisabled(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	srv.Svc.CFG.EnableExport = false
	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=ArbZG", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestBundle_HTTP_refuseStale(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	srv.Svc.CFG.RefuseExportStale = true
	now := time.Now().UTC()
	_ = srv.Svc.Store.SetMetaTime("last_toc_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_gii_feed_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_bgbl_feed_success", now)
	stand := domain.StandCitation{LawID: "arbzg", Raw: "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198", ParseOK: true, Year: 2023, Teil: 1, Number: "198"}
	_ = srv.Svc.Store.UpsertStand(stand)
	issueID := "BGBl-1/2026/999"
	_ = srv.Svc.Store.UpsertIssue(domain.GazetteIssue{ID: issueID, Teil: 1, Year: 2026, Number: "999"})
	_ = srv.Svc.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "arbzg", Class: domain.LinkConfirmed})

	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=ArbZG&format=hierarchical", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.ErrorCode == nil || *env.ErrorCode != "export_refused_confirmed_stale" {
		t.Fatalf("error_code=%v want export_refused_confirmed_stale", env.ErrorCode)
	}
}

func TestBundle_HTTP_allowStaleAndProfileIngest(t *testing.T) {
	srv, mt := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	srv.Svc.CFG.RefuseExportStale = true
	now := time.Now().UTC()
	_ = srv.Svc.Store.SetMetaTime("last_toc_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_gii_feed_success", now)
	_ = srv.Svc.Store.SetMetaTime("last_bgbl_feed_success", now)
	stand := domain.StandCitation{LawID: "arbzg", Raw: "Zuletzt geändert durch Art. 1 G v. 16.8.2023 BGBl. 2023 I Nr. 198", ParseOK: true, Year: 2023, Teil: 1, Number: "198"}
	_ = srv.Svc.Store.UpsertStand(stand)
	issueID := "BGBl-1/2026/999"
	_ = srv.Svc.Store.UpsertIssue(domain.GazetteIssue{ID: issueID, Teil: 1, Year: 2026, Number: "999"})
	_ = srv.Svc.Store.UpsertLink(domain.IssueLawLink{IssueID: issueID, LawID: "arbzg", Class: domain.LinkConfirmed})
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", fixtures.MustRead("arbzg_snippet.xml")))

	for _, q := range []string{
		"/v1/bundle?q=ArbZG&format=hierarchical&allow_stale=1",
		"/v1/bundle?q=ArbZG&format=hierarchical&profile=ingest",
		"/v1/index?q=ArbZG&profile=INGEST",
	} {
		req := httptest.NewRequest(http.MethodGet, q, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status %d body %s", q, rec.Code, rec.Body.String())
		}
	}
}

func TestBundle_HTTP_parentOnlySkipsCap(t *testing.T) {
	srv, mt := testServer(t, "")
	seedManyLinkedParentHTTP(t, srv)
	mt.SetBytes("www.gesetze-im-internet.de", "/p/xml.zip", fixtures.MustZipXML("p.xml", fixtures.MustRead("arbzg_snippet.xml")))

	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=p&format=hierarchical&parent_only=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestBundle_HTTP_oversizedStructuredBody(t *testing.T) {
	srv, _ := testServer(t, "")
	seedManyLinkedParentHTTP(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/v1/bundle?q=p&format=hierarchical", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || !strings.Contains(*env.Error, "operative bundle too large") {
		t.Fatalf("error=%v", env.Error)
	}
	if env.ErrorCode == nil || *env.ErrorCode != "operative_bundle_too_large" {
		t.Fatalf("error_code=%v want operative_bundle_too_large", env.ErrorCode)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		raw, _ := json.Marshal(env.Data)
		_ = json.Unmarshal(raw, &data)
	}
	if data["max"] == nil || data["actual"] == nil || data["members"] == nil {
		t.Fatalf("data=%+v want max/actual/members", data)
	}
}

func TestBundle_HTTP_invalidJSON(t *testing.T) {
	srv, _ := testServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/bundle", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestIndex_HTTP_getAndPost(t *testing.T) {
	srv, mt := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	xmlBody := fixtures.MustRead("arbzg_snippet.xml")
	mt.SetBytes("www.gesetze-im-internet.de", "/arbzg/xml.zip", fixtures.MustZipXML("arbzg.xml", xmlBody))

	req := httptest.NewRequest(http.MethodGet, "/v1/index?q=ArbZG&section=%C2%A7%201", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var env service.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if !env.Success {
		t.Fatalf("envelope %+v", env)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		// Data may be decoded as map from json.Raw - re-marshal path
		raw, _ := json.Marshal(env.Data)
		_ = json.Unmarshal(raw, &data)
	}
	body := `{"query":"ArbZG","section":"§ 1"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/index", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestIndex_HTTP_exportDisabled(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	srv.Svc.CFG.EnableExport = false
	req := httptest.NewRequest(http.MethodGet, "/v1/index?q=ArbZG", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestIndex_HTTP_invalidJSON(t *testing.T) {
	srv, _ := testServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/index", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestFreshness_queryParamQ(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/v1/freshness?q=arbzg", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestResolve_catalogNotReady(t *testing.T) {
	srv, _ := testServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/resolve?q=ArbZG", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestStale_endpoint(t *testing.T) {
	srv, _ := testServer(t, "")
	seedCatalogForHTTP(t, srv)
	now := time.Now().UTC()
	_ = srv.Svc.Store.PutFreshness(domain.FreshnessRecord{LawID: "arbzg", State: domain.FreshnessConfirmedStale, EvaluatedAt: now})

	req := httptest.NewRequest(http.MethodGet, "/v1/stale", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
