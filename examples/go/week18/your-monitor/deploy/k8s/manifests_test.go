// Package k8s holds the tests of the Kubernetes manifests in base/,
// overlays/kind/, overlays/prod/ and cluster/, and of kind-config.yaml.
// It has no other Go code. The tests read the objects the way kubectl
// apply -k sends them, and check what the week pages promise: probes,
// resources, the restricted Pod Security Standard, labels and selectors,
// and the Postgres volume in Week 15; the CI Role, the admission policy,
// the issuers and the Ingress in Week 16.
package k8s

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// object is one Kubernetes object: what every kind has, plus the raw node
// for the fields of its own kind.
type object struct {
	Kind     string `yaml:"kind"`
	Metadata meta   `yaml:"metadata"`
	node     *yaml.Node
}

type meta struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

// workload is a Deployment or a StatefulSet.
type workload struct {
	Metadata meta `yaml:"metadata"`
	Spec     struct {
		ServiceName string `yaml:"serviceName"`
		Selector    struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"selector"`
		Template struct {
			Metadata meta    `yaml:"metadata"`
			Spec     podSpec `yaml:"spec"`
		} `yaml:"template"`
		VolumeClaimTemplates []struct {
			Metadata meta `yaml:"metadata"`
		} `yaml:"volumeClaimTemplates"`
	} `yaml:"spec"`
}

type podSpec struct {
	SecurityContext struct {
		RunAsNonRoot   *bool  `yaml:"runAsNonRoot"`
		RunAsUser      *int64 `yaml:"runAsUser"`
		SeccompProfile struct {
			Type string `yaml:"type"`
		} `yaml:"seccompProfile"`
	} `yaml:"securityContext"`
	Containers []container      `yaml:"containers"`
	Volumes    []map[string]any `yaml:"volumes"`
	Extra      map[string]any   `yaml:",inline"`
}

type container struct {
	Name            string `yaml:"name"`
	Image           string `yaml:"image"`
	ImagePullPolicy string `yaml:"imagePullPolicy"`
	Ports           []struct {
		Name          string `yaml:"name"`
		ContainerPort int    `yaml:"containerPort"`
	} `yaml:"ports"`
	EnvFrom []struct {
		ConfigMapRef *struct{ Name string } `yaml:"configMapRef"`
		SecretRef    *struct{ Name string } `yaml:"secretRef"`
	} `yaml:"envFrom"`
	Env            []envVar `yaml:"env"`
	ReadinessProbe *probe   `yaml:"readinessProbe"`
	LivenessProbe  *probe   `yaml:"livenessProbe"`
	Resources      struct {
		Requests map[string]string `yaml:"requests"`
		Limits   map[string]string `yaml:"limits"`
	} `yaml:"resources"`
	SecurityContext struct {
		AllowPrivilegeEscalation *bool  `yaml:"allowPrivilegeEscalation"`
		ReadOnlyRootFilesystem   *bool  `yaml:"readOnlyRootFilesystem"`
		RunAsUser                *int64 `yaml:"runAsUser"`
		Capabilities             struct {
			Add  []string `yaml:"add"`
			Drop []string `yaml:"drop"`
		} `yaml:"capabilities"`
	} `yaml:"securityContext"`
	VolumeMounts []struct {
		Name      string `yaml:"name"`
		MountPath string `yaml:"mountPath"`
	} `yaml:"volumeMounts"`
}

type envVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type probe struct {
	HTTPGet *struct {
		Path string `yaml:"path"`
		Port string `yaml:"port"`
	} `yaml:"httpGet"`
	TCPSocket *struct {
		Port string `yaml:"port"`
	} `yaml:"tcpSocket"`
	Exec *struct {
		Command []string `yaml:"command"`
	} `yaml:"exec"`
}

type service struct {
	Metadata meta `yaml:"metadata"`
	Spec     struct {
		Type      string            `yaml:"type"`
		ClusterIP string            `yaml:"clusterIP"`
		Selector  map[string]string `yaml:"selector"`
		Ports     []struct {
			Name       string `yaml:"name"`
			Port       int    `yaml:"port"`
			TargetPort string `yaml:"targetPort"`
			NodePort   int    `yaml:"nodePort"`
		} `yaml:"ports"`
	} `yaml:"spec"`
}

// region: render

// render returns the objects of the Kustomize directory dir, such as
// "overlays/prod", as kubectl apply -k sends them. It runs kubectl
// kustomize on a copy of this directory, with a made-up postgres.env for
// the kind overlay, because the real file never enters Git. Without
// kubectl, the test fails in CI. On your laptop, it reads the base files
// as they are instead, and reports rendered as false.
func render(t *testing.T, dir string) (objs []object, rendered bool) {
	t.Helper()
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("kubectl is not on PATH: CI must install it to render the overlay")
		}
		t.Log("kubectl is not on PATH: checking the base files without the overlay")
		files, _ := filepath.Glob("base/*.yaml")
		for _, f := range files {
			if filepath.Base(f) == "kustomization.yaml" {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			objs = append(objs, decode(t, b)...)
		}
		return objs, false
	}

	tmp := t.TempDir()
	if err := os.CopyFS(tmp, os.DirFS(".")); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(tmp, "overlays", "kind", "postgres.env")
	if err := os.WriteFile(env, []byte("POSTGRES_PASSWORD=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(kubectl, "kustomize", filepath.Join(tmp, dir))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("kubectl kustomize %s: %v\n%s", dir, err, stderr.String())
	}
	return decode(t, out), true
}

// endregion: render

// decode splits a YAML stream into its objects.
func decode(t *testing.T, b []byte) []object {
	t.Helper()
	var objs []object
	dec := yaml.NewDecoder(bytes.NewReader(b))
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return objs
		}
		if err != nil {
			t.Fatal(err)
		}
		o := object{node: &n}
		if err := n.Decode(&o); err != nil {
			t.Fatal(err)
		}
		objs = append(objs, o)
	}
}

func as[T any](t *testing.T, o object) T {
	t.Helper()
	var v T
	if err := o.node.Decode(&v); err != nil {
		t.Fatalf("%s %s: %v", o.Kind, o.Metadata.Name, err)
	}
	return v
}

func workloads(t *testing.T, objs []object) map[string]workload {
	ws := map[string]workload{}
	for _, o := range objs {
		if o.Kind == "Deployment" || o.Kind == "StatefulSet" {
			ws[o.Metadata.Name] = as[workload](t, o)
		}
	}
	return ws
}

func services(t *testing.T, objs []object) map[string]service {
	ss := map[string]service{}
	for _, o := range objs {
		if o.Kind == "Service" {
			ss[o.Metadata.Name] = as[service](t, o)
		}
	}
	return ss
}

// subset reports whether every label of sel is in labels.
func subset(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func TestManifests(t *testing.T) {
	objs, rendered := render(t, "overlays/kind")
	ws := workloads(t, objs)
	ss := services(t, objs)

	t.Run("every program is there", func(t *testing.T) {
		want := []string{"alerter", "api", "postgres", "scheduler", "web", "worker"}
		if got := slices.Sorted(maps.Keys(ws)); !slices.Equal(got, want) {
			t.Fatalf("workloads %v, want %v", got, want)
		}
	})

	// region: probes-test
	t.Run("every container has probes, requests and limits", func(t *testing.T) {
		for name, w := range ws {
			for _, c := range w.Spec.Template.Spec.Containers {
				ports := map[string]bool{}
				for _, p := range c.Ports {
					ports[p.Name] = true
				}
				for kind, p := range map[string]*probe{"readiness": c.ReadinessProbe, "liveness": c.LivenessProbe} {
					switch {
					case p == nil:
						t.Errorf("%s: no %s probe", name, kind)
					case name == "postgres":
						// Postgres speaks no HTTP: pg_isready and a TCP connect.
					case p.HTTPGet == nil || p.HTTPGet.Path != "/healthz":
						t.Errorf("%s: the %s probe must GET /healthz", name, kind)
					case !ports[p.HTTPGet.Port]:
						t.Errorf("%s: the %s probe uses port %q, which the container does not name", name, kind, p.HTTPGet.Port)
					}
				}
				for _, r := range []string{"cpu", "memory"} {
					if c.Resources.Requests[r] == "" || c.Resources.Limits[r] == "" {
						t.Errorf("%s: %s needs both a request and a limit", name, r)
					}
				}
			}
		}
	})
	// endregion: probes-test

	// region: security-test
	t.Run("every pod meets the restricted Pod Security Standard", func(t *testing.T) {
		allowed := []string{"configMap", "csi", "downwardAPI", "emptyDir", "ephemeral", "persistentVolumeClaim", "projected", "secret"}
		for name, w := range ws {
			pod := w.Spec.Template.Spec
			if pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot {
				t.Errorf("%s: runAsNonRoot must be true", name)
			}
			if u := pod.SecurityContext.RunAsUser; u != nil && *u == 0 {
				t.Errorf("%s: runAsUser 0 is root", name)
			}
			if pod.SecurityContext.SeccompProfile.Type != "RuntimeDefault" {
				t.Errorf("%s: seccompProfile must be RuntimeDefault", name)
			}
			for _, k := range []string{"hostNetwork", "hostPID", "hostIPC"} {
				if pod.Extra[k] == true {
					t.Errorf("%s: %s shares the node with the pod", name, k)
				}
			}
			for _, v := range pod.Volumes {
				if !slices.ContainsFunc(allowed, func(k string) bool { _, ok := v[k]; return ok }) {
					t.Errorf("%s: volume %v has a type the restricted standard forbids", name, v["name"])
				}
			}
			for _, c := range pod.Containers {
				sc := c.SecurityContext
				if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
					t.Errorf("%s: allowPrivilegeEscalation must be false", name)
				}
				if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
					t.Errorf("%s: readOnlyRootFilesystem must be true", name)
				}
				if !slices.Equal(sc.Capabilities.Drop, []string{"ALL"}) || len(sc.Capabilities.Add) > 0 {
					t.Errorf("%s: drop ALL capabilities and add none", name)
				}
			}
		}
	})
	// endregion: security-test

	// region: selectors-test
	t.Run("selectors find their pods", func(t *testing.T) {
		for name, w := range ws {
			if !subset(w.Spec.Selector.MatchLabels, w.Spec.Template.Metadata.Labels) {
				t.Errorf("%s: the selector %v does not match its own pod labels %v",
					name, w.Spec.Selector.MatchLabels, w.Spec.Template.Metadata.Labels)
			}
		}
		for name, s := range ss {
			var found []string
			for wname, w := range ws {
				if subset(s.Spec.Selector, w.Spec.Template.Metadata.Labels) {
					found = append(found, wname)
				}
			}
			if len(found) != 1 || found[0] != name {
				t.Errorf("Service %s selects the pods of %v, want exactly %s", name, found, name)
				continue
			}
			named := map[string]bool{}
			for _, c := range ws[name].Spec.Template.Spec.Containers {
				for _, p := range c.Ports {
					named[p.Name] = true
				}
			}
			for _, p := range s.Spec.Ports {
				if !named[p.TargetPort] {
					t.Errorf("Service %s: targetPort %q is not a port name of its pods", name, p.TargetPort)
				}
			}
		}
	})
	// endregion: selectors-test

	t.Run("Postgres keeps its data on a claim", func(t *testing.T) {
		pg := ws["postgres"]
		if s, ok := ss[pg.Spec.ServiceName]; !ok || s.Spec.ClusterIP != "None" {
			t.Errorf("serviceName %q must name a headless Service", pg.Spec.ServiceName)
		}
		if len(pg.Spec.VolumeClaimTemplates) != 1 || pg.Spec.VolumeClaimTemplates[0].Metadata.Name != "data" {
			t.Fatal("want one volumeClaimTemplate named data")
		}
		c := pg.Spec.Template.Spec.Containers[0]
		var mount, pgdata string
		for _, m := range c.VolumeMounts {
			if m.Name == "data" {
				mount = m.MountPath
			}
		}
		for _, e := range c.Env {
			if e.Name == "PGDATA" {
				pgdata = e.Value
			}
		}
		if mount == "" || !strings.HasPrefix(pgdata, mount+"/") {
			t.Errorf("PGDATA %q must be a directory inside the claim, mounted at %q", pgdata, mount)
		}
	})

	t.Run("the programs reach Postgres through the Service", func(t *testing.T) {
		for _, name := range []string{"api", "worker", "scheduler", "alerter"} {
			c := ws[name].Spec.Template.Spec.Containers[0]
			i := slices.IndexFunc(c.Env, func(e envVar) bool { return e.Name == "DATABASE_URL" })
			if i < 0 || !strings.Contains(c.Env[i].Value, "$(POSTGRES_PASSWORD)@postgres:5432/") {
				t.Errorf("%s: DATABASE_URL must take $(POSTGRES_PASSWORD) and point at postgres:5432", name)
			}
		}
	})

	if !rendered {
		return
	}

	// region: overlay-test
	t.Run("the kind overlay", func(t *testing.T) {
		var secret string
		for _, o := range objs {
			if o.Kind == "Namespace" {
				// enforce refuses a pod; warn shows the breach in the
				// output of kubectl apply, at the Deployment.
				for _, mode := range []string{"enforce", "warn"} {
					l := o.Metadata.Labels
					if l["pod-security.kubernetes.io/"+mode] != "restricted" || l["pod-security.kubernetes.io/"+mode+"-version"] != "v1.36" {
						t.Errorf("the namespace must %s the restricted standard of v1.36", mode)
					}
				}
				continue
			}
			if o.Metadata.Namespace != "your-monitor" {
				t.Errorf("%s %s is in namespace %q, want your-monitor", o.Kind, o.Metadata.Name, o.Metadata.Namespace)
			}
			if o.Kind == "Secret" {
				secret = o.Metadata.Name
			}
		}
		// Kustomize adds a hash of the content to the name, and rewrites
		// every reference to the new name.
		if !strings.HasPrefix(secret, "postgres-") {
			t.Fatalf("no generated postgres Secret, got %q", secret)
		}
		for name, w := range ws {
			for _, c := range w.Spec.Template.Spec.Containers {
				for _, ef := range c.EnvFrom {
					if ef.SecretRef != nil && ef.SecretRef.Name != secret {
						t.Errorf("%s: secretRef %q, want %q", name, ef.SecretRef.Name, secret)
					}
				}
				if name == "postgres" {
					continue
				}
				if c.Image != "your-monitor-"+name+":dev" || c.ImagePullPolicy != "Never" {
					t.Errorf("%s: image %s, pull policy %q; want your-monitor-%s:dev, Never", name, c.Image, c.ImagePullPolicy, name)
				}
			}
		}
		web := ss["web"]
		if web.Spec.Type != "NodePort" || web.Spec.Ports[0].NodePort != kindNodePort(t) {
			t.Errorf("web: type %s, nodePort %d; want NodePort on the port kind-config.yaml maps", web.Spec.Type, web.Spec.Ports[0].NodePort)
		}
	})
	// endregion: overlay-test
}

// kindConfig holds the fields of kind's Cluster config, v1alpha4, that this
// repository uses. Decoding with KnownFields fails on any other field, the
// way kind does.
type kindConfig struct {
	Kind       string `yaml:"kind"`
	APIVersion string `yaml:"apiVersion"`
	Name       string `yaml:"name"`
	Nodes      []struct {
		Role              string `yaml:"role"`
		Image             string `yaml:"image"`
		ExtraPortMappings []struct {
			ContainerPort int    `yaml:"containerPort"`
			HostPort      int    `yaml:"hostPort"`
			ListenAddress string `yaml:"listenAddress"`
			Protocol      string `yaml:"protocol"`
		} `yaml:"extraPortMappings"`
	} `yaml:"nodes"`
}

func readKindConfig(t *testing.T) kindConfig {
	t.Helper()
	b, err := os.ReadFile("kind-config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var c kindConfig
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		t.Fatalf("kind-config.yaml: %v", err)
	}
	return c
}

func kindNodePort(t *testing.T) int {
	c := readKindConfig(t)
	return c.Nodes[0].ExtraPortMappings[0].ContainerPort
}

func TestKindConfig(t *testing.T) {
	c := readKindConfig(t)
	if c.Kind != "Cluster" || c.APIVersion != "kind.x-k8s.io/v1alpha4" || c.Name != "your-monitor" {
		t.Errorf("got %s %s %q, want a kind.x-k8s.io/v1alpha4 Cluster named your-monitor", c.APIVersion, c.Kind, c.Name)
	}
	if len(c.Nodes) != 1 || c.Nodes[0].Role != "control-plane" {
		t.Fatal("want one control-plane node")
	}
	n := c.Nodes[0]
	if !strings.HasPrefix(n.Image, "kindest/node:v1.36.") || !strings.Contains(n.Image, "@sha256:") {
		t.Errorf("node image %q: want kindest/node:v1.36.x pinned by digest", n.Image)
	}
	if len(n.ExtraPortMappings) != 1 {
		t.Fatal("want one port mapping, for the status page")
	}
	m := n.ExtraPortMappings[0]
	if m.HostPort != 3000 || m.ListenAddress != "127.0.0.1" || m.ContainerPort < 30000 || m.ContainerPort > 32767 {
		t.Errorf("port mapping %+v: want 127.0.0.1:3000 to a NodePort from 30000 to 32767", m)
	}
}

// region: week16-helpers

// built renders dir, or skips the test when kubectl is missing: the
// Week 16 checks need the overlay, not only the base files.
func built(t *testing.T, dir string) []object {
	t.Helper()
	objs, rendered := render(t, dir)
	if !rendered {
		t.Skip("kubectl is not on PATH")
	}
	return objs
}

func find(objs []object, kind, name string) (object, bool) {
	for _, o := range objs {
		if o.Kind == kind && o.Metadata.Name == name {
			return o, true
		}
	}
	return object{}, false
}

// rule is one rule of a Role: the verbs it allows on the resources of
// the API groups it names.
type rule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

type role struct {
	Rules []rule `yaml:"rules"`
}

// endregion: week16-helpers

const namespace = "your-monitor"

// TestProdHoldsNoSecretsAndNothingClusterWide renders the prod overlay.
// Every object must land in the app namespace: the CI ServiceAccount can
// write nowhere else. No Secret may appear: the database password and
// the GHCR token stay out of Git.
func TestProdHoldsNoSecretsAndNothingClusterWide(t *testing.T) {
	objs := built(t, "overlays/prod")
	if len(objs) == 0 {
		t.Fatal("the prod overlay renders no objects")
	}
	clusterWide := []string{"Namespace", "ClusterRole", "ClusterRoleBinding", "ClusterIssuer", "CustomResourceDefinition", "PersistentVolume", "IngressClass", "StorageClass", "ValidatingAdmissionPolicy", "ValidatingAdmissionPolicyBinding"}
	for _, o := range objs {
		if o.Metadata.Namespace != namespace {
			t.Errorf("%s %s is in namespace %q, want %q", o.Kind, o.Metadata.Name, o.Metadata.Namespace, namespace)
		}
		if slices.Contains(clusterWide, o.Kind) {
			t.Errorf("%s %s is cluster-wide: it belongs in deploy/k8s/cluster, which you apply as administrator", o.Kind, o.Metadata.Name)
		}
		if o.Kind == "Secret" {
			t.Errorf("Secret %s is in the prod overlay: create it on the cluster by hand, never in Git", o.Metadata.Name)
		}
	}
}

// TestDeploymentsPullFromGHCRWithASecret checks the patch of the prod
// overlay: every Deployment names the ghcr-pull Secret, because GHCR
// packages start out private.
func TestDeploymentsPullFromGHCRWithASecret(t *testing.T) {
	n := 0
	for _, o := range built(t, "overlays/prod") {
		if o.Kind != "Deployment" {
			continue
		}
		n++
		d := as[struct {
			Spec struct {
				Template struct {
					Spec struct {
						ImagePullSecrets []struct{ Name string } `yaml:"imagePullSecrets"`
					}
				}
			}
		}](t, o)
		got := d.Spec.Template.Spec.ImagePullSecrets
		if len(got) != 1 || got[0].Name != "ghcr-pull" {
			t.Errorf("Deployment %s pulls with %v, want [ghcr-pull]", o.Metadata.Name, got)
		}
	}
	if n != 5 {
		t.Errorf("the prod overlay renders %d Deployments, want 5", n)
	}
}

// region: rbac-test

// resources maps each kind the prod overlay may hold to its API group
// and resource name, the two things a Role rule names.
var resources = map[string][2]string{
	"ConfigMap":   {"", "configmaps"},
	"Service":     {"", "services"},
	"Deployment":  {"apps", "deployments"},
	"StatefulSet": {"apps", "statefulsets"},
	"Ingress":     {"networking.k8s.io", "ingresses"},
	"Middleware":  {"traefik.io", "middlewares"},
	// Week 17
	"HorizontalPodAutoscaler": {"autoscaling", "horizontalpodautoscalers"},
	"CronJob":                 {"batch", "cronjobs"},
}

// allows reports whether one of the rules grants verb on resource in group.
func allows(rules []rule, group, resource, verb string) bool {
	for _, r := range rules {
		if slices.Contains(r.APIGroups, group) && slices.Contains(r.Resources, resource) && slices.Contains(r.Verbs, verb) {
			return true
		}
	}
	return false
}

// TestDeployerCanApplyTheProdOverlay checks the Role of the CI
// ServiceAccount against what deploy.yml applies. kubectl apply reads
// each object, then creates or patches it. kubectl rollout status lists
// and watches the Deployments. A new kind in the overlay fails this test
// until the Role grants it.
func TestDeployerCanApplyTheProdOverlay(t *testing.T) {
	o, ok := find(built(t, "cluster"), "Role", "deployer")
	if !ok {
		t.Fatal("deploy/k8s/cluster has no Role named deployer")
	}
	rules := as[role](t, o).Rules
	for _, obj := range built(t, "overlays/prod") {
		gr, ok := resources[obj.Kind]
		if !ok {
			t.Errorf("kind %s is not in the resources map: add it there and to the deployer Role", obj.Kind)
			continue
		}
		verbs := []string{"get", "create", "patch"}
		if obj.Kind == "Deployment" {
			verbs = append(verbs, "list", "watch")
		}
		for _, verb := range verbs {
			if !allows(rules, gr[0], gr[1], verb) {
				t.Errorf("the deployer Role cannot %s %s, which deploy.yml applies as %s %s", verb, gr[1], obj.Kind, obj.Metadata.Name)
			}
		}
	}
}

// endregion: rbac-test

// TestDeployerRoleIsLeastPrivilege checks what the Role must never
// grant: wildcards, deletes, Secrets, Pods, and any way to change RBAC
// or act as someone else.
func TestDeployerRoleIsLeastPrivilege(t *testing.T) {
	objs := built(t, "cluster")
	for _, o := range objs {
		if o.Kind == "ClusterRole" || o.Kind == "ClusterRoleBinding" {
			t.Errorf("deploy/k8s/cluster holds %s %s: CI rights stay in one namespace", o.Kind, o.Metadata.Name)
		}
	}
	o, ok := find(objs, "Role", "deployer")
	if !ok || o.Metadata.Namespace != namespace {
		t.Fatalf("no Role deployer in namespace %s", namespace)
	}
	forbiddenVerbs := []string{"*", "delete", "deletecollection", "escalate", "bind", "impersonate"}
	forbiddenResources := []string{"*", "secrets", "pods", "pods/exec", "roles", "rolebindings", "serviceaccounts", "serviceaccounts/token"}
	for _, r := range as[role](t, o).Rules {
		for _, v := range r.Verbs {
			if slices.Contains(forbiddenVerbs, v) {
				t.Errorf("the deployer Role grants %q on %v", v, r.Resources)
			}
		}
		for _, res := range r.Resources {
			if slices.Contains(forbiddenResources, res) {
				t.Errorf("the deployer Role grants %v on %q", r.Verbs, res)
			}
		}
		if slices.Contains(r.APIGroups, "*") || slices.Contains(r.APIGroups, "rbac.authorization.k8s.io") || slices.Contains(r.APIGroups, "admissionregistration.k8s.io") {
			t.Errorf("the deployer Role names API groups %v", r.APIGroups)
		}
	}

	type subject struct{ Kind, Name, Namespace string }
	b, ok := find(objs, "RoleBinding", "deployer")
	binding := as[struct {
		RoleRef  struct{ APIGroup, Kind, Name string } `yaml:"roleRef"`
		Subjects []subject
	}](t, b)
	want := []subject{{"ServiceAccount", "deployer", namespace}}
	if !ok || binding.RoleRef.Kind != "Role" || binding.RoleRef.Name != "deployer" || !slices.Equal(binding.Subjects, want) {
		t.Errorf("RoleBinding deployer binds %+v to %+v, want Role deployer to %+v", binding.RoleRef, binding.Subjects, want)
	}
	s, ok := find(objs, "ServiceAccount", "deployer")
	sa := as[struct {
		Automount *bool `yaml:"automountServiceAccountToken"`
	}](t, s)
	if !ok || s.Metadata.Namespace != namespace || sa.Automount == nil || *sa.Automount {
		t.Errorf("ServiceAccount deployer must exist in %s with automountServiceAccountToken: false", namespace)
	}
}

// TestNamespaceEnforcesRestrictedPodSecurity checks the labels that stop
// a Deployment in the app namespace from running pods as root, with
// extra capabilities, or with any access to the VPS itself, and that
// make kubectl apply warn about such a Deployment.
func TestNamespaceEnforcesRestrictedPodSecurity(t *testing.T) {
	ns, ok := find(built(t, "cluster"), "Namespace", namespace)
	if !ok {
		t.Fatalf("deploy/k8s/cluster has no Namespace %s", namespace)
	}
	for k, v := range map[string]string{
		"pod-security.kubernetes.io/enforce":         "restricted",
		"pod-security.kubernetes.io/enforce-version": "v1.36",
		"pod-security.kubernetes.io/warn":            "restricted",
		"pod-security.kubernetes.io/warn-version":    "v1.36",
	} {
		if got := ns.Metadata.Labels[k]; got != v {
			t.Errorf("namespace %s has label %s=%q, want %q", namespace, k, got, v)
		}
	}
}

// TestServicesStayClusterIP checks the admission policy that keeps every
// Service in the app namespace off the ports of the VPS, and that the
// prod overlay itself asks for no other type.
func TestServicesStayClusterIP(t *testing.T) {
	objs := built(t, "cluster")
	p, ok := find(objs, "ValidatingAdmissionPolicy", "clusterip-services-only")
	if !ok {
		t.Fatal("deploy/k8s/cluster has no ValidatingAdmissionPolicy clusterip-services-only")
	}
	policy := as[struct {
		Spec struct {
			FailurePolicy    string `yaml:"failurePolicy"`
			MatchConstraints struct {
				ResourceRules []struct {
					APIGroups  []string `yaml:"apiGroups"`
					Operations []string `yaml:"operations"`
					Resources  []string `yaml:"resources"`
				} `yaml:"resourceRules"`
			} `yaml:"matchConstraints"`
			Validations []struct{ Expression string }
		}
	}](t, p)
	rr := policy.Spec.MatchConstraints.ResourceRules
	if len(rr) != 1 || !slices.Equal(rr[0].APIGroups, []string{""}) || !slices.Equal(rr[0].Resources, []string{"services"}) ||
		!slices.Contains(rr[0].Operations, "CREATE") || !slices.Contains(rr[0].Operations, "UPDATE") {
		t.Errorf("the policy matches %+v, want services on CREATE and UPDATE", rr)
	}
	if policy.Spec.FailurePolicy != "Fail" {
		t.Errorf("failurePolicy is %q, want Fail", policy.Spec.FailurePolicy)
	}
	var exprs []string
	for _, v := range policy.Spec.Validations {
		exprs = append(exprs, v.Expression)
	}
	// The type alone is not enough: a ClusterIP Service with externalIPs
	// takes traffic for the public address of the VPS, past ufw.
	want := []string{
		"object.spec.type == 'ClusterIP'",
		"!has(object.spec.externalIPs) || size(object.spec.externalIPs) == 0",
	}
	if !slices.Equal(exprs, want) {
		t.Errorf("the policy validates %q, want %q", exprs, want)
	}

	b, ok := find(objs, "ValidatingAdmissionPolicyBinding", "clusterip-services-only")
	binding := as[struct {
		Spec struct {
			PolicyName        string   `yaml:"policyName"`
			ValidationActions []string `yaml:"validationActions"`
			MatchResources    struct {
				NamespaceSelector struct {
					MatchLabels      map[string]string `yaml:"matchLabels"`
					MatchExpressions []struct {
						Key      string
						Operator string
						Values   []string
					} `yaml:"matchExpressions"`
				} `yaml:"namespaceSelector"`
			} `yaml:"matchResources"`
		}
	}](t, b)
	// Week 18 adds the monitoring namespace, where Prometheus and
	// Alertmanager run, next to the app namespace.
	sel := binding.Spec.MatchResources.NamespaceSelector
	if !ok || binding.Spec.PolicyName != p.Metadata.Name || !slices.Equal(binding.Spec.ValidationActions, []string{"Deny"}) ||
		len(sel.MatchLabels) != 0 || len(sel.MatchExpressions) != 1 ||
		sel.MatchExpressions[0].Key != "kubernetes.io/metadata.name" || sel.MatchExpressions[0].Operator != "In" ||
		!slices.Equal(sel.MatchExpressions[0].Values, []string{namespace, "monitoring"}) {
		t.Errorf("the binding is %+v, want Deny for the namespaces %s and monitoring", binding.Spec, namespace)
	}

	for _, o := range built(t, "overlays/prod") {
		if o.Kind != "Service" {
			continue
		}
		s := as[struct {
			Spec struct {
				Type        string
				ExternalIPs []string `yaml:"externalIPs"`
			}
		}](t, o)
		if s.Spec.Type != "" && s.Spec.Type != "ClusterIP" {
			t.Errorf("Service %s has type %s: the policy refuses it", o.Metadata.Name, s.Spec.Type)
		}
		if len(s.Spec.ExternalIPs) > 0 {
			t.Errorf("Service %s sets externalIPs %v: the policy refuses it", o.Metadata.Name, s.Spec.ExternalIPs)
		}
	}
}

type issuer struct {
	Spec struct {
		ACME struct {
			Server              string
			PrivateKeySecretRef struct{ Name string } `yaml:"privateKeySecretRef"`
			Solvers             []struct {
				HTTP01 struct {
					Ingress struct {
						IngressClassName string `yaml:"ingressClassName"`
						ServiceType      string `yaml:"serviceType"`
					}
				} `yaml:"http01"`
			}
		}
	}
}

// TestIssuersUseHTTP01ThroughTraefik checks the two ClusterIssuers: one
// for the Let's Encrypt staging environment, one for production, each
// with its own account key, both solving HTTP-01 challenges through
// Traefik with a ClusterIP Service, which opens no node port.
func TestIssuersUseHTTP01ThroughTraefik(t *testing.T) {
	objs := built(t, "cluster")
	servers := map[string]string{
		"letsencrypt-staging": "https://acme-staging-v02.api.letsencrypt.org/directory",
		"letsencrypt-prod":    "https://acme-v02.api.letsencrypt.org/directory",
	}
	keys := map[string]bool{}
	for name, server := range servers {
		o, ok := find(objs, "ClusterIssuer", name)
		if !ok {
			t.Errorf("no ClusterIssuer %s", name)
			continue
		}
		acme := as[issuer](t, o).Spec.ACME
		if acme.Server != server {
			t.Errorf("%s uses ACME server %q, want %q", name, acme.Server, server)
		}
		keys[acme.PrivateKeySecretRef.Name] = true
		if len(acme.Solvers) != 1 {
			t.Fatalf("%s has %d solvers, want 1", name, len(acme.Solvers))
		}
		ing := acme.Solvers[0].HTTP01.Ingress
		if ing.IngressClassName != "traefik" || ing.ServiceType != "ClusterIP" {
			t.Errorf("%s solves HTTP-01 with ingress class %q and Service type %q, want traefik and ClusterIP", name, ing.IngressClassName, ing.ServiceType)
		}
	}
	if len(keys) != 2 {
		t.Errorf("the issuers share an account key Secret %v: staging and production accounts are separate", keys)
	}
}

// TestIngressServesTheStatusPageOverTLS checks that the pieces of the
// prod overlay point at each other: the Ingress names an issuer that
// exists, asks for a certificate for exactly the host it routes, sends
// the traffic to a Service port that exists, and uses the redirect
// Middleware by its Traefik name, <namespace>-<name>@kubernetescrd.
func TestIngressServesTheStatusPageOverTLS(t *testing.T) {
	objs := built(t, "overlays/prod")
	o, ok := find(objs, "Ingress", "status-page")
	if !ok {
		t.Fatal("the prod overlay has no Ingress status-page")
	}
	ing := as[struct {
		Spec struct {
			IngressClassName string `yaml:"ingressClassName"`
			TLS              []struct {
				Hosts      []string
				SecretName string `yaml:"secretName"`
			}
			Rules []struct {
				Host string
				HTTP struct {
					Paths []struct {
						Path    string
						Backend struct {
							Service struct {
								Name string
								Port struct{ Number int }
							}
						}
					}
				}
			}
		}
	}](t, o).Spec
	if ing.IngressClassName != "traefik" {
		t.Errorf("ingressClassName is %q, want traefik, the class K3s installs", ing.IngressClassName)
	}

	name := o.Metadata.Annotations["cert-manager.io/cluster-issuer"]
	if _, ok := find(built(t, "cluster"), "ClusterIssuer", name); !ok {
		t.Errorf("the Ingress asks ClusterIssuer %q, which deploy/k8s/cluster does not define", name)
	}

	var ruleHosts, tlsHosts []string
	for _, r := range ing.Rules {
		ruleHosts = append(ruleHosts, r.Host)
		for _, p := range r.HTTP.Paths {
			svc, ok := find(objs, "Service", p.Backend.Service.Name)
			if !ok {
				t.Errorf("path %s goes to Service %q, which the overlay does not hold", p.Path, p.Backend.Service.Name)
				continue
			}
			ports := as[struct {
				Spec struct{ Ports []struct{ Port int } }
			}](t, svc).Spec.Ports
			if !slices.ContainsFunc(ports, func(sp struct{ Port int }) bool { return sp.Port == p.Backend.Service.Port.Number }) {
				t.Errorf("path %s goes to port %d, which Service %s does not expose", p.Path, p.Backend.Service.Port.Number, svc.Metadata.Name)
			}
		}
	}
	for _, tl := range ing.TLS {
		tlsHosts = append(tlsHosts, tl.Hosts...)
		if tl.SecretName == "" {
			t.Error("a tls entry has no secretName: cert-manager has nowhere to store the certificate")
		}
	}
	slices.Sort(ruleHosts)
	slices.Sort(tlsHosts)
	if len(ruleHosts) == 0 || !slices.Equal(ruleHosts, tlsHosts) {
		t.Errorf("the Ingress routes hosts %v but asks for a certificate for %v", ruleHosts, tlsHosts)
	}

	mw, ok := find(objs, "Middleware", "redirect-https")
	if !ok {
		t.Fatal("the prod overlay has no Middleware redirect-https")
	}
	want := fmt.Sprintf("%s-%s@kubernetescrd", mw.Metadata.Namespace, mw.Metadata.Name)
	// Week 17 adds the rate limit after the redirect.
	got := o.Metadata.Annotations["traefik.ingress.kubernetes.io/router.middlewares"]
	if first, _, _ := strings.Cut(got, ","); first != want {
		t.Errorf("the Ingress uses middlewares %q, want %q first", got, want)
	}
	redirect := as[struct {
		Spec struct {
			RedirectScheme struct {
				Scheme    string
				Permanent bool
			} `yaml:"redirectScheme"`
		}
	}](t, mw).Spec.RedirectScheme
	if redirect.Scheme != "https" || !redirect.Permanent {
		t.Errorf("redirect-https redirects with %+v, want scheme https, permanent", redirect)
	}
}
