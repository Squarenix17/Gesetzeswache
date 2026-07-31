package apihttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/citation"
	"github.com/Squarenix17/gesetzeswache/internal/config"
	"github.com/Squarenix17/gesetzeswache/internal/domain"
	"github.com/Squarenix17/gesetzeswache/internal/instruments"
	"github.com/Squarenix17/gesetzeswache/internal/search"
	"github.com/Squarenix17/gesetzeswache/internal/service"
	"github.com/Squarenix17/gesetzeswache/internal/store"
)

func TestResolve_includePastQuery(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	law := domain.Law{
		ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
		GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
	}
	_ = st.UpsertLaws([]domain.Law{law})
	now := time.Now().UTC()
	_ = st.SetMetaTime("last_toc_success", now)
	_ = st.SetMetaTime("last_gii_feed_success", now)
	_ = st.SetMetaTime("last_bgbl_feed_success", now)
	_ = st.UpsertStand(citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137"))
	_ = st.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
	})

	eng := search.NewEngine()
	laws, _ := st.ListLaws()
	variants, _ := st.ListVariants()
	eng.Swap(laws, variants)

	svc := &service.Service{
		CFG:         config.Config{MatchThreshold: 0.75, FreshnessMaxAge: 6 * time.Hour, GIIBase: "https://www.gesetze-im-internet.de"},
		Store:       st,
		Search:      eng,
		Instruments: cat,
	}
	srv := &Server{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/v1/resolve?q=MiLoG", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env service.Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(env.Data)
	var res service.ResolveResult
	_ = json.Unmarshal(data, &res)
	if res.Freshness == nil || len(res.Freshness.LinkedInstruments) != 1 {
		t.Fatalf("default linked=%+v", res.Freshness)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/resolve?q=MiLoG&include=past", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	_ = json.Unmarshal(rr2.Body.Bytes(), &env)
	data, _ = json.Marshal(env.Data)
	_ = json.Unmarshal(data, &res)
	if res.Freshness == nil || len(res.Freshness.LinkedInstruments) != 2 {
		t.Fatalf("include=past linked=%+v", res.Freshness.LinkedInstruments)
	}
}

func TestFreshness_includeProofQuery(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat, err := instruments.LoadTSV(filepath.Join("..", "..", "variants", "linked_instruments.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	law := domain.Law{
		ID: "milog", Abbreviation: "MiLoG", Title: "Mindestlohngesetz",
		GIIPath: "milog", GIIURL: "https://www.gesetze-im-internet.de/milog/",
	}
	_ = st.UpsertLaws([]domain.Law{law})
	now := time.Now().UTC()
	_ = st.SetMetaTime("last_toc_success", now)
	_ = st.SetMetaTime("last_gii_feed_success", now)
	_ = st.SetMetaTime("last_bgbl_feed_success", now)
	_ = st.UpsertStand(citation.Parse("milog", "Zuletzt geändert durch Art. 8 Abs. 3 G v. 12.5.2026 I Nr. 137"))
	_ = st.UpsertIssue(domain.GazetteIssue{
		ID: citation.IssueID(1, 2025, "268"), Teil: 1, Year: 2025, Number: "268",
	})

	eng := search.NewEngine()
	laws, _ := st.ListLaws()
	variants, _ := st.ListVariants()
	eng.Swap(laws, variants)

	svc := &service.Service{
		CFG:         config.Config{MatchThreshold: 0.75, FreshnessMaxAge: 6 * time.Hour, GIIBase: "https://www.gesetze-im-internet.de"},
		Store:       st,
		Search:      eng,
		Instruments: cat,
	}
	srv := &Server{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/v1/freshness?id=milog", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var env service.Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(env.Data)
	var meta service.FreshnessMeta
	_ = json.Unmarshal(data, &meta)
	if len(meta.Proof) != 0 {
		t.Fatalf("default must omit proof; got %+v", meta.Proof)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/freshness?id=milog&include=proof", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(env.Data)
	_ = json.Unmarshal(data, &meta)
	if len(meta.Proof) == 0 {
		t.Fatalf("include=proof must attach proof; body=%s", rr2.Body.String())
	}
}
