//go:build linux

package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	return NewServer(nil, []string{"PATH=/bin"}, logger)
}

func TestHandleStatus(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status code: got %d, want 200", rec.Code)
	}

	var body map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body["ok"] {
		t.Errorf("status: got %v, want ok=true", body)
	}
}

func TestHandleSignal_InvalidBody(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/v1/signals", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want 400", rec.Code)
	}
}

func TestHandleSignal_InvalidSignalNumber(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name string
		sig  int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too_high", 65},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]int{"signal": tt.sig})
			req := httptest.NewRequest("POST", "/v1/signals", bytes.NewBuffer(body))
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("signal %d: status code: got %d, want 400", tt.sig, rec.Code)
			}
		})
	}
}

func TestHandleExec_InvalidBody(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/v1/exec", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want 400", rec.Code)
	}
}

func TestHandleExec_EmptyCmd(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(map[string][]string{"cmd": {}})
	req := httptest.NewRequest("POST", "/v1/exec", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want 400", rec.Code)
	}
}

func TestRouteRegistration(t *testing.T) {
	srv := newTestServer(t)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/status"},
		{"POST", "/v1/signals"},
		{"POST", "/v1/exec"},
		{"GET", "/v1/ws/exec"},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, nil)
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)

			// Any registered route should NOT return 404.
			if rec.Code == http.StatusNotFound {
				t.Errorf("%s %s: route not registered (got 404)", r.method, r.path)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type: got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("body: got %v", body)
	}
}
