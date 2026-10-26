package main

import (
	"testing"
	"time"
)

func TestRecentlySeenUsesPeerReportAge(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for _, tc := range []struct {
		name     string
		lastSeen int64
		want     bool
	}{
		{"direct reply", now.Unix(), true},
		{"recent PEX report without direct reply", now.Add(-5 * time.Minute).Unix(), true},
		{"edge of recent window", now.Add(-recentNodeWindow).Unix(), true},
		{"older cached report", now.Add(-recentNodeWindow - time.Second).Unix(), false},
		{"too far in future", now.Add(3 * time.Minute).Unix(), false},
		{"unknown", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := recentlySeen(tc.lastSeen, now); got != tc.want {
				t.Fatalf("recentlySeen(%d) = %v, want %v", tc.lastSeen, got, tc.want)
			}
		})
	}
}
