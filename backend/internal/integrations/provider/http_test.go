package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJSONRejectsMalformedAndOversizedResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: "{"},
		{name: "oversized", body: strings.Repeat("x", MaxResponseBody+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			var destination map[string]any
			if err := NewClient().JSON(context.Background(), http.MethodGet, server.URL, http.Header{}, nil, &destination, false); err == nil {
				t.Fatal("expected invalid provider response to fail")
			}
		})
	}
}

func TestJSONRetriesTransientReadOnlyCallsAtMostOnce(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	var destination map[string]any
	if err := NewClient().JSON(context.Background(), http.MethodGet, server.URL, http.Header{}, nil, &destination, true); err == nil {
		t.Fatal("expected transient provider failure")
	}
	if attempts.Load() != 2 {
		t.Fatalf("transient attempts = %d, want 2", attempts.Load())
	}
}

func TestJSONHonorsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	var destination map[string]any
	if err := NewClient().JSON(ctx, http.MethodGet, server.URL, http.Header{}, nil, &destination, false); err == nil {
		t.Fatal("expected timeout")
	}
}
