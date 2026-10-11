// Package k8s holds the tests of the Kubernetes manifests in base/ and
// overlays/kind/, and of kind-config.yaml. It has no other Go code. The
// tests read the objects the way kubectl apply -k sends them, and check
// what the week page promises: probes, resources, the restricted Pod
// Security Standard, labels and selectors, and the Postgres volume.
package k8s

import (
	"bytes"
	"errors"
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
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
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

// render returns the objects of the kind overlay as kubectl apply -k
// sends them. It runs kubectl kustomize on a copy of this directory, with
// a made-up postgres.env, because the real file never enters Git. Without
// kubectl, the test fails in CI. On your laptop, it reads the base files
// as they are instead, and skips the checks that need the overlay.
func render(t *testing.T) (objs []object, rendered bool) {
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

	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(".")); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(dir, "overlays", "kind", "postgres.env")
	if err := os.WriteFile(env, []byte("POSTGRES_PASSWORD=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(kubectl, "kustomize", filepath.Join(dir, "overlays", "kind"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("kubectl kustomize: %v\n%s", err, stderr.String())
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
	objs, rendered := render(t)
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
