package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// region: tls-test

// TestTLSRowIsTheHandshakeOnly connects through a dialer that waits for
// gap after the TCP handshake and before TLS starts. That wait belongs to
// neither row: Connect ends before it, and TLS starts after it.
func TestTLSRowIsTheHandshakeOnly(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	const gap = 300 * time.Millisecond
	tr := srv.Client().Transport.(*http.Transport).Clone() // trusts the test certificate
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr) // fires ConnectStart and ConnectDone
		time.Sleep(gap)
		return conn, err
	}
	old := client
	client = &http.Client{Transport: tr}
	t.Cleanup(func() { client = old })

	timings, status, err := Check(context.Background(), srv.URL)
	if err != nil || status != http.StatusOK {
		t.Fatalf("Check = %d, %v; want 200 and no error", status, err)
	}
	if timings.Connect >= gap || timings.TLS >= gap {
		t.Errorf("Connect = %v, TLS = %v; want each under the %v gap between them", timings.Connect, timings.TLS, gap)
	}
	if timings.TLS == 0 {
		t.Error("TLS = 0; want the time of the handshake")
	}
}

// endregion: tls-test
