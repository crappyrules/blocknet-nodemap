# blocknet-nodemap
A small Go service that crawls the Blocknet libp2p network (via PEX) and serves a Leaflet-based world map of discovered nodes.

## What it shows (and what it doesn’t)
- The crawler deduplicates nodes by **public IP**, not peer ID.
  - If you run multiple peers behind the same public IP (NAT), they will appear as **one** node on the map.
- Only peers advertising **publicly routable** IP multiaddrs are kept.
- Geolocation is performed via the free `ip-api.com` batch endpoint; nodes without geo data won’t render as map markers.
- The map retains nodes **reported to or reached by the crawler within the last 12 hours** so a partial or missed crawl does not empty the map. Green nodes answered PEX in the latest completed crawl; dim nodes were reported within the 12-hour window but did not answer that crawl. A response is evidence from the crawl, not a guarantee that the peer is online now.
- The cache is in memory. Restarting the service starts a new 12-hour window.

## Build
```bash
go build -o nodemap .
```

## Run
```bash
./nodemap -listen :8782 -interval 2m
```

Flags:
- `-listen` (default `:8080`): HTTP listen address
- `-interval` (default `5m`): crawl interval
- `-node-api-url` or `NODE_API_URL` (optional): internal origin for the same host's BNT node API; when unset, network metrics show as unavailable

Endpoints:
- `GET /` – web UI
- `GET /api/nodes` – node list (with geo fields when available)
- `GET /api/stats` – 12-hour `total_nodes`, `current_nodes` that answered the latest completed crawl, `countries`, and `last_crawl`
- `GET /api/network` – cached BNT `network_hashrate` (H/s) and next-block `difficulty` from the node's public `/api/stats`; reports stale data for at most 10 minutes, then 503 unavailable

## Docker
```bash
docker build -t blocknet-nodemap .
docker run --rm -p 8080:8080 blocknet-nodemap
```

## Notes
- The frontend is embedded into the binary using Go `embed` (`//go:embed web`). If you change anything under `web/`, you must rebuild the `nodemap` binary.
- The 1200×630 social card is `web/og.png`; regenerate it with `python3 scripts/generate-social-card.py` (requires Pillow).
- Seed nodes are currently hard-coded in `main.go`.
