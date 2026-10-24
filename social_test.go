package main

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestEmbeddedSocialCard(t *testing.T) {
	html, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, meta := range []string{
		`property="og:image" content="https://blocknetnodes.toxicrainbows.com/og.png"`,
		`name="twitter:card" content="summary_large_image"`,
		`property="og:image:alt"`,
	} {
		if !strings.Contains(string(html), meta) {
			t.Fatalf("missing social metadata %q", meta)
		}
	}
	card, err := webFS.ReadFile("web/og.png")
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(card))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1200 || config.Height != 630 {
		t.Fatalf("unexpected social card size: %dx%d", config.Width, config.Height)
	}
}
