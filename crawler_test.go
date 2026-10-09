package main

import "testing"

func TestPublishNodesCopiesMutableCrawlData(t *testing.T) {
	c := &Crawler{}
	discovered := map[string]*DiscoveredNode{
		"192.0.2.1": {
			IP:       "192.0.2.1",
			Addrs:    []string{"/ip4/192.0.2.1/tcp/28080"},
			LastSeen: 1,
		},
	}

	c.publishNodes(discovered)
	discovered["192.0.2.1"].LastSeen = 2
	discovered["192.0.2.1"].Addrs[0] = "changed"
	discovered["192.0.2.2"] = &DiscoveredNode{IP: "192.0.2.2"}

	nodes := c.Nodes()
	if len(nodes) != 1 || nodes[0].LastSeen != 1 || nodes[0].Addrs[0] != "/ip4/192.0.2.1/tcp/28080" {
		t.Fatalf("published nodes changed with the ongoing crawl: %+v", nodes)
	}
}
