// Package deploy holds the tests of the Week 17 files under deploy/.
// No cluster runs here, so these tests read the files and check that
// their parts agree with each other and with the api: every HPA scales a
// Deployment that has a CPU request and no replicas field, the Ingress
// names Middlewares that exist, the api settings parse the way the api
// parses them, the backup CronJob runs the backup image, and every new
// pod passes the restricted Pod Security Standard. The CI Role check
// lives with the Week 16 tests in k8s/manifests_test.go.
package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"fsp/examples/week18/your-monitor/internal/ratelimit"
)

const (
	base = "k8s/base"
	prod = "k8s/overlays/prod"
)

// doc is one YAML document, read into maps and slices.
type doc map[string]any

// load reads every document of a YAML file.
func load(t *testing.T, path string) []doc {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var docs []doc
	dec := yaml.NewDecoder(bytes.NewReader(b))
	for {
		var d doc
		err := dec.Decode(&d)
		if errors.Is(err, io.EOF) {
			return docs
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if d != nil {
			docs = append(docs, d)
		}
	}
}

// get follows keys and list indexes from d: get(d, "spec", "containers", 0).
func get(d any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := d.(doc)
			if !ok {
				if mm, ok2 := d.(map[string]any); ok2 {
					m, ok = doc(mm), true
				}
			}
			if !ok {
				return nil
			}
			d = m[k]
		case int:
			l, ok := d.([]any)
			if !ok || k >= len(l) {
				return nil
			}
			d = l[k]
		}
	}
	return d
}

// find returns the document of kind and name among docs.
func find(t *testing.T, docs []doc, kind, name string) doc {
	t.Helper()
	for _, d := range docs {
		if d["kind"] == kind && get(d, "metadata", "name") == name {
			return d
		}
	}
	t.Fatalf("no %s %s", kind, name)
	return nil
}

// region: hpa-test

// TestHPAsScaleDeploymentsWithoutReplicas: an HPA computes CPU use as a
// share of the CPU request, and a replicas field in the manifest resets
// the count the HPA chose on every kubectl apply.
func TestHPAsScaleDeploymentsWithoutReplicas(t *testing.T) {
	kust := load(t, filepath.Join(prod, "kustomization.yaml"))[0]
	var removed *regexp.Regexp
	for _, p := range get(kust, "patches").([]any) {
		if patch, _ := get(p, "patch").(string); strings.Contains(patch, "path: /spec/replicas") && strings.Contains(patch, "op: remove") {
			removed = regexp.MustCompile("^(" + get(p, "target", "name").(string) + ")$")
		}
	}
	if removed == nil {
		t.Fatal("kustomization.yaml has no patch that removes /spec/replicas")
	}

	hpas := load(t, filepath.Join(prod, "hpa.yaml"))
	var scaled []string
	for _, h := range hpas {
		if h["apiVersion"] != "autoscaling/v2" {
			t.Errorf("HPA %v: apiVersion %v, want autoscaling/v2", get(h, "metadata", "name"), h["apiVersion"])
		}
		name := get(h, "spec", "scaleTargetRef", "name").(string)
		scaled = append(scaled, name)
		dep := find(t, load(t, filepath.Join(base, name+".yaml")), "Deployment", name)
		if get(dep, "spec", "template", "spec", "containers", 0, "resources", "requests", "cpu") == nil {
			t.Errorf("Deployment %s has no CPU request: its HPA has nothing to divide by", name)
		}
		if !removed.MatchString(name) {
			t.Errorf("Deployment %s keeps its replicas field, which fights its HPA", name)
		}
		minR, maxR := get(h, "spec", "minReplicas").(int), get(h, "spec", "maxReplicas").(int)
		if minR < 1 || maxR <= minR {
			t.Errorf("HPA %s: min %d, max %d", name, minR, maxR)
		}
	}
	for _, name := range []string{"api", "worker", "scheduler", "alerter", "web"} {
		if removed.MatchString(name) && !strings.Contains(strings.Join(scaled, " "), name) {
			t.Errorf("Deployment %s loses its replicas field but no HPA scales it", name)
		}
	}
}

// endregion: hpa-test

// TestRolloutSettings: the rolling Deployments never drop below their
// count, and the two singletons keep the Recreate strategy of Week 15,
// which keeps them from running twice.
func TestRolloutSettings(t *testing.T) {
	roll := load(t, filepath.Join(prod, "rollout.yaml"))[0]
	if get(roll, "spec", "strategy", "rollingUpdate", "maxUnavailable") != 0 ||
		get(roll, "spec", "strategy", "rollingUpdate", "maxSurge") != 1 {
		t.Errorf("rollout.yaml: strategy %v, want maxSurge 1 and maxUnavailable 0", get(roll, "spec", "strategy"))
	}
	ready, deadline := get(roll, "spec", "minReadySeconds").(int), get(roll, "spec", "progressDeadlineSeconds").(int)
	if deadline <= ready {
		t.Errorf("progressDeadlineSeconds %d must be greater than minReadySeconds %d", deadline, ready)
	}
	for _, name := range []string{"scheduler", "alerter"} {
		d := find(t, load(t, filepath.Join(base, name+".yaml")), "Deployment", name)
		if get(d, "spec", "strategy", "type") != "Recreate" {
			t.Errorf("base/%s.yaml: strategy %v, want Recreate", name, get(d, "spec", "strategy"))
		}
	}
}

// region: middleware-test

// TestIngressNamesTheMiddleware: Traefik finds a Middleware from an
// Ingress annotation that reads <namespace>-<name>@kubernetescrd. A typo
// there makes Traefik drop the whole route, and the site answers 404.
// The patch replaces the annotation of Week 16, so it must keep the
// redirect that annotation named.
func TestIngressNamesTheMiddleware(t *testing.T) {
	ns := load(t, filepath.Join(prod, "kustomization.yaml"))[0]["namespace"].(string)
	exists := map[string]bool{}
	for _, f := range []string{"ratelimit.yaml", "redirect-https.yaml"} {
		for _, mw := range load(t, filepath.Join(prod, f)) {
			if mw["apiVersion"] != "traefik.io/v1alpha1" || mw["kind"] != "Middleware" {
				t.Errorf("%s holds %v %v, want traefik.io/v1alpha1 Middleware", f, mw["apiVersion"], mw["kind"])
			}
			exists[ns+"-"+get(mw, "metadata", "name").(string)+"@kubernetescrd"] = true
		}
	}
	const key = "traefik.ingress.kubernetes.io/router.middlewares"
	week16 := load(t, filepath.Join(prod, "ingress.yaml"))[0]
	patch := load(t, filepath.Join(prod, "ingress-middleware.yaml"))[0]
	if get(patch, "metadata", "name") != get(week16, "metadata", "name") {
		t.Errorf("the patch names Ingress %v, but the Ingress is %v", get(patch, "metadata", "name"), get(week16, "metadata", "name"))
	}
	got := strings.Split(get(patch, "metadata", "annotations", key).(string), ",")
	for _, ref := range got {
		if !exists[ref] {
			t.Errorf("router.middlewares names %s, which no Middleware of the overlay matches", ref)
		}
	}
	for _, ref := range strings.Split(get(week16, "metadata", "annotations", key).(string), ",") {
		if !slices.Contains(got, ref) {
			t.Errorf("router.middlewares %v drops %s, which the Ingress of Week 16 names", got, ref)
		}
	}
	if !slices.Contains(got, ns+"-ratelimit@kubernetescrd") {
		t.Errorf("router.middlewares %v does not name the rate limit", got)
	}
	mw := load(t, filepath.Join(prod, "ratelimit.yaml"))[0]
	if v6 := get(mw, "spec", "rateLimit", "sourceCriterion", "ipStrategy", "ipv6Subnet"); v6 != 64 {
		t.Errorf("ipv6Subnet = %v, want 64, as the api keys IPv6 clients", v6)
	}
}

// endregion: middleware-test

// TestAPIPatch: the settings parse the way cmd/api parses them, and the
// preStop pause that prestop.yaml adds to api and web fits inside the
// grace period, with time to spare for the 5-second shutdown of serve.
func TestAPIPatch(t *testing.T) {
	d := load(t, filepath.Join(prod, "api.yaml"))[0]
	env := map[string]string{}
	for _, e := range get(d, "spec", "template", "spec", "containers", 0, "env").([]any) {
		env[get(e, "name").(string)] = get(e, "value").(string)
	}
	nets, err := ratelimit.ParsePrefixes(env["TRUSTED_PROXIES"])
	if err != nil || len(nets) == 0 {
		t.Errorf("TRUSTED_PROXIES %q: %v", env["TRUSTED_PROXIES"], err)
	}
	limit, err1 := strconv.ParseFloat(env["RATE_LIMIT"], 64)
	burst, err2 := strconv.Atoi(env["RATE_BURST"])
	if err1 != nil || err2 != nil || limit <= 0 || burst < 1 {
		t.Errorf("RATE_LIMIT %q, RATE_BURST %q", env["RATE_LIMIT"], env["RATE_BURST"])
	}
	// prestop.yaml is a JSON patch: a list of operations, not an object.
	b, err := os.ReadFile(filepath.Join(prod, "prestop.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var ops []map[string]any
	if err := yaml.Unmarshal(b, &ops); err != nil || len(ops) != 1 {
		t.Fatalf("prestop.yaml: want one JSON patch operation, got %d (%v)", len(ops), err)
	}
	pause, _ := get(doc(ops[0]), "value", "preStop", "sleep", "seconds").(int)
	grace, _ := get(d, "spec", "template", "spec", "terminationGracePeriodSeconds").(int)
	if pause < 1 || pause+5 >= grace {
		t.Errorf("preStop %d s plus 5 s of shutdown must stay under the grace period of %d s", pause, grace)
	}
	kust := load(t, filepath.Join(prod, "kustomization.yaml"))[0]
	var targets string
	for _, p := range get(kust, "patches").([]any) {
		if get(p, "path") == "prestop.yaml" {
			targets, _ = get(p, "target", "name").(string)
		}
	}
	if targets != "api|web" {
		t.Errorf("prestop.yaml patches %q, want api|web: Traefik sends to web, kube-proxy to api", targets)
	}
}

// region: cronjob-test

// TestBackupCronJob: the CronJob runs backup.sh from the backup image,
// which the Dockerfile builds and CI pushes and deploy.yml names like the
// other five. It reaches Postgres with the settings the programs of Week
// 15 use, and reads the age public key, the remote and the upload key.
func TestBackupCronJob(t *testing.T) {
	cj := find(t, load(t, filepath.Join(prod, "backup.yaml")), "CronJob", "pg-backup")
	if get(cj, "spec", "concurrencyPolicy") != "Forbid" {
		t.Errorf("concurrencyPolicy %v, want Forbid", get(cj, "spec", "concurrencyPolicy"))
	}
	pod := get(cj, "spec", "jobTemplate", "spec", "template", "spec")
	c := get(pod, "containers", 0)
	if get(c, "image") != "your-monitor-backup" || fmt.Sprint(get(c, "command")) != "[backup.sh]" {
		t.Errorf("container runs %v from %v, want backup.sh from your-monitor-backup", get(c, "command"), get(c, "image"))
	}
	if fmt.Sprint(get(pod, "imagePullSecrets")) != "[map[name:ghcr-pull]]" {
		t.Errorf("imagePullSecrets %v, want ghcr-pull: the backup image lives on GHCR", get(pod, "imagePullSecrets"))
	}

	apiEnv := envFrom(get(find(t, load(t, filepath.Join(base, "api.yaml")), "Deployment", "api"),
		"spec", "template", "spec", "containers", 0))
	if got, want := envFrom(c), apiEnv+", configMap backup-config, secret offsite"; got != want {
		t.Errorf("envFrom %s, want %s", got, want)
	}
	kust := load(t, filepath.Join(prod, "kustomization.yaml"))[0]
	var literals string
	for _, g := range get(kust, "configMapGenerator").([]any) {
		if get(g, "name") == "backup-config" {
			literals = fmt.Sprint(get(g, "literals"))
		}
	}
	for _, key := range []string{"AGE_RECIPIENT=age1", "BACKUP_REMOTE="} {
		if !strings.Contains(literals, key) {
			t.Errorf("backup-config %s has no %s", literals, key)
		}
	}

	// The image: the scripts it copies exist, CI builds it and installs
	// the tools the backup tests need, deploy.yml points
	// your-monitor-backup at GHCR.
	for file, wants := range map[string][]string{
		"../Dockerfile":                   {"AS backup"},
		"../.github/workflows/ci.yml":     {"web, backup]", "install -y age rclone"},
		"../.github/workflows/deploy.yml": {"alerter web backup; do"},
	} {
		b, err := os.ReadFile(file)
		for _, want := range wants {
			if err != nil || !strings.Contains(string(b), want) {
				t.Errorf("%s does not contain %q (%v)", file, want, err)
			}
		}
	}
	for _, f := range []string{"backup/backup.sh", "backup/restore-drill.sh"} {
		if _, err := os.Stat(f); err != nil {
			t.Error(err)
		}
	}

	// postgres:17 purges its CA certificates. Without the package, rclone
	// fails x509 verification on every https endpoint, and no backup
	// ever uploads.
	df, _ := os.ReadFile("../Dockerfile")
	_, stage, _ := strings.Cut(string(df), " AS backup\n")
	stage, _, _ = strings.Cut(stage, "\nFROM ")
	if !regexp.MustCompile(`apt-get install [^\n]*\bca-certificates\b`).MatchString(stage) {
		t.Errorf("the backup stage of the Dockerfile does not install ca-certificates:\n%s", stage)
	}
	// backup.sh pings healthchecks.io with curl, which postgres:17 lacks.
	if !regexp.MustCompile(`apt-get install [^\n]*\bcurl\b`).MatchString(stage) {
		t.Errorf("the backup stage of the Dockerfile does not install curl:\n%s", stage)
	}
}

// envFrom lists the ConfigMap and Secret names a container reads all keys
// of, such as "configMap monitor-config, secret postgres".
func envFrom(container any) string {
	var refs []string
	l, _ := get(container, "envFrom").([]any)
	for _, e := range l {
		if n := get(e, "configMapRef", "name"); n != nil {
			refs = append(refs, "configMap "+n.(string))
		}
		if n := get(e, "secretRef", "name"); n != nil {
			refs = append(refs, "secret "+n.(string))
		}
	}
	return strings.Join(refs, ", ")
}

// endregion: cronjob-test

// region: restricted-test

// TestNewPodsPassRestricted: the namespace enforces the restricted Pod
// Security Standard, and the API server refuses a pod that breaks it. The
// Job pod of the backup CronJob and the load-drill pod must not run as root,
// must not gain privileges, must drop every capability, must use the
// default seccomp profile, and may mount only volumes that come from the
// cluster, never from the node.
func TestNewPodsPassRestricted(t *testing.T) {
	pods := map[string]any{}
	for _, cj := range load(t, filepath.Join(prod, "backup.yaml")) {
		if cj["kind"] == "CronJob" {
			pods["CronJob "+get(cj, "metadata", "name").(string)] = get(cj, "spec", "jobTemplate", "spec", "template", "spec")
		}
	}
	pods["Pod load"] = get(load(t, "k8s/load/load.yaml")[0], "spec")
	allowed := []string{"configMap", "emptyDir", "persistentVolumeClaim", "secret", "projected", "downwardAPI", "ephemeral", "csi"}
	for name, pod := range pods {
		if get(pod, "securityContext", "runAsNonRoot") != true {
			t.Errorf("%s: runAsNonRoot is not true", name)
		}
		if u, ok := get(pod, "securityContext", "runAsUser").(int); ok && u == 0 {
			t.Errorf("%s: runAsUser 0", name)
		}
		if get(pod, "securityContext", "seccompProfile", "type") != "RuntimeDefault" {
			t.Errorf("%s: seccompProfile is not RuntimeDefault", name)
		}
		for i, c := range get(pod, "containers").([]any) {
			if get(c, "securityContext", "allowPrivilegeEscalation") != false {
				t.Errorf("%s, container %d: allowPrivilegeEscalation is not false", name, i)
			}
			drop, _ := get(c, "securityContext", "capabilities", "drop").([]any)
			if !slices.Contains(drop, any("ALL")) {
				t.Errorf("%s, container %d: capabilities.drop %v, want ALL", name, i, drop)
			}
		}
		vols, _ := get(pod, "volumes").([]any)
		for _, v := range vols {
			for k := range v.(doc) {
				if k != "name" && !slices.Contains(allowed, k) {
					t.Errorf("%s: volume %v is of type %s, which restricted refuses", name, get(v, "name"), k)
				}
			}
		}
	}
}

// endregion: restricted-test
