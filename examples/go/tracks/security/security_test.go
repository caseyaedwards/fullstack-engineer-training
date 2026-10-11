package security

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// region: table
func TestCheckAddr(t *testing.T) {
	tests := []struct {
		addr    string
		blocked bool
		why     string
	}{
		{"127.0.0.1", true, "loopback"},
		{"127.10.20.30", true, "loopback, all of 127.0.0.0/8"},
		{"::1", true, "IPv6 loopback"},
		{"10.0.0.5", true, "private, 10.0.0.0/8"},
		{"172.16.3.4", true, "private, 172.16.0.0/12"},
		{"192.168.1.20", true, "private, 192.168.0.0/16"},
		{"169.254.169.254", true, "link-local: the cloud metadata service"},
		{"fe80::1", true, "IPv6 link-local"},
		{"0.0.0.0", true, "unspecified: Linux connects it to the local host"},
		{"0.1.2.3", true, "this network, 0.0.0.0/8"},
		{"::", true, "IPv6 unspecified"},
		{"224.0.0.1", true, "multicast"},
		{"ff02::1", true, "IPv6 multicast"},
		{"100.64.0.1", true, "shared address space, 100.64.0.0/10"},
		{"100.127.255.254", true, "shared address space, top of the range"},
		{"fd00:ec2::254", true, "IPv6 unique local: the AWS metadata service over IPv6"},
		{"::ffff:127.0.0.1", true, "IPv4-mapped loopback"},
		{"::ffff:169.254.169.254", true, "IPv4-mapped metadata address"},
		{"::ffff:100.64.0.1", true, "IPv4-mapped shared address: a prefix match needs Unmap"},
		{"203.0.113.10", false, "public, a documentation address"},
		{"198.51.100.7", false, "public, a documentation address"},
		{"2001:db8::10", false, "public IPv6, a documentation address"},
		{"100.128.0.1", false, "just above the shared address space"},
		{"172.32.0.1", false, "just above 172.16.0.0/12"},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := CheckAddr(netip.MustParseAddr(tt.addr))
			if got := errors.Is(err, ErrBlockedAddress); got != tt.blocked {
				t.Errorf("CheckAddr(%s) = %v, want blocked=%v: %s", tt.addr, err, tt.blocked, tt.why)
			}
		})
	}
	if err := CheckAddr(netip.Addr{}); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("CheckAddr(zero Addr) = %v, want ErrBlockedAddress", err)
	}
}

// endregion: table

// region: dial
func TestSafeDialerRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	// The control: a plain client reaches the server, which proves that the
	// refusal below comes from SafeDialer and not from the test setup.
	plain := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 2 * time.Second}
	resp, err := plain.Get(srv.URL)
	if err != nil {
		t.Fatalf("plain client: %v", err)
	}
	resp.Body.Close()

	client := NewClient()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	for _, u := range []string{
		srv.URL, // http://127.0.0.1:port
		"http://" + net.JoinHostPort("localhost", strconv.Itoa(port)),
	} {
		_, err := client.Get(u)
		if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("GET %s: err = %v, want ErrBlockedAddress", u, err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("server saw %d requests, want 1: only the control may reach it", n)
	}
}

// endregion: dial
