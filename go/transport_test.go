package localai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestForwardedRequestsCarryTheAPIKey(t *testing.T) {
	var seen atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Method + " " + r.URL.Path + " " + r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	key := strings.Repeat("k", 64)
	client := withAPIKey(server.Client(), key)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer caller-supplied")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got, want := seen.Load(), "POST /v1/chat/completions Bearer "+key; got != want {
		t.Fatalf("LocalAI saw %q, want %q", got, want)
	}
	if req.Header.Get("Authorization") != "Bearer caller-supplied" {
		t.Fatal("transport modified the caller's request")
	}
}

func TestOnlyChatCompletionsAreForwarded(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer server.Close()
	client := withAPIKey(server.Client(), strings.Repeat("k", 64))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/models/apply"},
		{http.MethodPost, "/backends/apply"},
		{http.MethodGet, "/backends"},
		{http.MethodPost, "/api/settings"},
		{http.MethodGet, "/v1/models"},
		{http.MethodGet, "/v1/chat/completions"},
		{http.MethodPost, "/v1/chat/completions?x=1"},
	} {
		req, err := http.NewRequest(tc.method, server.URL+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp, err := client.Do(req); !errors.Is(err, ErrEndpointRefused) {
			if resp != nil {
				resp.Body.Close()
			}
			t.Fatalf("%s %s: err = %v, want ErrEndpointRefused", tc.method, tc.path, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("LocalAI received %d refused requests", n)
	}
}
