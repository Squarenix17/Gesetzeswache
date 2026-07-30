package apihttp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Squarenix17/gesetzeswache/internal/clienterr"
	"github.com/Squarenix17/gesetzeswache/internal/export"
	"github.com/Squarenix17/gesetzeswache/internal/metrics"
	"github.com/Squarenix17/gesetzeswache/internal/service"
)

const (
	maxBodyDrain               = 1 << 20
	recheckWriteDeadlineMargin = 30 * time.Second
)

type ctxKey int

const requestIDKey ctxKey = iota

// Server is the REST adapter.
type Server struct {
	Svc          *service.Service
	SharedSecret string
	Metrics      *metrics.Registry
	Log          *slog.Logger
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
	mux.HandleFunc("/v1/bundle", s.bundle)
	mux.HandleFunc("/v1/index", s.index)
	if s.Metrics == nil {
		s.Metrics = metrics.NewRegistry()
		metrics.RegisterDefaults(s.Metrics)
	}
	mux.Handle("/metrics", s.Metrics.Handler(s.collectMetrics))
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(reqID) {
			reqID = newRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func (s *Server) collectMetrics(reg *metrics.Registry) {
	if s != nil && s.Svc != nil {
		s.Svc.CollectMetrics(reg)
	}
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

func (s *Server) logError(ctx context.Context, msg string, err error) {
	if err == nil {
		return
	}
	log := s.Log
	if log == nil && s.Svc != nil {
		log = s.Svc.Log
	}
	if log == nil {
		return
	}
	args := []any{"err", err}
	if id := requestIDFrom(ctx); id != "" {
		args = append(args, "request_id", id)
	}
	log.Error(msg, args...)
}

func (s *Server) clientError(err error) string {
	return clienterr.Sanitize(err)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func drainRequestBody(r *http.Request) {
	if r.Body == nil || r.Body == http.NoBody {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxBodyDrain))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	s.write(w, 200, map[string]any{"status": "up", "time": time.Now().UTC()}, "")
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	st, err := s.Svc.SyncStatus(r.Context())
	if err != nil {
		s.logError(r.Context(), "readyz sync status", err)
		s.write(w, 503, nil, clienterr.Internal)
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
	if q != "" && r.Method == http.MethodPost {
		drainRequestBody(r)
	}
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
	res, err := s.Svc.Resolve(r.Context(), q, service.MergeInclude(r.URL.Query()["include"]))
	if err != nil {
		msg := s.clientError(err)
		switch {
		case errors.Is(err, service.ErrQueryTooLong):
			s.write(w, 400, nil, msg)
		case msg == "query required":
			s.write(w, 400, nil, msg)
		case msg == "catalog not ready":
			s.write(w, 503, res, msg)
		case msg == clienterr.Internal:
			s.logError(r.Context(), "resolve", err)
			s.write(w, 503, res, msg)
		default:
			s.logError(r.Context(), "resolve", err)
			s.write(w, 503, res, clienterr.Internal)
		}
		return
	}
	s.write(w, 200, res, "")
}

func (s *Server) freshness(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = r.URL.Query().Get("q")
	}
	meta, err := s.Svc.Freshness(r.Context(), id, service.MergeInclude(r.URL.Query()["include"]))
	if err != nil {
		msg := s.clientError(err)
		switch {
		case errors.Is(err, service.ErrQueryTooLong):
			s.write(w, 400, nil, msg)
		case msg == "law id required":
			s.write(w, 400, nil, msg)
		case msg == "law not found":
			s.write(w, 404, nil, msg)
		case msg == clienterr.Internal:
			s.logError(r.Context(), "freshness", err)
			s.write(w, 500, nil, msg)
		default:
			s.logError(r.Context(), "freshness", err)
			s.write(w, 500, nil, clienterr.Internal)
		}
		return
	}
	s.write(w, 200, meta, "")
}

func (s *Server) stale(w http.ResponseWriter, r *http.Request) {
	list, err := s.Svc.ListStale(r.Context())
	if err != nil {
		s.logError(r.Context(), "list stale", err)
		s.write(w, 500, nil, clienterr.Internal)
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
	recheckTimeout := 4 * s.Svc.CFG.HTTPTimeout
	ctx, cancel := context.WithTimeout(r.Context(), recheckTimeout)
	defer cancel()
	// Best-effort: extend write deadline for this long-running route (may fail on hijacked/test writers).
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Now().Add(recheckTimeout + recheckWriteDeadlineMargin)) //nolint:errcheck // unsupported ResponseWriter
	}
	id := r.URL.Query().Get("id")
	if err := s.Svc.ForceRecheck(ctx, id); err != nil {
		msg := s.clientError(err)
		switch {
		case errors.Is(err, service.ErrQueryTooLong):
			s.write(w, 400, nil, msg)
		case errors.Is(err, service.ErrLawNotFound):
			s.write(w, 404, nil, msg)
		case errors.Is(err, service.ErrRecheckTimeout):
			s.logError(r.Context(), "force recheck timed out", err)
			s.write(w, 504, nil, msg)
		case msg == clienterr.Internal:
			s.logError(r.Context(), "force recheck", err)
			s.write(w, 500, nil, msg)
		default:
			s.logError(r.Context(), "force recheck", err)
			s.write(w, 500, nil, clienterr.Internal)
		}
		return
	}
	s.write(w, 200, map[string]string{"status": "recheck_completed"}, "")
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.Svc.SyncStatus(r.Context())
	if err != nil {
		s.logError(r.Context(), "sync status", err)
		s.write(w, 500, nil, clienterr.Internal)
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
	res, err := s.Svc.ExportText(r.Context(), q, fmts, service.MergeInclude(r.URL.Query()["include"]))
	if err != nil {
		msg := s.clientError(err)
		switch {
		case errors.Is(err, service.ErrQueryTooLong):
			s.write(w, 400, nil, msg)
		case msg == "export disabled", strings.HasPrefix(msg, "unknown format"), msg == "empty format list":
			s.write(w, 400, nil, msg)
		case msg == "export refused: law confirmed_stale":
			s.write(w, 502, res, msg)
		case msg == clienterr.Internal:
			s.logError(r.Context(), "export", err)
			s.write(w, 502, res, msg)
		default:
			s.logError(r.Context(), "export", err)
			s.write(w, 502, res, clienterr.Internal)
		}
		return
	}
	s.write(w, 200, res, "")
}

func (s *Server) bundle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	formats := r.URL.Query()["format"]
	if len(formats) == 1 && strings.Contains(formats[0], ",") {
		formats = strings.Split(formats[0], ",")
	}
	compose := false
	for _, v := range r.URL.Query()["compose"] {
		if v == "1" || strings.EqualFold(v, "true") || v == "" {
			compose = true
		}
	}
	includeValues := append([]string{}, r.URL.Query()["include"]...)
	if q == "" && r.Body != nil && r.Body != http.NoBody {
		var body struct {
			Query   string   `json:"query"`
			Formats []string `json:"formats"`
			Compose bool     `json:"compose"`
			Include string   `json:"include"`
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
		compose = compose || body.Compose
		if body.Include != "" {
			includeValues = append(includeValues, body.Include)
		}
	}
	inc := service.MergeInclude(includeValues)
	opts := service.BundleOpts{Past: inc.Past, Compose: compose}
	var fmts []string
	if len(formats) != 0 {
		fmts = formats
	}
	res, err := s.Svc.ExportOperativeBundle(r.Context(), q, fmts, opts)
	if err != nil {
		msg := s.clientError(err)
		switch {
		case errors.Is(err, service.ErrQueryTooLong):
			s.write(w, 400, nil, msg)
		case msg == "export disabled", strings.HasPrefix(msg, "unknown format"), msg == "empty format list",
			strings.HasPrefix(msg, "operative bundle too large"):
			s.write(w, 400, nil, msg)
		case msg == "export refused: bundle member confirmed_stale", msg == "export refused: law confirmed_stale":
			s.write(w, 502, res, msg)
		case msg == clienterr.Internal:
			s.logError(r.Context(), "bundle", err)
			s.write(w, 502, res, msg)
		default:
			s.logError(r.Context(), "bundle", err)
			s.write(w, 502, res, clienterr.Internal)
		}
		return
	}
	s.write(w, 200, res, "")
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	sectionRaw := r.URL.Query().Get("section")
	includeValues := append([]string{}, r.URL.Query()["include"]...)
	if q == "" && r.Body != nil && r.Body != http.NoBody {
		var body struct {
			Query   string `json:"query"`
			Section string `json:"section"`
			Include string `json:"include"`
		}
		if err := decodeJSON(w, r, &body); err != nil && err != io.EOF {
			s.write(w, 400, nil, "invalid json body")
			return
		}
		if q == "" {
			q = body.Query
		}
		if sectionRaw == "" {
			sectionRaw = body.Section
		}
		if body.Include != "" {
			includeValues = append(includeValues, body.Include)
		}
	}
	inc := service.MergeInclude(includeValues)
	opts := service.IndexOpts{
		Past:     inc.Past,
		Sections: export.ParseSectionRefs(sectionRaw),
	}
	res, err := s.Svc.ExportIndexChunks(r.Context(), q, opts)
	if err != nil {
		msg := s.clientError(err)
		switch {
		case errors.Is(err, service.ErrQueryTooLong):
			s.write(w, 400, nil, msg)
		case msg == "export disabled", strings.HasPrefix(msg, "operative bundle too large"),
			strings.HasPrefix(msg, "index:"):
			s.write(w, 400, nil, msg)
		case msg == "export refused: bundle member confirmed_stale", msg == "export refused: law confirmed_stale":
			s.write(w, 502, res, msg)
		case msg == clienterr.Internal:
			s.logError(r.Context(), "index", err)
			s.write(w, 502, res, msg)
		default:
			s.logError(r.Context(), "index", err)
			s.write(w, 502, res, clienterr.Internal)
		}
		return
	}
	s.write(w, 200, res, "")
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *Server) authorize(r *http.Request) bool {
	if s.SharedSecret == "" {
		return false
	}
	got := r.Header.Get("X-Gesetzeswache-Token")
	return subtle.ConstantTimeCompare([]byte(sha256Hex(got)), []byte(sha256Hex(s.SharedSecret))) == 1
}
