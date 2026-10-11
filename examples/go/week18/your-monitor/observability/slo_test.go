package observability

// These tests keep the SLO files of Week 18 in step with each other and
// with the Go code: every metric a rule names exists, every alert has a
// route and a runbook section, Compose and Kubernetes load the same files
// and images, and every postmortem keeps the sections of the template.
// promtool and amtool check the files themselves: make check-slo.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func sloRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type rule struct {
	Record      string
	Alert       string
	Expr        string
	Labels      map[string]string
	Annotations map[string]string
}

func sloRules(t *testing.T) []rule {
	t.Helper()
	var file struct {
		Groups []struct {
			Name  string
			Rules []rule
		}
	}
	if err := yaml.Unmarshal([]byte(sloRead(t, "rules/slo.rules.yml")), &file); err != nil {
		t.Fatal(err)
	}
	var rules []rule
	for _, g := range file.Groups {
		rules = append(rules, g.Rules...)
	}
	return rules
}

// registered returns every metric name the Go code of the repository
// registers, with the series a histogram adds.
func registered(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	opts := regexp.MustCompile(`prometheus\.New(Counter|Gauge|Histogram)(?:Vec)?\(prometheus\.\w+Opts\{\s*Name:\s*"(\w+)"`)
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		for _, m := range opts.FindAllStringSubmatch(sloRead(t, path), -1) {
			names[m[2]] = true
			if m[1] == "Histogram" {
				for _, s := range []string{"_bucket", "_count", "_sum"} {
					names[m[2]+s] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

// region: rules-test

// TestRulesNameRealMetrics fails when a rule names a metric that no
// program registers: a typo there gives an SLI with no data, which never
// alerts.
func TestRulesNameRealMetrics(t *testing.T) {
	names := registered(t)
	recorded := map[string]bool{}
	for _, r := range sloRules(t) {
		recorded[r.Record] = true
	}
	word := regexp.MustCompile(`[a-z_][a-z0-9_:]*`)
	for _, r := range sloRules(t) {
		for _, w := range word.FindAllString(r.Expr, -1) {
			switch {
			case strings.HasPrefix(w, "monitor_"), strings.HasPrefix(w, "http_"):
				if !names[w] {
					t.Errorf("rule %s%s names %s, which the Go code does not register", r.Record, r.Alert, w)
				}
			case strings.HasPrefix(w, "slo:"):
				if !recorded[w] {
					t.Errorf("rule %s%s reads %s, which no rule records", r.Record, r.Alert, w)
				}
			}
		}
	}
}

// TestEveryAlertHasARouteAndARunbook checks the path from an alert to the
// person who acts on it: a severity that Alertmanager routes, and a
// runbook section under the name of the alert.
func TestEveryAlertHasARouteAndARunbook(t *testing.T) {
	routes := sloRead(t, "alertmanager.yml")
	runbook := sloRead(t, "../docs/runbook.md")
	alerts := 0
	for _, r := range sloRules(t) {
		if r.Alert == "" {
			continue
		}
		alerts++
		sev := r.Labels["severity"]
		if !slices.Contains([]string{"page", "ticket", "heartbeat"}, sev) {
			t.Errorf("%s: severity %q, want page, ticket or heartbeat", r.Alert, sev)
		}
		if sev != "heartbeat" && !strings.Contains(routes, `matchers: ['severity = "`+sev+`"']`) {
			t.Errorf("%s: alertmanager.yml has no route for severity %s", r.Alert, sev)
		}
		if sev != "heartbeat" && r.Labels["slo"] == "" && !strings.Contains(r.Expr, "slo:") {
			t.Errorf("%s: no slo label, which the inhibit rule and group_by need", r.Alert)
		}
		if want := "/docs/runbook.md#" + strings.ToLower(r.Alert); !strings.HasSuffix(r.Annotations["runbook_url"], want) {
			t.Errorf("%s: runbook_url %q does not end in %s", r.Alert, r.Annotations["runbook_url"], want)
		}
		if !strings.Contains(runbook, "\n## "+r.Alert+"\n") {
			t.Errorf("%s: docs/runbook.md has no section ## %s", r.Alert, r.Alert)
		}
	}
	if !strings.Contains(routes, `matchers: ['alertname = "Watchdog"']`) {
		t.Error("alertmanager.yml does not route the Watchdog to the heartbeat")
	}
	if alerts < 5 {
		t.Errorf("found %d alerts, want at least 5", alerts)
	}
}

// endregion: rules-test

// TestTheBudgetsMatchTheSLODocument checks the targets in docs/slo.md
// against the budgets the rules use.
func TestTheBudgetsMatchTheSLODocument(t *testing.T) {
	doc := sloRead(t, "../docs/slo.md")
	budget := map[string]string{}
	for _, r := range sloRules(t) {
		if r.Record == "slo:error_budget:ratio" {
			budget[r.Labels["slo"]] = r.Expr
		}
	}
	for slo, c := range map[string]struct{ expr, target string }{
		"availability": {"vector(0.005)", "**Target:** 99.5%."},
		"freshness":    {"vector(0.01)", "**Target:** 99%."},
	} {
		if budget[slo] != c.expr {
			t.Errorf("budget of %s is %q, want %s", slo, budget[slo], c.expr)
		}
		if !strings.Contains(doc, c.target) {
			t.Errorf("docs/slo.md does not state %s for %s", c.target, slo)
		}
	}
}

// TestComposeAndKubernetesLoadTheSameFiles checks that both stacks load
// one rule file and one Alertmanager file, with the same images.
func TestComposeAndKubernetesLoadTheSameFiles(t *testing.T) {
	compose := sloRead(t, "../compose.yaml")
	kustomization := sloRead(t, "kustomization.yaml")
	k8s := sloRead(t, "k8s/prometheus.yaml") + sloRead(t, "k8s/alertmanager.yaml") + sloRead(t, "k8s/blackbox.yaml")
	for _, want := range []string{"./observability/rules:/etc/prometheus/rules:ro", "./observability/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml does not mount %s", want)
		}
	}
	for _, want := range []string{"- rules/slo.rules.yml", "- alertmanager.yml", "- prometheus.yml=k8s/prometheus.yml"} {
		if !strings.Contains(kustomization, want) {
			t.Errorf("kustomization.yaml has no %s", want)
		}
	}
	if !strings.Contains(sloRead(t, "prometheus.yml"), "- /etc/prometheus/rules/slo.rules.yml") ||
		!strings.Contains(sloRead(t, "k8s/prometheus.yml"), "- /etc/prometheus/config/slo.rules.yml") {
		t.Error("a Prometheus configuration does not load slo.rules.yml from where its stack mounts it")
	}
	images := regexp.MustCompile(`image: (prom/\S+)`)
	var inCompose, inK8s []string
	for _, m := range images.FindAllStringSubmatch(compose, -1) {
		inCompose = append(inCompose, m[1])
	}
	for _, m := range images.FindAllStringSubmatch(k8s, -1) {
		inK8s = append(inK8s, m[1])
		if !slices.Contains(inCompose, m[1]) {
			t.Errorf("Kubernetes runs %s, Compose does not", m[1])
		}
	}
	if len(inK8s) != 3 {
		t.Errorf("Kubernetes runs %v, want prometheus, alertmanager and blackbox-exporter", inK8s)
	}
	for _, secret := range []string{"telegram-bot-token", "telegram-chat-id", "heartbeat-url"} {
		if !strings.Contains(sloRead(t, "alertmanager.yml"), "/etc/alertmanager/secrets/"+secret) {
			t.Errorf("alertmanager.yml does not read %s from the mounted secrets", secret)
		}
	}
	// Each stack pings its own heartbeat check: a laptop running Compose
	// must not keep the check of a dead VPS green.
	for _, want := range []string{
		"./observability/secrets/telegram-bot-token:/etc/alertmanager/secrets/telegram-bot-token:ro",
		"./observability/secrets/telegram-chat-id:/etc/alertmanager/secrets/telegram-chat-id:ro",
		"./observability/secrets/heartbeat-url-laptop:/etc/alertmanager/secrets/heartbeat-url:ro",
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml does not mount %s", want)
		}
	}
	if strings.Contains(compose, "heartbeat-url-vps") {
		t.Error("compose.yaml mounts the heartbeat URL of the VPS")
	}
	// On the VPS, the probe goes through the public URL as well as the
	// web Service: only the public one passes DNS, the certificate and
	// Traefik.
	probe := sloRead(t, "k8s/prometheus.yml")
	for _, want := range []string{"- https://status.example.com/api/monitors", "- http://web.your-monitor.svc:8080/api/monitors"} {
		if !strings.Contains(probe, want) {
			t.Errorf("k8s/prometheus.yml does not probe %s", strings.TrimPrefix(want, "- "))
		}
	}
}

// region: postmortem-test

// TestPostmortemsKeepTheTemplate checks that the template has the
// sections of a blameless postmortem, and that every postmortem in the
// folder keeps them, in the same order.
func TestPostmortemsKeepTheTemplate(t *testing.T) {
	heading := regexp.MustCompile(`(?m)^## (.+)$`)
	sections := func(path string) []string {
		var s []string
		for _, m := range heading.FindAllStringSubmatch(sloRead(t, path), -1) {
			s = append(s, m[1])
		}
		return s
	}
	want := []string{"Summary", "Impact", "Timeline", "Detection", "Causes",
		"What Went Well", "What Went Badly", "Where You Got Lucky", "Actions"}
	if got := sections("../docs/postmortems/TEMPLATE.md"); !slices.Equal(got, want) {
		t.Fatalf("TEMPLATE.md sections %v, want %v", got, want)
	}
	files, err := filepath.Glob("../docs/postmortems/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if got := sections(f); !slices.Equal(got, want) {
			t.Errorf("%s: sections %v, want those of TEMPLATE.md", f, got)
		}
	}
}

// endregion: postmortem-test
