package apihttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/domain"
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
