package main

import (
	"testing"
	"time"
)

func TestPublishNodesCopiesMutableCrawlData(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	c := &Crawler{now: func() time.Time { return now }}
	discovered := map[string]*DiscoveredNode{
		"192.0.2.1": {
			IP:         "192.0.2.1",
			Addrs:      []string{"/ip4/192.0.2.1/tcp/28080"},
			LastSeen:   1,
			ObservedAt: now.Unix(),
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

func TestNodeCacheSurvivesPartialCrawlAndExpiresAfterTwelveHours(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	c := &Crawler{now: func() time.Time { return now }}
	first := map[string]*DiscoveredNode{
		"192.0.2.1": {IP: "192.0.2.1", ObservedAt: now.Unix()},
		"192.0.2.2": {IP: "192.0.2.2", ObservedAt: now.Unix()},
		"192.0.2.3": {IP: "192.0.2.3", ObservedAt: now.Unix()},
	}
	c.completeCrawl(first, map[string]bool{"192.0.2.1": true, "192.0.2.2": true})
	firstCrawlTime := c.LastCrawl()
	byIP := map[string]*DiscoveredNode{}
	for _, node := range c.Nodes() {
		byIP[node.IP] = node
	}
	if len(byIP) != 3 || !byIP["192.0.2.1"].InLastCrawl || !byIP["192.0.2.2"].InLastCrawl || byIP["192.0.2.3"].InLastCrawl {
		t.Fatalf("reported peer was marked responsive: %+v", byIP)
	}

	now = now.Add(time.Hour)
	second := map[string]*DiscoveredNode{
		"192.0.2.2": {IP: "192.0.2.2", ObservedAt: now.Unix()},
	}
	c.publishNodes(second)
	if got := c.Nodes(); len(got) != 3 {
		t.Fatalf("partial crawl dropped cached nodes: %+v", got)
	}
	if !c.LastCrawl().Equal(firstCrawlTime) {
		t.Fatal("partial crawl replaced the latest completed crawl time")
	}
	c.completeCrawl(second, map[string]bool{"192.0.2.2": true})
	byIP = map[string]*DiscoveredNode{}
	for _, node := range c.Nodes() {
		byIP[node.IP] = node
	}
	if len(byIP) != 3 || byIP["192.0.2.1"].InLastCrawl || !byIP["192.0.2.2"].InLastCrawl || byIP["192.0.2.3"].InLastCrawl {
		t.Fatalf("latest-crawl status wrong: %+v", byIP)
	}
	now = now.Add(time.Minute)
	c.completeCrawl(nil, nil)
	for _, node := range c.Nodes() {
		if node.InLastCrawl {
			t.Fatalf("empty completed crawl reported a cached node as current: %+v", node)
		}
	}

	now = time.Unix(1_000_000, 0).Add(nodeCacheWindow + time.Second)
	if got := c.Nodes(); len(got) != 1 || got[0].IP != "192.0.2.2" {
		t.Fatalf("expired entry still visible: %+v", got)
	}
	c.publishNodes(nil)
	if len(c.nodes) != 1 {
		t.Fatalf("expired entry retained in cache: %+v", c.nodes)
	}
}
