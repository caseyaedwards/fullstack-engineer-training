package week04

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// useClient makes Check talk through c for the length of the test.
func useClient(t *testing.T, c *http.Client) {
	t.Helper()
	old := client
	client = c
	t.Cleanup(func() { client = old })
}

func TestCheckRecordsTimings(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	useClient(t, srv.Client()) // trusts the test server's self-signed certificate

	timings, status, err := Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	// DNS is 0 here: the test server is an IP literal, so there is nothing to look up.
	if timings.Connect <= 0 {
		t.Errorf("Connect = %v, want > 0", timings.Connect)
	}
	if timings.TLS <= 0 {
		t.Errorf("TLS = %v, want > 0", timings.TLS)
	}
	if timings.FirstByte <= 0 {
		t.Errorf("FirstByte = %v, want > 0", timings.FirstByte)
	}
	if timings.Total < timings.FirstByte {
		t.Errorf("Total %v < FirstByte %v", timings.Total, timings.FirstByte)
	}
}

func TestCheckHonorsContext(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	// The client gives up mid-handshake, which the server would log as an error.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	useClient(t, srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	_, _, err := Check(ctx, srv.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestCheckDoesNotFollowRedirects(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("moved here"))
	}))
	defer target.Close()
	srv := httptest.NewTLSServer(http.RedirectHandler(target.URL, http.StatusFound))
	defer srv.Close()
	useClient(t, srv.Client())

	_, status, err := Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// An uptime check reports what the URL itself answered, so the timings
	// never mix two servers.
	if status != http.StatusFound {
		t.Errorf("status = %d, want 302", status)
	}
}

func TestCheckOpensFreshConnectionEachTime(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	useClient(t, srv.Client())

	// A monitor checks the same URL again and again. If the second check reused
	// the first one's connection, it would skip connect and TLS and report zero.
	for i := range 2 {
		timings, _, err := Check(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("check %d: %v", i+1, err)
		}
		if timings.Connect <= 0 || timings.TLS <= 0 {
			t.Errorf("check %d: Connect = %v, TLS = %v, want both > 0", i+1, timings.Connect, timings.TLS)
		}
	}
}
