package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthProbeURL(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{":8080", "http://127.0.0.1:8080/healthz"},
		{"127.0.0.1:9090", "http://127.0.0.1:9090/healthz"},
		{"0.0.0.0:8080", "http://127.0.0.1:8080/healthz"},
		{"[::]:3000", "http://127.0.0.1:3000/healthz"},
		{"", "http://127.0.0.1:8080/healthz"},
	}
	for _, tc := range cases {
		got, err := HealthProbeURL(tc.addr)
		if err != nil {
			t.Fatalf("addr=%q: %v", tc.addr, err)
		}
		if got != tc.want {
			t.Fatalf("addr=%q got %q want %q", tc.addr, got, tc.want)
		}
	}
}

func TestProbeHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	if err := ProbeHealth(srv.URL); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestProbeHealth_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	if err := ProbeHealth(srv.URL); err == nil {
		t.Fatal("want error on 500")
	}
}

func TestProbeHealth_connectionRefused(t *testing.T) {
	if err := ProbeHealth("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("want error on connection refused")
	}
}
