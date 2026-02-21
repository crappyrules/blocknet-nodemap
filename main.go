package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

// Default seed nodes (from blocknet daemon.go).
var defaultSeeds = []string{
	"/ip4/46.62.203.242/tcp/28080/p2p/12D3KooWQUNGJrsU5nRXNk45FT3ZumdtWC9Sg9Xt2AgU3XkP382R",
	"/ip4/46.62.243.192/tcp/28080/p2p/12D3KooWSQTy8rav5nmapgxomAMpSrigJTUXHmjH25dtHhGU3BAM",
	"/ip4/46.62.252.254/tcp/28080/p2p/12D3KooWCJzbwahELLrssLMbvxCXAp2HeeH1nPTYHH1kFERZxxb8",
	"/ip4/46.62.202.165/tcp/28080/p2p/12D3KooWSAWJofx3Gk9Rbgs5UQFsUVCDkMDybr85v2j5gN3ocnUL",
	"/ip4/46.62.249.240/tcp/28080/p2p/12D3KooWKSgY1tDEfTNXo7AEE3tzsrnieEHmcUjYJfGz1LF72UJt",
	"/ip4/46.62.201.220/tcp/28080/p2p/12D3KooWPMeQZB8pJTavN6KXJ12LpMYfBSYDkV1md2xsYaWC8VMa",
}

// NodeInfo is the JSON structure served by /api/nodes.
type NodeInfo struct {
	PeerID   string  `json:"peer_id"`
	IP       string  `json:"ip"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Country  string  `json:"country"`
	City     string  `json:"city"`
	ISP      string  `json:"isp"`
	LastSeen int64   `json:"last_seen"`
}

func main() {
	listenAddr := flag.String("listen", ":8080", "HTTP listen address")
	crawlInterval := flag.Duration("interval", 5*time.Minute, "Crawl interval")
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lshortfile)

	// Create crawler.
	crawler, err := NewCrawler(defaultSeeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create crawler: %v\n", err)
		os.Exit(1)
	}
	defer crawler.Close()

	// Create geo cache.
	geo := NewGeoCache()

	// Context with signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	// Start crawl loop in background.
	go crawler.RunCrawlLoop(ctx, *crawlInterval)

	// HTTP server.
	mux := http.NewServeMux()

	// Serve embedded web files.
	webSub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Failed to get web sub-filesystem: %v", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(webSub)))

	// API: node list with geolocation.
	mux.HandleFunc("GET /api/nodes", func(w http.ResponseWriter, r *http.Request) {
		nodes := crawler.Nodes()

		// Collect IPs for batch geolocation.
		ips := make([]string, 0, len(nodes))
		for _, n := range nodes {
			ips = append(ips, n.IP)
		}

		geoResults, err := geo.BatchLookup(ips)
		if err != nil {
			log.Printf("geo lookup error: %v", err)
		}

		// Build response.
		infos := make([]NodeInfo, 0, len(nodes))
		for _, n := range nodes {
			info := NodeInfo{
				PeerID:   n.PeerID,
				IP:       n.IP,
				LastSeen: n.LastSeen,
			}
			if gr, ok := geoResults[n.IP]; ok {
				info.Lat = gr.Lat
				info.Lng = gr.Lon
				info.Country = gr.Country
				info.City = gr.City
				info.ISP = gr.ISP
			}
			infos = append(infos, info)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infos)
	})

	// API: stats.
	mux.HandleFunc("GET /api/stats", func(w http.ResponseWriter, r *http.Request) {
		nodes := crawler.Nodes()
		countries := make(map[string]bool)
		for _, n := range nodes {
			if gr := geo.Lookup(n.IP); gr != nil {
				countries[gr.Country] = true
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"total_nodes": len(nodes),
			"countries":   len(countries),
			"last_crawl":  crawler.LastCrawl().Unix(),
		})
	})

	srv := &http.Server{
		Addr:         *listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Run server in goroutine.
	go func() {
		log.Printf("Node map server listening on %s", *listenAddr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown.
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
