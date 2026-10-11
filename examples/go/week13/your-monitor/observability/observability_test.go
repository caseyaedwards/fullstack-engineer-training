// Package observability holds no Go code: only the configuration that
// Compose mounts into Prometheus, the collector and Grafana. These tests
// keep that configuration in step with the Go programs: every metric and
// label a query names exists in the code, every scrape target is a port a
// program listens on, and every program sends spans to the collector.
package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// goSources returns the non-test Go files of the repository, by path.
func goSources(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files[path] = read(t, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// metrics returns every metric name the Go code registers, with the
// series names a histogram adds, and every label name it declares.
func metrics(t *testing.T) (names, labels map[string]bool) {
	t.Helper()
	names, labels = map[string]bool{}, map[string]bool{"le": true, "job": true, "instance": true}
	opts := regexp.MustCompile(`prometheus\.New(Counter|Gauge|Histogram)(?:Vec)?\(prometheus\.\w+Opts\{\s*Name:\s*"(\w+)"[\s\S]*?\}(?:, \[\]string\{([^}]*)\})?\)`)
	for _, src := range goSources(t) {
		for _, m := range opts.FindAllStringSubmatch(src, -1) {
			names[m[2]] = true
			if m[1] == "Histogram" {
				for _, suffix := range []string{"_bucket", "_count", "_sum"} {
					names[m[2]+suffix] = true
				}
			}
			for _, l := range regexp.MustCompile(`"(\w+)"`).FindAllStringSubmatch(m[3], -1) {
				labels[l[1]] = true
			}
		}
	}
	return names, labels
}

func TestTheCodeRegistersTheMetricsOfTheContract(t *testing.T) {
	names, labels := metrics(t)
	for _, n := range []string{"monitor_checks_total", "monitor_check_duration_seconds_bucket", "monitor_jobs_queued",
		"monitor_jobs_oldest_seconds", "http_requests_total", "http_request_duration_seconds_bucket"} {
		if !names[n] {
			t.Errorf("no metric %s in the Go code; found %v", n, names)
		}
	}
	for _, l := range []string{"result", "method", "route", "code"} {
		if !labels[l] {
			t.Errorf("no label %s in the Go code", l)
		}
	}
}

// region: dashboard-test

// TestDashboardQueriesNameRealMetrics reads every PromQL query of the
// dashboard and checks each metric and label it names against the code.
func TestDashboardQueriesNameRealMetrics(t *testing.T) {
	names, labels := metrics(t)
	var dash struct {
		Panels []struct {
			Title   string
			Targets []struct {
				Expr       string
				Datasource struct{ UID string }
			}
		}
	}
	if err := json.Unmarshal([]byte(read(t, "grafana/dashboards/your-monitor.json")), &dash); err != nil {
		t.Fatal(err)
	}
	words := regexp.MustCompile(`[a-z_]+`)
	grouped := regexp.MustCompile(`by \(([^)]*)\)|\{(\w+)=`)
	queries := 0
	for _, p := range dash.Panels {
		for _, q := range p.Targets {
			queries++
			if q.Datasource.UID != "prometheus" {
				t.Errorf("%s: data source %q, want the provisioned uid prometheus", p.Title, q.Datasource.UID)
			}
			found := false
			for _, w := range words.FindAllString(q.Expr, -1) {
				if strings.HasPrefix(w, "monitor_") || strings.HasPrefix(w, "http_") {
					found = true
					if !names[w] {
						t.Errorf("%s: %s names metric %s, which the code does not register", p.Title, q.Expr, w)
					}
				}
			}
			if !found {
				t.Errorf("%s: %s names no metric of the monitor", p.Title, q.Expr)
			}
			for _, m := range grouped.FindAllStringSubmatch(q.Expr, -1) {
				for _, l := range strings.Split(m[1]+m[2], ",") {
					if l = strings.TrimSpace(l); l != "" && !labels[l] {
						t.Errorf("%s: %s uses label %s, which no metric has", p.Title, q.Expr, l)
					}
				}
			}
		}
	}
	if queries < 6 {
		t.Errorf("the dashboard has %d queries, want at least 6", queries)
	}
}

// endregion: dashboard-test

// TestScrapeTargetsMatchCompose checks that Prometheus scrapes each
// program on the port that compose.yaml tells it to listen on.
func TestScrapeTargetsMatchCompose(t *testing.T) {
	prom := read(t, "prometheus.yml")
	compose := read(t, "../compose.yaml")
	for _, c := range []struct{ target, command string }{
		{`"api:2112"`, `command: ["-metrics-addr", ":2112"]`},
		{`"scheduler:2112"`, `command: ["-metrics-addr", ":2112"]`},
		{`"worker:6060"`, `"-debug-addr", ":6060"]`},
		{`"otel-collector:8888"`, ``},
	} {
		if !strings.Contains(prom, c.target) {
			t.Errorf("prometheus.yml does not scrape %s", c.target)
		}
		if !strings.Contains(compose, c.command) {
			t.Errorf("compose.yaml has no %s", c.command)
		}
	}
	if !strings.Contains(read(t, "otel-collector.yaml"), "port: 8888") {
		t.Error("the collector does not serve its own metrics on port 8888")
	}
}

// TestSpansReachTheCollector checks the path of a span: each program that
// traces sends to the collector, the collector receives there and exports
// to Jaeger, and Jaeger runs in Compose.
func TestSpansReachTheCollector(t *testing.T) {
	compose := read(t, "../compose.yaml")
	collector := read(t, "otel-collector.yaml")
	if n := strings.Count(compose, "OTEL_EXPORTER_OTLP_ENDPOINT: http://otel-collector:4318"); n != 2 {
		t.Errorf("%d services send spans to http://otel-collector:4318, want 2: api and worker", n)
	}
	for path, src := range goSources(t) {
		if strings.Contains(src, "obs.StartTracing(") && !slices.Contains([]string{"cmd/api", "cmd/worker"}, filepath.Base(filepath.Dir(filepath.Dir(path)))+"/"+filepath.Base(filepath.Dir(path))) {
			t.Errorf("%s starts tracing, but compose.yaml gives it no collector endpoint", path)
		}
	}
	for _, want := range []string{"endpoint: 0.0.0.0:4318", "endpoint: jaeger:4317", "exporters: [otlp_grpc/jaeger]"} {
		if !strings.Contains(collector, want) {
			t.Errorf("otel-collector.yaml has no %s", want)
		}
	}
	for _, service := range []string{"otel-collector:", "jaeger:", "prometheus:", "grafana:"} {
		if !strings.Contains(compose, "\n  "+service+"\n") {
			t.Errorf("compose.yaml has no %s service", strings.TrimSuffix(service, ":"))
		}
	}
	if regexp.MustCompile(`image: \S+:latest|image: [^:\s]+\n`).MatchString(compose) {
		t.Error("compose.yaml uses an image without a pinned version")
	}
}

func TestGrafanaProvisioning(t *testing.T) {
	compose := read(t, "../compose.yaml")
	if !strings.Contains(read(t, "grafana/provisioning/datasources/datasources.yaml"), "uid: prometheus") {
		t.Error("the Prometheus data source has no uid prometheus, which the dashboard names")
	}
	path := regexp.MustCompile(`path: (\S+)`).FindStringSubmatch(read(t, "grafana/provisioning/dashboards/dashboards.yaml"))
	if path == nil || !strings.Contains(compose, "./observability/grafana/dashboards:"+path[1]+":ro") {
		t.Errorf("compose.yaml does not mount the dashboards where the provider reads them: %v", path)
	}
}

// TestLogsAreStructured checks that no program logs through the log
// package any more, and that every program sets up the JSON logger.
func TestLogsAreStructured(t *testing.T) {
	for path, src := range goSources(t) {
		if strings.Contains(src, "\n\t\"log\"\n") {
			t.Errorf("%s imports log; use log/slog", path)
		}
		if filepath.Base(path) == "main.go" && !strings.Contains(src, "slog.SetDefault(obs.NewLogger(os.Stdout, ") {
			t.Errorf("%s does not set the JSON logger", path)
		}
	}
}
