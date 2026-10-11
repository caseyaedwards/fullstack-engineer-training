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

	"fsp/examples/week18/your-monitor/internal/dbtest"
)

// TestHealthzAnswersForTheKubelet builds the scheduler and starts it the way
// its Deployment does, with -metrics-addr. The kubelet probes GET /healthz on
// that address: it must answer 200 while the loop runs.
func TestHealthzAnswersForTheKubelet(t *testing.T) {
	_, url := dbtest.NewWithURL(t)
	bin := filepath.Join(t.TempDir(), "scheduler")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const addr = "127.0.0.1:18150"
	cmd := exec.CommandContext(ctx, bin, "-metrics-addr", addr, "-interval", "200ms")
	cmd.Env = append(os.Environ(), "DATABASE_URL="+url)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}()

	var last string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err != nil {
			last = err.Error()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		last = resp.Status + " " + string(body)
		if resp.StatusCode == http.StatusOK && string(body) == "{\"status\":\"ok\"}\n" {
			return
		}
	}
	t.Fatalf("GET /healthz on %s never answered 200 ok; last answer: %s", addr, last)
}
