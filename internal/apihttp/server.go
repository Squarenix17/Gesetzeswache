package apihttp

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/service"
)

// Server is the REST adapter.
type Server struct {
	Svc          *service.Service
	SharedSecret string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/v1/resolve", s.resolve)
	mux.HandleFunc("/v1/freshness", s.freshness)
	mux.HandleFunc("/v1/stale", s.stale)
	mux.HandleFunc("/v1/recheck", s.recheck)
	mux.HandleFunc("/v1/sync/status", s.syncStatus)
	mux.HandleFunc("/v1/export", s.export)
	return mux
}

func (s *Server) write(w http.ResponseWriter, status int, data any, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	var eptr *string
	if errMsg != "" {
		eptr = &errMsg
	}
	_ = json.NewEncoder(w).Encode(service.Envelope{
		Success: errMsg == "" && status < 400,
		Data:    data,
		Error:   eptr,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	s.write(w, 200, map[string]any{"status": "up", "time": time.Now().UTC()}, "")
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	st, err := s.Svc.SyncStatus(r.Context())
	if err != nil {
		s.write(w, 503, nil, err.Error())
		return
	}
	code := 200
	if !st.CatalogReady {
		code = 503
	}
	s.write(w, code, st, "")
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" && r.Body != nil && r.Body != http.NoBody {
		var body struct {
			Query string `json:"query"`
		}
		if err := decodeJSON(w, r, &body); err != nil && err != io.EOF {
			s.write(w, 400, nil, "invalid json body")
			return
		}
		q = body.Query
	}
	res, err := s.Svc.Resolve(r.Context(), q)
	if err != nil {
		s.write(w, 503, res, err.Error())
		return
	}
	s.write(w, 200, res, "")
}

func (s *Server) freshness(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = r.URL.Query().Get("q")
	}
	meta, err := s.Svc.Freshness(r.Context(), id)
	if err != nil {
		s.write(w, 404, nil, err.Error())
		return
	}
	s.write(w, 200, meta, "")
}

func (s *Server) stale(w http.ResponseWriter, r *http.Request) {
	list, err := s.Svc.ListStale(r.Context())
	if err != nil {
		s.write(w, 500, nil, err.Error())
		return
	}
	s.write(w, 200, list, "")
}

func (s *Server) recheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		s.write(w, 405, nil, "POST required")
		return
	}
	if !s.authorize(r) {
		s.write(w, 401, nil, "unauthorized: set GEW_SHARED_SECRET and X-Gesetzeswache-Token")
		return
	}
	id := r.URL.Query().Get("id")
	if err := s.Svc.ForceRecheck(r.Context(), id); err != nil {
		s.write(w, 500, nil, err.Error())
		return
	}
	s.write(w, 200, map[string]string{"status": "recheck_completed"}, "")
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.Svc.SyncStatus(r.Context())
	if err != nil {
		s.write(w, 500, nil, err.Error())
		return
	}
	s.write(w, 200, st, "")
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	formats := r.URL.Query()["format"]
	if len(formats) == 1 && strings.Contains(formats[0], ",") {
		formats = strings.Split(formats[0], ",")
	}
	if q == "" && r.Body != nil && r.Body != http.NoBody {
		var body struct {
			Query   string   `json:"query"`
			Formats []string `json:"formats"`
		}
		if err := decodeJSON(w, r, &body); err != nil && err != io.EOF {
			s.write(w, 400, nil, "invalid json body")
			return
		}
		if q == "" {
			q = body.Query
		}
		if len(formats) == 0 {
			formats = body.Formats
		}
	}
	var fmts []string
	if len(formats) == 0 {
		fmts = nil
	} else {
		fmts = formats
	}
	res, err := s.Svc.ExportText(r.Context(), q, fmts)
	if err != nil {
		s.write(w, 502, res, err.Error())
		return
	}
	s.write(w, 200, res, "")
}

func (s *Server) authorize(r *http.Request) bool {
	if s.SharedSecret == "" {
		return false
	}
	got := r.Header.Get("X-Gesetzeswache-Token")
	if len(got) != len(s.SharedSecret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.SharedSecret)) == 1
}
