package ratelimit

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// region: client-ip-test

func TestClientIP(t *testing.T) {
	// 10.42.0.0/16 is the pod network of k3s: Traefik and nginx live there.
	trusted, err := ParsePrefixes("10.42.0.0/16, 2001:db8:ffff::/48")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		remote string
		xff    []string // one entry per X-Forwarded-For line
		want   string
	}{
		{"direct client, no header", "203.0.113.7:51000", nil, "203.0.113.7"},
		{"direct client spoofs the header", "203.0.113.7:51000", []string{"198.51.100.1"}, "203.0.113.7"},
		{"through Traefik", "10.42.0.9:40000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"through Traefik and nginx", "10.42.0.12:40000", []string{"203.0.113.7, 10.42.0.9"}, "203.0.113.7"},
		{"client spoofs behind a proxy that appends", "10.42.0.9:40000", []string{"198.51.100.1, 203.0.113.7"}, "203.0.113.7"},
		{"client claims a trusted address", "10.42.0.9:40000", []string{"10.42.0.50, 203.0.113.7"}, "203.0.113.7"},
		{"header split over two lines", "10.42.0.12:40000", []string{"198.51.100.1", "203.0.113.7, 10.42.0.9"}, "203.0.113.7"},
		{"garbage from a trusted hop", "10.42.0.12:40000", []string{"203.0.113.7, not-an-ip"}, "10.42.0.12"},
		{"garbage left of the client", "10.42.0.9:40000", []string{"<script>, 203.0.113.7"}, "203.0.113.7"},
		{"pod inside the cluster", "10.42.0.30:40000", nil, "10.42.0.30"},
		{"IPv6 client through a v6 proxy", "[2001:db8:ffff::5]:443", []string{"2001:db8:1:2::77"}, "2001:db8:1:2::77"},
		{"IPv4 in IPv6 form", "[::ffff:10.42.0.9]:40000", []string{"::ffff:203.0.113.7"}, "203.0.113.7"},
		{"not a TCP peer", "pipe", []string{"203.0.113.7"}, "invalid IP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/monitors", nil)
			r.RemoteAddr = tt.remote
			for _, line := range tt.xff {
				r.Header.Add("X-Forwarded-For", line)
			}
			if got := ClientIP(r, trusted).String(); got != tt.want {
				t.Errorf("ClientIP = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestClientIPTrustsNobodyByDefault: without -trusted-proxies, the header
// never counts, whoever sends it.
func TestClientIPTrustsNobodyByDefault(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.42.0.9:40000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := ClientIP(r, nil).String(); got != "10.42.0.9" {
		t.Errorf("ClientIP = %s, want the peer 10.42.0.9", got)
	}
}

// endregion: client-ip-test

func TestParsePrefixes(t *testing.T) {
	nets, err := ParsePrefixes(" 10.42.0.0/16 ,192.0.2.7,, 2001:db8::/32")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.42.0.0/16", "192.0.2.7/32", "2001:db8::/32"}
	if len(nets) != len(want) {
		t.Fatalf("got %v, want %v", nets, want)
	}
	for i := range want {
		if nets[i].String() != want[i] {
			t.Errorf("network %d = %s, want %s", i, nets[i], want[i])
		}
	}
	for _, bad := range []string{"10.42.0.0/33", "traefik", "10.42.0.0/16, 300.1.1.1"} {
		if _, err := ParsePrefixes(bad); err == nil {
			t.Errorf("ParsePrefixes(%q): got nil, want an error", bad)
		}
	}
}

func TestKey(t *testing.T) {
	for addr, want := range map[string]string{
		"203.0.113.7":         "203.0.113.7/32",
		"2001:db8:1:2::77":    "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::": "2001:db8:1:2::/64",
		"2001:db8:1:3::77":    "2001:db8:1:3::/64",
	} {
		if got := Key(netip.MustParseAddr(addr)).String(); got != want {
			t.Errorf("Key(%s) = %s, want %s", addr, got, want)
		}
	}
}

// region: limiter-test

func TestLimiterBurstThenRefill(t *testing.T) {
	l := New(2, 3) // 3 at once, then 2 per second
	alice := Key(netip.MustParseAddr("203.0.113.7"))
	bob := Key(netip.MustParseAddr("198.51.100.1"))
	start := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	for i := range 3 {
		if wait := l.Allow(alice, start); wait != 0 {
			t.Fatalf("request %d of the burst: wait %v, want 0", i+1, wait)
		}
	}
	if wait := l.Allow(alice, start); wait != 500*time.Millisecond {
		t.Errorf("fourth request: wait %v, want 500ms, one token at 2 per second", wait)
	}
	// A refused request takes no token: after the 500 ms, one is there.
	if wait := l.Allow(alice, start.Add(500*time.Millisecond)); wait != 0 {
		t.Errorf("after 500ms: wait %v, want 0", wait)
	}
	if wait := l.Allow(bob, start); wait != 0 {
		t.Errorf("another client: wait %v, want 0, its own bucket", wait)
	}
}

func TestLimiterCapsItsMemory(t *testing.T) {
	l := New(rate.Every(time.Second), 1) // a bucket refills in 1 s
	l.max = 3
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	for i := range 3 {
		l.Allow(Key(netip.AddrFrom4([4]byte{198, 51, 100, byte(i)})), now)
	}
	newcomer := Key(netip.MustParseAddr("203.0.113.7"))
	if wait := l.Allow(newcomer, now); wait != time.Second {
		t.Errorf("new client while full of fresh buckets: wait %v, want 1s", wait)
	}
	// One second later every bucket is full again, the sweep deletes them,
	// and the newcomer gets a bucket.
	if wait := l.Allow(newcomer, now.Add(time.Second)); wait != 0 {
		t.Errorf("after the refill: wait %v, want 0", wait)
	}
	if n := len(l.buckets); n != 1 {
		t.Errorf("%d buckets after the sweep, want 1", n)
	}
}

// endregion: limiter-test
