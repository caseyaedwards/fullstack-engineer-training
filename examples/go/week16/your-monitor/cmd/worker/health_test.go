package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"fsp/examples/week16/your-monitor/internal/db/dbtest"
)

// TestHealthzAnswersForTheKubelet builds the worker and starts it the way its
// Deployment does, with -debug-addr, and with a 400ms check timeout and a
// 100ms poll. The kubelet probes GET /healthz on that address. It must answer
// 200 at once, and still answer 200 after more than one maxAge: only a loop
// that beats keeps it there.
func TestHealthzAnswersForTheKubelet(t *testing.T) {
	url := dbtest.URL(t)
	bin := filepath.Join(t.TempDir(), "worker")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const addr = "127.0.0.1:18165"
	cmd := exec.CommandContext(ctx, bin, "-debug-addr", addr, "-timeout", "400ms", "-poll", "100ms")
	cmd.Env = append(os.Environ(), "DATABASE_URL="+url)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}()

	get := func() (int, string, error) {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body), err
	}
	const ok = "{\"status\":\"ok\"}\n"
	up := false
	for deadline := time.Now().Add(10 * time.Second); !up && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		code, body, err := get()
		up = err == nil && code == http.StatusOK && body == ok
	}
	if !up {
		t.Fatalf("GET /healthz on %s never answered 200 ok", addr)
	}
	// NewHealth beats once at the start, so the first 200 proves only the
	// route. For 2.5s, longer than maxAge (3 × (timeout + poll), 1.5s),
	// every answer must still be 200: without Beat in the loop, /healthz
	// turns 503.
	for end := time.Now().Add(2500 * time.Millisecond); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		code, body, err := get()
		if err != nil || code != http.StatusOK || body != ok {
			t.Fatalf("GET /healthz after the start: %d %q %v; want 200 ok while the loop runs", code, body, err)
		}
	}

	// pprof belongs on -pprof-addr only, which takes loopback addresses.
	// Package obs imports net/http/pprof, which registers its pages on
	// http.DefaultServeMux: a -debug-addr server on that mux would hand
	// pprof to every pod in the cluster.
	resp, err := http.Get("http://" + addr + "/debug/pprof/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /debug/pprof/ on -debug-addr: %s, want 404 Not Found", resp.Status)
	}
}
