package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNetworkMetricsFreshStaleUnavailable(t *testing.T) {
	serverOK := true
	cache, err := newNetworkMetricsCache("http://node.test:8332")
	if err != nil {
		t.Fatal(err)
	}
	cache.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !serverOK {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("offline"))}, nil
		}
		body := ""
		switch r.URL.Path {
		case "/api/stats":
			body = `{"height":68889,"difficulty":19951833,"network_hashrate":63974.1}`
		case "/api/status":
			body = `{"syncing":false}`
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	value, err := cache.get(context.Background())
	if err != nil || value.Status != "fresh" || value.Difficulty != 19951833 || value.NetworkHashrate != 63974.1 {
		t.Fatalf("fresh metrics: %+v, %v", value, err)
	}

	serverOK = false
	cache.lastTry = time.Now().Add(-time.Minute)
	cache.lastGood = time.Now().Add(-2 * time.Minute)
	value, err = cache.get(context.Background())
	if err != nil || value.Status != "stale" {
		t.Fatalf("stale metrics: %+v, %v", value, err)
	}

	cache.lastTry = time.Now().Add(-time.Minute)
	cache.lastGood = time.Now().Add(-11 * time.Minute)
	if _, err := cache.get(context.Background()); err == nil {
		t.Fatal("expected unavailable metrics after stale limit")
	}
}

func TestNetworkMetricsRejectsCredentialURL(t *testing.T) {
	if _, err := newNetworkMetricsCache("http://user:secret@localhost:8332"); err == nil {
		t.Fatal("expected credential URL to be rejected")
	}
}
