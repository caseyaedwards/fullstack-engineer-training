// Package deploy holds the tests of the Week 17 files under deploy/.
// No cluster runs here, so these tests read the files and check that
// their parts agree with each other and with the api: every HPA scales a
// Deployment that has a CPU request and no replicas field, the Ingress
// names Middlewares that exist, the api settings parse the way the api
// parses them, each CronJob runs a script that the ConfigMap holds, and
// the CI Role of Week 16 can apply every kind the prod overlay holds.
package deploy

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"fsp/examples/week17/your-monitor/internal/ratelimit"
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
// count, and the two singletons never run twice.
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
	single := load(t, filepath.Join(prod, "singletons.yaml"))[0]
	if get(single, "spec", "strategy", "type") != "Recreate" {
		t.Errorf("singletons.yaml: strategy %v, want Recreate", get(single, "spec", "strategy"))
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
// preStop pause fits inside the grace period with time to spare for the
// 5-second shutdown of serve.
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
	pause := get(d, "spec", "template", "spec", "containers", 0, "lifecycle", "preStop", "sleep", "seconds").(int)
	grace := get(d, "spec", "template", "spec", "terminationGracePeriodSeconds").(int)
	if pause+5 >= grace {
		t.Errorf("preStop %d s plus 5 s of shutdown must stay under the grace period of %d s", pause, grace)
	}
}

// region: cronjob-test

// TestCronJobsRunTheirScripts: each CronJob runs a script from the
// backup-scripts ConfigMap, and the script is one the generator holds.
func TestCronJobsRunTheirScripts(t *testing.T) {
	kust := load(t, filepath.Join(prod, "kustomization.yaml"))[0]
	files := map[string]bool{}
	for _, g := range get(kust, "configMapGenerator").([]any) {
		if get(g, "name") != "backup-scripts" {
			continue
		}
		for _, f := range get(g, "files").([]any) {
			if _, err := os.Stat(filepath.Join(prod, f.(string))); err != nil {
				t.Error(err)
			}
			files[filepath.Base(f.(string))] = true
		}
	}
	// The CronJobs reach Postgres with the user, database and password the
	// programs of Week 15 use.
	apiEnv := envFrom(get(find(t, load(t, filepath.Join(base, "api.yaml")), "Deployment", "api"),
		"spec", "template", "spec", "containers", 0))
	for _, name := range []string{"pg-backup", "restore-drill"} {
		cj := find(t, load(t, filepath.Join(prod, "backup.yaml")), "CronJob", name)
		if get(cj, "spec", "concurrencyPolicy") != "Forbid" {
			t.Errorf("CronJob %s: concurrencyPolicy %v, want Forbid", name, get(cj, "spec", "concurrencyPolicy"))
		}
		pod := get(cj, "spec", "jobTemplate", "spec", "template", "spec")
		cmd := get(pod, "containers", 0, "command").([]any)
		script := strings.TrimPrefix(cmd[len(cmd)-1].(string), "/scripts/")
		if !files[script] {
			t.Errorf("CronJob %s runs %s, which backup-scripts does not hold", name, script)
		}
		var mounted bool
		for _, v := range get(pod, "volumes").([]any) {
			if get(v, "configMap", "name") == "backup-scripts" {
				mounted = true
			}
		}
		if !mounted {
			t.Errorf("CronJob %s does not mount backup-scripts", name)
		}
		if got := envFrom(get(pod, "containers", 0)); got != apiEnv {
			t.Errorf("CronJob %s reads %s, the api reads %s", name, got, apiEnv)
		}
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

// region: rbac-test

// resources maps each kind the prod overlay may hold to the API group and
// resource name that a Role rule names.
var resources = map[string][2]string{
	"ConfigMap":               {"", "configmaps"},
	"Service":                 {"", "services"},
	"PersistentVolumeClaim":   {"", "persistentvolumeclaims"},
	"Deployment":              {"apps", "deployments"},
	"StatefulSet":             {"apps", "statefulsets"},
	"Ingress":                 {"networking.k8s.io", "ingresses"},
	"Middleware":              {"traefik.io", "middlewares"},
	"HorizontalPodAutoscaler": {"autoscaling", "horizontalpodautoscalers"},
	"CronJob":                 {"batch", "cronjobs"},
}

// kinds returns the kind of every object that the Kustomization in dir
// holds, through its resources, its bases and its generators.
func kinds(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	k := load(t, filepath.Join(dir, "kustomization.yaml"))[0]
	res, _ := get(k, "resources").([]any)
	for _, r := range res {
		p := filepath.Join(dir, r.(string))
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			for kind := range kinds(t, p) {
				out[kind] = true
			}
			continue
		}
		for _, d := range load(t, p) {
			out[d["kind"].(string)] = true
		}
	}
	if k["configMapGenerator"] != nil {
		out["ConfigMap"] = true
	}
	if k["secretGenerator"] != nil {
		out["Secret"] = true
	}
	return out
}

// TestDeployerCanApplyTheProdOverlay: deploy.yml applies the prod overlay
// as the deployer ServiceAccount of Week 16, whose Role names each kind it
// may touch. kubectl apply reads each object, then creates or patches it;
// kubectl rollout status lists and watches the Deployments. A kind that
// the Role does not grant fails the deploy halfway, with Forbidden.
func TestDeployerCanApplyTheProdOverlay(t *testing.T) {
	role := find(t, load(t, "k8s/cluster/ci-deployer.yaml"), "Role", "deployer")
	allows := func(group, resource, verb string) bool {
		for _, r := range get(role, "rules").([]any) {
			has := func(field, want string) bool {
				l, _ := get(r, field).([]any)
				return slices.Contains(l, any(want))
			}
			if has("apiGroups", group) && has("resources", resource) && has("verbs", verb) {
				return true
			}
		}
		return false
	}
	for kind := range kinds(t, prod) {
		gr, ok := resources[kind]
		if !ok {
			t.Errorf("the prod overlay holds a %s: add it to the resources map and to the deployer Role", kind)
			continue
		}
		verbs := []string{"get", "create", "patch"}
		if kind == "Deployment" {
			verbs = append(verbs, "list", "watch")
		}
		for _, verb := range verbs {
			if !allows(gr[0], gr[1], verb) {
				t.Errorf("the deployer Role cannot %s %s, which the prod overlay holds", verb, gr[1])
			}
		}
	}
}

// endregion: rbac-test

// region: restricted-test

// TestNewPodsPassRestricted: the namespace enforces the restricted Pod
// Security Standard, and the API server refuses a pod that breaks it. The
// Job pods of both CronJobs and the load-drill pod must not run as root,
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
