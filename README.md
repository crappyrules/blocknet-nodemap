# blocknet-nodemap
A small Go service that crawls the Blocknet libp2p network (via PEX) and serves a Leaflet-based world map of discovered nodes.

## What it shows (and what it doesn’t)
- The crawler deduplicates nodes by **public IP**, not peer ID.
  - If you run multiple peers behind the same public IP (NAT), they will appear as **one** node on the map.
- Only peers advertising **publicly routable** IP multiaddrs are kept.
- Geolocation is performed via the free `ip-api.com` batch endpoint; nodes without geo data won’t render as map markers.

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

Endpoints:
- `GET /` – web UI
- `GET /api/nodes` – node list (with geo fields when available)
- `GET /api/stats` – simple stats (`total_nodes`, `countries`, `last_crawl`)

## Docker
```bash
docker build -t blocknet-nodemap .
docker run --rm -p 8080:8080 blocknet-nodemap
```

## Notes
- The frontend is embedded into the binary using Go `embed` (`//go:embed web`). If you change anything under `web/`, you must rebuild the `nodemap` binary.
- Seed nodes are currently hard-coded in `main.go`.
