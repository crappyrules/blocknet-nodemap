package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// NetworkMetrics contains only public chain metrics from this host's BNT node.
type NetworkMetrics struct {
	Difficulty      uint64  `json:"difficulty"`
	NetworkHashrate float64 `json:"network_hashrate"`
	Height          int64   `json:"height"`
	SampledAt       int64   `json:"sampled_at"`
	Status          string  `json:"status"`
}

type networkMetricsCache struct {
	mu       sync.Mutex
	baseURL  string
	client   *http.Client
	value    NetworkMetrics
	lastTry  time.Time
	lastGood time.Time
}

func newNetworkMetricsCache(rawURL string) (*networkMetricsCache, error) {
	if rawURL == "" {
		return &networkMetricsCache{}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("node API URL must be an http(s) origin without credentials or path")
	}
	return &networkMetricsCache{baseURL: strings.TrimRight(rawURL, "/"), client: &http.Client{Timeout: 4 * time.Second}}, nil
}

func (c *networkMetricsCache) get(ctx context.Context) (NetworkMetrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.baseURL == "" {
		return NetworkMetrics{}, errors.New("node API URL is not configured")
	}
	now := time.Now()
	if !c.lastTry.IsZero() && now.Sub(c.lastTry) < time.Minute {
		return c.cached(now)
	}
	c.lastTry = now

	var stats struct {
		Height          int64   `json:"height"`
		Difficulty      uint64  `json:"difficulty"`
		NetworkHashrate float64 `json:"network_hashrate"`
	}
	var status struct {
		Syncing *bool `json:"syncing"`
	}
	requestCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	err := c.fetch(requestCtx, "/api/stats", &stats)
	if err == nil {
		err = c.fetch(requestCtx, "/api/status", &status)
	}
	if err == nil && (stats.Height <= 0 || stats.Difficulty == 0 || stats.NetworkHashrate <= 0 || status.Syncing == nil || *status.Syncing) {
		err = errors.New("node metrics unavailable or node is syncing")
	}
	if err == nil {
		c.value = NetworkMetrics{Difficulty: stats.Difficulty, NetworkHashrate: stats.NetworkHashrate, Height: stats.Height, SampledAt: now.Unix(), Status: "fresh"}
		c.lastGood = now
	}
	return c.cached(now)
}

func (c *networkMetricsCache) cached(now time.Time) (NetworkMetrics, error) {
	if c.lastGood.IsZero() || now.Sub(c.lastGood) >= 10*time.Minute {
		return NetworkMetrics{}, errors.New("node metrics unavailable")
	}
	value := c.value
	if now.Sub(c.lastGood) >= time.Minute {
		value.Status = "stale"
	}
	return value, nil
}

func (c *networkMetricsCache) fetch(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("node API returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(target)
}
