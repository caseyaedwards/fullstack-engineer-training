// Package week18 runs the tools of Prometheus and Alertmanager against the
// SLO files of Week 18: promtool checks the rules and both Prometheus
// configurations and runs the rule unit tests; amtool checks the
// Alertmanager configuration and its routes. The site CI installs both
// tools. Elsewhere a missing tool skips its test, and PROMTOOL or AMTOOL
// can name the binary.
package week18

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const obs = "your-monitor/observability"

// tool returns the path of a tool, from the environment variable env or
// from PATH. In CI a missing tool fails the test; elsewhere it skips.
func tool(t *testing.T, name, env string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatalf("%s is not on PATH; the site workflow installs it", name)
		}
		t.Skipf("%s is not on PATH; set %s to run this test", name, env)
	}
	return p
}

func run(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command(bin, args...).CombinedOutput()
	return string(out), err
}

func copyFile(t *testing.T, from, to string, replace ...string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.NewReplacer(replace...).Replace(string(b))
	if err := os.WriteFile(to, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPromtoolChecksTheRules(t *testing.T) {
	promtool := tool(t, "promtool", "PROMTOOL")
	out, err := run(t, promtool, "check", "rules", obs+"/rules/slo.rules.yml")
	if err != nil || !strings.Contains(out, "SUCCESS: 27 rules found") {
		t.Fatalf("promtool check rules: %v\n%s", err, out)
	}
}

// region: promtool-test

func TestPromtoolRunsTheRuleUnitTests(t *testing.T) {
	promtool := tool(t, "promtool", "PROMTOOL")
	out, err := run(t, promtool, "test", "rules", obs+"/rules/slo.test.yml")
	if err != nil || !strings.Contains(out, "SUCCESS") {
		t.Fatalf("promtool test rules: %v\n%s", err, out)
	}
}

// TestTheUnitTestsCatchBrokenRules breaks the rules on purpose, one way at
// a time, and expects the unit tests to fail on each.
func TestTheUnitTestsCatchBrokenRules(t *testing.T) {
	promtool := tool(t, "promtool", "PROMTOOL")
	for _, m := range []struct{ name, from, to string }{
		{"no long window", "slo:burn_rate:1h > 14.4 and slo:burn_rate:5m > 14.4", "slo:burn_rate:5m > 14.4"},
		{"no short window", "slo:burn_rate:1h > 14.4 and slo:burn_rate:5m > 14.4", "slo:burn_rate:1h > 14.4"},
		{"missing data counts as on time", "\n            or on() vector(0)\n", "\n"},
		{"first start counts as late", "and on() (count(up offset 2m) > 0)", ""},
		{"queue age only", "*\n              (sum(increase(monitor_checks_total[2m])) > bool 0)", ""},
		{"budget of 5%", "vector(0.005)", "vector(0.05)"},
		{"no absent probe", "or absent(probe_success{job=\"status-page-probe\"})", ""},
		{"slow burn needs 100 times", "slo:burn_rate:6h > 6", "slo:burn_rate:6h > 600"},
		{"slow burn never fires", "slo:burn_rate:6h > 6 and slo:burn_rate:30m > 6", "vector(0) > 1"},
		{"ticket without the 6-hour window", "slo:burn_rate:3d > 1 and slo:burn_rate:6h > 1", "slo:burn_rate:3d > 1"},
		{"ticket without the 3-day window", "slo:burn_rate:3d > 1 and slo:burn_rate:6h > 1", "slo:burn_rate:6h > 1"},
		{"queue age of 500 seconds", "<= bool 50", "<= bool 500"},
		{"30-minute ratio over 5 minutes", "[30m])) or vector(0))\n          /\n          sum(rate(http_requests_total{method=\"GET\", route=~\"/api/.+\"}[30m]))", "[5m])) or vector(0))\n          /\n          sum(rate(http_requests_total{method=\"GET\", route=~\"/api/.+\"}[30m]))"},
		{"budget left without the budget", "1 - slo:sli_error:ratio_rate30d / on(slo) slo:error_budget:ratio", "1 - slo:sli_error:ratio_rate30d"},
		{"no certificate ticket", "- time() < 14 * 86400", "- time() < 7 * 86400"},
	} {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			rules, err := os.ReadFile(obs + "/rules/slo.rules.yml")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(rules), m.from) {
				t.Fatalf("slo.rules.yml has no %q to break", m.from)
			}
			broken := strings.Replace(string(rules), m.from, m.to, 1)
			if err := os.WriteFile(filepath.Join(dir, "slo.rules.yml"), []byte(broken), 0o644); err != nil {
				t.Fatal(err)
			}
			copyFile(t, obs+"/rules/slo.test.yml", filepath.Join(dir, "slo.test.yml"))
			if out, err := run(t, promtool, "test", "rules", filepath.Join(dir, "slo.test.yml")); err == nil {
				t.Errorf("the unit tests pass with the rules broken\n%s", out)
			}
		})
	}
}

// endregion: promtool-test

// TestPromtoolChecksBothConfigs checks the Compose and the Kubernetes
// configuration of Prometheus, with the rule path each one mounts pointed
// at a copy of the rule file.
func TestPromtoolChecksBothConfigs(t *testing.T) {
	promtool := tool(t, "promtool", "PROMTOOL")
	dir := t.TempDir()
	rules := filepath.Join(dir, "slo.rules.yml")
	copyFile(t, obs+"/rules/slo.rules.yml", rules)
	for _, c := range []struct{ file, mounted string }{
		{obs + "/prometheus.yml", "/etc/prometheus/rules/slo.rules.yml"},
		{obs + "/k8s/prometheus.yml", "/etc/prometheus/config/slo.rules.yml"},
	} {
		to := filepath.Join(dir, strings.ReplaceAll(c.file, "/", "_"))
		copyFile(t, c.file, to, c.mounted, rules)
		out, err := run(t, promtool, "check", "config", to)
		if err != nil || !strings.Contains(out, "is valid prometheus config file syntax") {
			t.Errorf("promtool check config %s: %v\n%s", c.file, err, out)
		}
	}
}

func TestAmtoolChecksTheConfig(t *testing.T) {
	amtool := tool(t, "amtool", "AMTOOL")
	out, err := run(t, amtool, "check-config", obs+"/alertmanager.yml")
	if err != nil || !strings.Contains(out, "SUCCESS") {
		t.Fatalf("amtool check-config: %v\n%s", err, out)
	}
}

// TestAmtoolRoutesEachAlert asks amtool which receiver gets an alert with
// the labels each rule sets.
func TestAmtoolRoutesEachAlert(t *testing.T) {
	amtool := tool(t, "amtool", "AMTOOL")
	for labels, want := range map[string]string{
		"alertname=ErrorBudgetFastBurn severity=page slo=availability":      "page",
		"alertname=ErrorBudgetSlowBurn severity=page slo=freshness":         "page",
		"alertname=StatusPageDown severity=page slo=availability":           "page",
		"alertname=ErrorBudgetTicket severity=ticket slo=freshness":         "ticket",
		"alertname=CertificateExpiresSoon severity=ticket slo=availability": "ticket",
		"alertname=Watchdog severity=heartbeat":                             "heartbeat",
		"alertname=SomethingNew":                                            "ticket",
	} {
		args := append([]string{"config", "routes", "test", "--config.file=" + obs + "/alertmanager.yml"}, strings.Fields(labels)...)
		out, err := run(t, amtool, args...)
		if err != nil || strings.TrimSpace(out) != want {
			t.Errorf("%s: got %q (%v), want %s", labels, strings.TrimSpace(out), err, want)
		}
	}
}
