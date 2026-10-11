// Package week16 holds the tests of the Week 16 deploy files. The files
// live in your-monitor/, laid out as the reader's repository:
// deploy/k8s/overlays/prod, deploy/k8s/cluster and
// .github/workflows/deploy.yml. No cluster runs here, so the tests render
// the Kustomize directories with the Kustomize library that kubectl uses
// and check what the page promises: the prod overlay holds only
// namespaced objects and no Secrets, the CI Role can apply every object
// it holds and nothing more, and the Ingress, the issuers and the deploy
// workflow fit together.
package week16

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	repo      = "your-monitor"
	namespace = "your-monitor"
)

// object holds the fields of a Kubernetes object that the tests read.
// Spec stays a yaml.Node: each test decodes the part it needs.
type object struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name        string            `yaml:"name"`
		Namespace   string            `yaml:"namespace"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
	Spec yaml.Node `yaml:"spec"`
	// Only Role and RoleBinding objects have these.
	Rules    []rule    `yaml:"rules"`
	RoleRef  roleRef   `yaml:"roleRef"`
	Subjects []subject `yaml:"subjects"`
	// Only ServiceAccount objects have this.
	AutomountServiceAccountToken *bool `yaml:"automountServiceAccountToken"`
}

type rule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

type roleRef struct {
	APIGroup string `yaml:"apiGroup"`
	Kind     string `yaml:"kind"`
	Name     string `yaml:"name"`
}

type subject struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

func (o object) id() string { return o.Kind + "/" + o.Metadata.Name }

// decodeSpec decodes the spec of o into v.
func (o object) decodeSpec(t *testing.T, v any) {
	t.Helper()
	if err := o.Spec.Decode(v); err != nil {
		t.Fatalf("%s: %v", o.id(), err)
	}
}

// render builds the Kustomize directory dir the way kubectl apply -k
// does, and returns its objects in the order Kustomize prints them.
func render(t *testing.T, dir string) []object {
	t.Helper()
	k := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resMap, err := k.Run(filesys.MakeFsOnDisk(), dir)
	if err != nil {
		t.Fatalf("kustomize build %s: %v", dir, err)
	}
	out, err := resMap.AsYaml()
	if err != nil {
		t.Fatal(err)
	}
	var objs []object
	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var o object
		err := dec.Decode(&o)
		if errors.Is(err, io.EOF) {
			return objs
		}
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		objs = append(objs, o)
	}
}

func find(objs []object, kind, name string) (object, bool) {
	for _, o := range objs {
		if o.Kind == kind && o.Metadata.Name == name {
			return o, true
		}
	}
	return object{}, false
}

func ofKind(objs []object, kind string) []object {
	var out []object
	for _, o := range objs {
		if o.Kind == kind {
			out = append(out, o)
		}
	}
	return out
}

var (
	prodDir    = filepath.Join(repo, "deploy/k8s/overlays/prod")
	clusterDir = filepath.Join(repo, "deploy/k8s/cluster")
)

// TestProdHoldsNoSecretsAndNothingClusterWide renders the prod overlay.
// Every object must land in the app namespace: the CI ServiceAccount can
// write nowhere else. No Secret may appear: the database password and
// the GHCR token stay out of Git.
func TestProdHoldsNoSecretsAndNothingClusterWide(t *testing.T) {
	objs := render(t, prodDir)
	if len(objs) == 0 {
		t.Fatal("the prod overlay renders no objects")
	}
	clusterWide := []string{"Namespace", "ClusterRole", "ClusterRoleBinding", "ClusterIssuer", "CustomResourceDefinition", "PersistentVolume", "IngressClass", "StorageClass"}
	for _, o := range objs {
		if o.Metadata.Namespace != namespace {
			t.Errorf("%s is in namespace %q, want %q", o.id(), o.Metadata.Namespace, namespace)
		}
		if slices.Contains(clusterWide, o.Kind) {
			t.Errorf("%s is cluster-wide: it belongs in deploy/k8s/cluster, which you apply as administrator", o.id())
		}
		if o.Kind == "Secret" {
			t.Errorf("%s is in the prod overlay: create it on the cluster by hand, never in Git", o.id())
		}
	}
}

// TestDeploymentsPullFromGHCRWithASecret checks the patch of the prod
// overlay: every Deployment names the ghcr-pull Secret, because GHCR
// packages start out private.
func TestDeploymentsPullFromGHCRWithASecret(t *testing.T) {
	deployments := ofKind(render(t, prodDir), "Deployment")
	if len(deployments) != 5 {
		t.Fatalf("the prod overlay renders %d Deployments, want 5", len(deployments))
	}
	for _, d := range deployments {
		var spec struct {
			Template struct {
				Spec struct {
					ImagePullSecrets []struct{ Name string } `yaml:"imagePullSecrets"`
				}
			}
		}
		d.decodeSpec(t, &spec)
		got := spec.Template.Spec.ImagePullSecrets
		if len(got) != 1 || got[0].Name != "ghcr-pull" {
			t.Errorf("%s pulls with %v, want [ghcr-pull]", d.id(), got)
		}
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
	role, ok := find(render(t, clusterDir), "Role", "deployer")
	if !ok {
		t.Fatal("deploy/k8s/cluster has no Role named deployer")
	}
	for _, o := range render(t, prodDir) {
		gr, ok := resources[o.Kind]
		if !ok {
			t.Errorf("%s: kind %s is not in the resources map; add it there and to the deployer Role", o.id(), o.Kind)
			continue
		}
		verbs := []string{"get", "create", "patch"}
		if o.Kind == "Deployment" {
			verbs = append(verbs, "list", "watch")
		}
		for _, verb := range verbs {
			if !allows(role.Rules, gr[0], gr[1], verb) {
				t.Errorf("the deployer Role cannot %s %s, which deploy.yml applies as %s", verb, gr[1], o.id())
			}
		}
	}
}

// endregion: rbac-test

// TestDeployerRoleIsLeastPrivilege checks what the Role must never
// grant: wildcards, deletes, Secrets, Pods, and any way to change RBAC
// or act as someone else.
func TestDeployerRoleIsLeastPrivilege(t *testing.T) {
	objs := render(t, clusterDir)
	if roles := ofKind(objs, "ClusterRole"); len(roles) != 0 {
		t.Errorf("deploy/k8s/cluster holds ClusterRoles %v: CI rights stay in one namespace", roles)
	}
	role, _ := find(objs, "Role", "deployer")
	if role.Metadata.Namespace != namespace {
		t.Errorf("the deployer Role is in namespace %q, want %q", role.Metadata.Namespace, namespace)
	}
	forbiddenVerbs := []string{"*", "delete", "deletecollection", "escalate", "bind", "impersonate"}
	forbiddenResources := []string{"*", "secrets", "pods", "pods/exec", "roles", "rolebindings", "serviceaccounts", "serviceaccounts/token"}
	for _, r := range role.Rules {
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
		if slices.Contains(r.APIGroups, "*") || slices.Contains(r.APIGroups, "rbac.authorization.k8s.io") {
			t.Errorf("the deployer Role names API groups %v", r.APIGroups)
		}
	}

	binding, ok := find(objs, "RoleBinding", "deployer")
	want := []subject{{Kind: "ServiceAccount", Name: "deployer", Namespace: namespace}}
	if !ok || binding.RoleRef != (roleRef{"rbac.authorization.k8s.io", "Role", "deployer"}) || !slices.Equal(binding.Subjects, want) {
		t.Errorf("RoleBinding deployer binds %+v to %+v, want Role deployer to %+v", binding.RoleRef, binding.Subjects, want)
	}
	sa, ok := find(objs, "ServiceAccount", "deployer")
	if !ok || sa.Metadata.Namespace != namespace || sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
		t.Errorf("ServiceAccount deployer must exist in %s with automountServiceAccountToken: false", namespace)
	}
}

// TestNamespaceEnforcesBaselinePodSecurity checks the label that stops a
// Deployment in the app namespace from running privileged pods or
// mounting the VPS disk.
func TestNamespaceEnforcesBaselinePodSecurity(t *testing.T) {
	ns, ok := find(render(t, clusterDir), "Namespace", namespace)
	if !ok {
		t.Fatalf("deploy/k8s/cluster has no Namespace %s", namespace)
	}
	if got := ns.Metadata.Labels["pod-security.kubernetes.io/enforce"]; got != "baseline" && got != "restricted" {
		t.Errorf("namespace %s enforces Pod Security level %q, want baseline", namespace, got)
	}
}

type solver struct {
	HTTP01 struct {
		Ingress struct {
			IngressClassName string `yaml:"ingressClassName"`
			ServiceType      string `yaml:"serviceType"`
		}
	} `yaml:"http01"`
}

type issuerSpec struct {
	ACME struct {
		Server              string
		PrivateKeySecretRef struct{ Name string } `yaml:"privateKeySecretRef"`
		Solvers             []solver
	}
}

// TestIssuersUseHTTP01ThroughTraefik checks the two ClusterIssuers: one
// for the Let's Encrypt staging environment, one for production, each
// with its own account key, both solving HTTP-01 challenges through
// Traefik with a ClusterIP Service, which opens no node port.
func TestIssuersUseHTTP01ThroughTraefik(t *testing.T) {
	objs := render(t, clusterDir)
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
		var spec issuerSpec
		o.decodeSpec(t, &spec)
		if spec.ACME.Server != server {
			t.Errorf("%s uses ACME server %q, want %q", name, spec.ACME.Server, server)
		}
		keys[spec.ACME.PrivateKeySecretRef.Name] = true
		if len(spec.ACME.Solvers) != 1 {
			t.Fatalf("%s has %d solvers, want 1", name, len(spec.ACME.Solvers))
		}
		ing := spec.ACME.Solvers[0].HTTP01.Ingress
		if ing.IngressClassName != "traefik" || ing.ServiceType != "ClusterIP" {
			t.Errorf("%s solves HTTP-01 with ingress class %q and Service type %q, want traefik and ClusterIP", name, ing.IngressClassName, ing.ServiceType)
		}
	}
	if len(keys) != 2 {
		t.Errorf("the issuers share an account key Secret %v: staging and production accounts are separate", keys)
	}
}

// region: ingress-test

// TestIngressServesTheStatusPageOverTLS checks that the pieces of the
// prod overlay point at each other: the Ingress names an issuer that
// exists, asks for a certificate for exactly the host it routes, sends
// the traffic to a Service port that exists, and uses the redirect
// Middleware by its Traefik name, <namespace>-<name>@kubernetescrd.
func TestIngressServesTheStatusPageOverTLS(t *testing.T) {
	objs := render(t, prodDir)
	ing, ok := find(objs, "Ingress", "status-page")
	if !ok {
		t.Fatal("the prod overlay has no Ingress status-page")
	}
	var spec struct {
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
	ing.decodeSpec(t, &spec)
	if spec.IngressClassName != "traefik" {
		t.Errorf("ingressClassName is %q, want traefik, the class k3s installs", spec.IngressClassName)
	}

	issuer := ing.Metadata.Annotations["cert-manager.io/cluster-issuer"]
	if _, ok := find(render(t, clusterDir), "ClusterIssuer", issuer); !ok {
		t.Errorf("the Ingress asks ClusterIssuer %q, which deploy/k8s/cluster does not define", issuer)
	}

	var ruleHosts, tlsHosts []string
	for _, r := range spec.Rules {
		ruleHosts = append(ruleHosts, r.Host)
		for _, p := range r.HTTP.Paths {
			svc, ok := find(objs, "Service", p.Backend.Service.Name)
			if !ok {
				t.Errorf("path %s goes to Service %q, which the overlay does not hold", p.Path, p.Backend.Service.Name)
				continue
			}
			var s struct{ Ports []struct{ Port int } }
			svc.decodeSpec(t, &s)
			if !slices.ContainsFunc(s.Ports, func(sp struct{ Port int }) bool { return sp.Port == p.Backend.Service.Port.Number }) {
				t.Errorf("path %s goes to port %d, which Service %s does not expose", p.Path, p.Backend.Service.Port.Number, svc.Metadata.Name)
			}
		}
	}
	for _, tl := range spec.TLS {
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
	if got := ing.Metadata.Annotations["traefik.ingress.kubernetes.io/router.middlewares"]; got != want {
		t.Errorf("the Ingress uses middlewares %q, want %q", got, want)
	}
	var redirect struct {
		RedirectScheme struct {
			Scheme    string
			Permanent bool
		} `yaml:"redirectScheme"`
	}
	mw.decodeSpec(t, &redirect)
	if redirect.RedirectScheme.Scheme != "https" || !redirect.RedirectScheme.Permanent {
		t.Errorf("redirect-https redirects with %+v, want scheme https, permanent", redirect.RedirectScheme)
	}
}

// endregion: ingress-test
