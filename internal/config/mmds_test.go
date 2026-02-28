package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Step 2: MMDS config delivery (V2 token + V1 fallback).

func TestFetchMMDS_V2(t *testing.T) {
	cfg := RunConfig{Hostname: "v2-host", MTU: 1400}
	cfgJSON, _ := json.Marshal(cfg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/api/token"):
			// V2 token endpoint.
			if r.Header.Get("X-metadata-token-ttl-seconds") == "" {
				t.Error("V2 token request missing TTL header")
			}
			w.Write([]byte("test-token"))
		case r.Method == "GET" && r.URL.Path == "/":
			// V2 data endpoint.
			token := r.Header.Get("X-metadata-token")
			if token != "test-token" {
				t.Errorf("V2 data request token: got %q, want test-token", token)
			}
			if r.Header.Get("Accept") != "application/json" {
				t.Error("V2 data request missing Accept header")
			}
			w.Write(cfgJSON)
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer srv.Close()

	result, err := fetchMMDSFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMMDS V2: %v", err)
	}
	if result.Hostname != "v2-host" {
		t.Errorf("Hostname: got %q, want v2-host", result.Hostname)
	}
	if result.MTU != 1400 {
		t.Errorf("MTU: got %d, want 1400", result.MTU)
	}
}

func TestFetchMMDS_V1Fallback(t *testing.T) {
	cfg := RunConfig{Hostname: "v1-host"}
	cfgJSON, _ := json.Marshal(cfg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT":
			// V2 token fails.
			http.Error(w, "not implemented", 405)
		case r.Method == "GET" && r.URL.Path == "/":
			// V1 fallback succeeds.
			if r.Header.Get("Accept") != "application/json" {
				t.Error("V1 request missing Accept header")
			}
			w.Write(cfgJSON)
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer srv.Close()

	result, err := fetchMMDSFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMMDS V1 fallback: %v", err)
	}
	if result.Hostname != "v1-host" {
		t.Errorf("Hostname: got %q, want v1-host", result.Hostname)
	}
}

func TestFetchMMDS_BothFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", 500)
	}))
	defer srv.Close()

	_, err := fetchMMDSFrom(context.Background(), srv.URL)
	if err == nil {
		t.Error("FetchMMDS should fail when both V2 and V1 fail")
	}
}

func TestFetchMMDS_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT":
			w.Write([]byte("token"))
		case r.Method == "GET":
			w.Write([]byte("{invalid json"))
		}
	}))
	defer srv.Close()

	_, err := fetchMMDSFrom(context.Background(), srv.URL)
	if err == nil {
		t.Error("FetchMMDS should fail on invalid JSON")
	}
}
