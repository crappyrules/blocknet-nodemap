package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/netip"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

const (
	ProtocolPEX    protocol.ID = "/blocknet/mainnet/pex/1.0.0"
	PEXMsgGetPeers byte        = 0x01
	PEXMsgPeers    byte        = 0x02

	maxPEXPayload   = 512 * 1024
	crawlTimeout    = 30 * time.Second
	exchangeTimeout = 15 * time.Second
)

// PeerRecord matches blocknet's PEX peer record format.
type PeerRecord struct {
	ID       string   `json:"id"`
	Addrs    []string `json:"addrs"`
	LastSeen int64    `json:"last_seen"`
	Score    int      `json:"score"`
}

// DiscoveredNode represents a node found during crawling.
type DiscoveredNode struct {
	PeerID   string   `json:"peer_id"`
	IP       string   `json:"ip"`
	Addrs    []string `json:"addrs"`
	LastSeen int64    `json:"last_seen"`
}

// Crawler crawls the blocknet P2P network via PEX.
type Crawler struct {
	mu    sync.RWMutex
	host  host.Host
	seeds []string
	nodes map[string]*DiscoveredNode // keyed by IP

	lastCrawl time.Time
	crawling  bool
}

// NewCrawler creates a new network crawler.
func NewCrawler(seeds []string) (*Crawler, error) {
	h, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.UserAgent("blocknet-nodemap/1.0"),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	return &Crawler{
		host:  h,
		seeds: seeds,
		nodes: make(map[string]*DiscoveredNode),
	}, nil
}

// Close shuts down the crawler's libp2p host.
func (c *Crawler) Close() error {
	return c.host.Close()
}

// Nodes returns the current snapshot of discovered nodes.
func (c *Crawler) Nodes() []*DiscoveredNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]*DiscoveredNode, 0, len(c.nodes))
	for _, n := range c.nodes {
		out = append(out, n)
	}
	return out
}

// LastCrawl returns when the last crawl completed.
func (c *Crawler) LastCrawl() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCrawl
}

// Crawl performs a full network crawl starting from seed nodes.
func (c *Crawler) Crawl(ctx context.Context) error {
	c.mu.Lock()
	if c.crawling {
		c.mu.Unlock()
		return fmt.Errorf("crawl already in progress")
	}
	c.crawling = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.crawling = false
		c.lastCrawl = time.Now()
		c.mu.Unlock()
	}()

	// Parse seed addresses.
	seedInfos := make([]peer.AddrInfo, 0, len(c.seeds))
	for _, addr := range c.seeds {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			log.Printf("crawler: invalid seed addr %s: %v", addr, err)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			log.Printf("crawler: invalid seed peer addr %s: %v", addr, err)
			continue
		}
		seedInfos = append(seedInfos, *pi)
	}

	if len(seedInfos) == 0 {
		return fmt.Errorf("no valid seed nodes")
	}

	// BFS crawl: visited tracks peer IDs we've already queried.
	visited := make(map[peer.ID]bool)
	discovered := make(map[string]*DiscoveredNode) // keyed by IP
	queue := make([]peer.AddrInfo, 0, 256)

	// Seed the queue.
	for _, si := range seedInfos {
		queue = append(queue, si)
		// Also record seeds as discovered nodes.
		for _, addr := range si.Addrs {
			if ip := extractIP(addr); ip != "" {
				discovered[ip] = &DiscoveredNode{
					PeerID:   si.ID.String(),
					IP:       ip,
					Addrs:    []string{addr.String()},
					LastSeen: time.Now().Unix(),
				}
			}
		}
	}

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Pop next peer.
		pi := queue[0]
		queue = queue[1:]

		if visited[pi.ID] {
			continue
		}
		visited[pi.ID] = true

		// Connect and exchange.
		records, err := c.exchangeWith(ctx, pi)
		if err != nil {
			continue
		}

		// Process discovered peers.
		for _, rec := range records {
			pid, err := peer.Decode(rec.ID)
			if err != nil {
				continue
			}

			addrs := make([]multiaddr.Multiaddr, 0, len(rec.Addrs))
			addrStrs := make([]string, 0, len(rec.Addrs))
			for _, a := range rec.Addrs {
				ma, err := multiaddr.NewMultiaddr(a)
				if err != nil {
					continue
				}
				if isRoutable(ma) {
					addrs = append(addrs, ma)
					addrStrs = append(addrStrs, a)
				}
			}

			for _, addr := range addrs {
				if ip := extractIP(addr); ip != "" {
					if existing, ok := discovered[ip]; ok {
						// Update last seen if newer.
						if rec.LastSeen > existing.LastSeen {
							existing.LastSeen = rec.LastSeen
						}
					} else {
						discovered[ip] = &DiscoveredNode{
							PeerID:   rec.ID,
							IP:       ip,
							Addrs:    addrStrs,
							LastSeen: rec.LastSeen,
						}
					}
				}
			}

			// Enqueue for further crawling if not yet visited.
			if !visited[pid] && len(addrs) > 0 {
				queue = append(queue, peer.AddrInfo{ID: pid, Addrs: addrs})
			}
		}
	}

	// Disconnect from all peers after crawl.
	for _, conn := range c.host.Network().Conns() {
		_ = conn.Close()
	}

	// Update the node map.
	c.mu.Lock()
	c.nodes = discovered
	c.mu.Unlock()

	log.Printf("crawler: found %d unique IPs from %d peers", len(discovered), len(visited))
	return nil
}

// exchangeWith connects to a peer and requests their peer list via PEX.
func (c *Crawler) exchangeWith(ctx context.Context, pi peer.AddrInfo) ([]PeerRecord, error) {
	connectCtx, cancel := context.WithTimeout(ctx, crawlTimeout)
	defer cancel()

	if err := c.host.Connect(connectCtx, pi); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", pi.ID.String()[:16], err)
	}

	streamCtx, streamCancel := context.WithTimeout(ctx, exchangeTimeout)
	defer streamCancel()

	s, err := c.host.NewStream(streamCtx, pi.ID, ProtocolPEX)
	if err != nil {
		return nil, fmt.Errorf("open PEX stream to %s: %w", pi.ID.String()[:16], err)
	}
	defer s.Close()

	// Send GetPeers: [type byte] + [4-byte length] + [payload]
	if _, err := s.Write([]byte{PEXMsgGetPeers}); err != nil {
		return nil, err
	}
	// Empty payload, length = 0.
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, 0)
	if _, err := s.Write(lenBuf); err != nil {
		return nil, err
	}

	// Read response: [type byte] + [4-byte length] + [JSON payload]
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(s, typeBuf); err != nil {
		return nil, err
	}
	if typeBuf[0] != PEXMsgPeers {
		return nil, fmt.Errorf("unexpected PEX response type: %d", typeBuf[0])
	}

	if _, err := io.ReadFull(s, lenBuf); err != nil {
		return nil, err
	}
	payloadLen := binary.BigEndian.Uint32(lenBuf)
	if payloadLen > maxPEXPayload {
		return nil, fmt.Errorf("PEX payload too large: %d", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(s, payload); err != nil {
		return nil, err
	}

	var records []PeerRecord
	if err := json.Unmarshal(payload, &records); err != nil {
		return nil, fmt.Errorf("decode PEX response: %w", err)
	}

	return records, nil
}

// extractIP pulls the IP address string from a multiaddr.
func extractIP(ma multiaddr.Multiaddr) string {
	if ip4, err := ma.ValueForProtocol(multiaddr.P_IP4); err == nil && ip4 != "" {
		return ip4
	}
	if ip6, err := ma.ValueForProtocol(multiaddr.P_IP6); err == nil && ip6 != "" {
		return ip6
	}
	return ""
}

// isRoutable checks if a multiaddr contains a publicly routable IP.
func isRoutable(ma multiaddr.Multiaddr) bool {
	ipStr := extractIP(ma)
	if ipStr == "" {
		return false
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	if !addr.IsValid() || !addr.IsGlobalUnicast() {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	return true
}

// RunCrawlLoop runs periodic crawls in the background.
func (c *Crawler) RunCrawlLoop(ctx context.Context, interval time.Duration) {
	// Initial crawl immediately.
	if err := c.Crawl(ctx); err != nil && ctx.Err() == nil {
		log.Printf("crawler: initial crawl failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Crawl(ctx); err != nil && ctx.Err() == nil {
				log.Printf("crawler: crawl failed: %v", err)
			}
		}
	}
}
