// Package security keeps an uptime check from reaching addresses inside
// your own network: a server-side request forgery guard that checks the
// address a name resolves to at the moment of the dial.
package security

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// region: check

// ErrBlockedAddress is the error for a dial to an address that a
// user-supplied URL must never reach.
var ErrBlockedAddress = errors.New("blocked address")

var (
	thisNetwork = netip.MustParsePrefix("0.0.0.0/8")
	sharedSpace = netip.MustParsePrefix("100.64.0.0/10")
)

// CheckAddr returns an error that wraps ErrBlockedAddress when a is not a
// public unicast address.
func CheckAddr(a netip.Addr) error {
	if !a.IsValid() {
		return fmt.Errorf("%w: no address", ErrBlockedAddress)
	}
	a = a.Unmap() // ::ffff:127.0.0.1 is 127.0.0.1
	switch {
	case a.IsLoopback(), // 127.0.0.0/8, ::1
		a.IsPrivate(),          // 10/8, 172.16/12, 192.168/16, fc00::/7
		a.IsLinkLocalUnicast(), // 169.254/16, with 169.254.169.254, and fe80::/10
		a.IsUnspecified(),      // 0.0.0.0, ::
		a.IsMulticast(),        // 224/4, ff00::/8
		thisNetwork.Contains(a),
		sharedSpace.Contains(a):
		return fmt.Errorf("%w: %s", ErrBlockedAddress, a)
	}
	return nil
}

// endregion: check

// region: dialer

// SafeDialer returns a dialer that refuses to connect to a blocked address.
// Control runs after DNS resolution and before the connect, once for every
// address the dialer tries, which closes the gap that a check before the
// lookup leaves open.
func SafeDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("%w: %q: %v", ErrBlockedAddress, address, err)
			}
			return CheckAddr(ap.Addr())
		},
	}
}

// endregion: dialer

// region: client

// NewClient returns an HTTP client for user-supplied URLs. Proxy stays nil:
// through a proxy, the dialer sees only the proxy address, and the check
// guards nothing.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: SafeDialer().DialContext,
		},
	}
}

// endregion: client
