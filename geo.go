package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// GeoResult holds geolocation data for a single IP.
type GeoResult struct {
	IP      string  `json:"query"`
	Status  string  `json:"status"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	ISP     string  `json:"isp"`
}

// GeoCache provides batch IP geolocation with in-memory caching.
type GeoCache struct {
	mu    sync.RWMutex
	cache map[string]*GeoResult
}

// NewGeoCache creates a new geolocation cache.
func NewGeoCache() *GeoCache {
	return &GeoCache{
		cache: make(map[string]*GeoResult),
	}
}

// Lookup returns cached geolocation data for an IP, or nil if not cached.
func (g *GeoCache) Lookup(ip string) *GeoResult {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cache[ip]
}

// BatchLookup geolocates a list of IPs, using cache where possible.
// Returns results for all IPs (cached + freshly resolved).
func (g *GeoCache) BatchLookup(ips []string) (map[string]*GeoResult, error) {
	results := make(map[string]*GeoResult, len(ips))
	var uncached []string

	g.mu.RLock()
	for _, ip := range ips {
		if r, ok := g.cache[ip]; ok {
			results[ip] = r
		} else {
			uncached = append(uncached, ip)
		}
	}
	g.mu.RUnlock()

	if len(uncached) == 0 {
		return results, nil
	}

	// ip-api.com batch endpoint: max 100 per request.
	const batchSize = 100
	for i := 0; i < len(uncached); i += batchSize {
		end := i + batchSize
		if end > len(uncached) {
			end = len(uncached)
		}
		batch := uncached[i:end]

		resolved, err := batchGeolocate(batch)
		if err != nil {
			return results, fmt.Errorf("batch geolocate: %w", err)
		}

		g.mu.Lock()
		for _, r := range resolved {
			if r.Status == "success" {
				g.cache[r.IP] = r
				results[r.IP] = r
			}
		}
		g.mu.Unlock()

		// Rate limit: ip-api.com allows 15 requests/minute for free tier.
		if end < len(uncached) {
			time.Sleep(4 * time.Second)
		}
	}

	return results, nil
}

// batchGeolocate calls ip-api.com batch endpoint.
func batchGeolocate(ips []string) ([]*GeoResult, error) {
	body, err := json.Marshal(ips)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "http://ip-api.com/batch?fields=query,status,country,city,lat,lon,isp", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited by ip-api.com")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ip-api.com returned status %d", resp.StatusCode)
	}

	var results []*GeoResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}
