// Package k8s holds the tests of the Kubernetes manifests: the base, the
// overlays and the cluster setup. It has no other Go code. The tests read
// the objects the way kubectl apply -k sends them, and check what the
// week pages promise.
//
// contract stub: week 15. The types, decode, as and render come from
// Week 15's manifests_test.go, with two Week 16 changes: meta gains
// Annotations, and render takes the directory to build, so Week 15's
// own call becomes render(t, "overlays/kind").
package k8s

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	if v := policy.Spec.Validations; len(v) != 1 || v[0].Expression != "object.spec.type == 'ClusterIP'" {
		t.Errorf("the policy validates %+v, want object.spec.type == 'ClusterIP'", v)
	}

	b, ok := find(objs, "ValidatingAdmissionPolicyBinding", "clusterip-services-only")
	binding := as[struct {
		Spec struct {
			PolicyName        string   `yaml:"policyName"`
			ValidationActions []string `yaml:"validationActions"`
			MatchResources    struct {
				NamespaceSelector struct {
					MatchLabels map[string]string `yaml:"matchLabels"`
				} `yaml:"namespaceSelector"`
			} `yaml:"matchResources"`
		}
	}](t, b)
	if !ok || binding.Spec.PolicyName != p.Metadata.Name || !slices.Equal(binding.Spec.ValidationActions, []string{"Deny"}) ||
		binding.Spec.MatchResources.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != namespace {
		t.Errorf("the binding is %+v, want Deny for namespace %s", binding.Spec, namespace)
	}

	for _, o := range built(t, "overlays/prod") {
		if o.Kind != "Service" {
			continue
		}
		s := as[struct{ Spec struct{ Type string } }](t, o)
		if s.Spec.Type != "" && s.Spec.Type != "ClusterIP" {
			t.Errorf("Service %s has type %s: the policy refuses it", o.Metadata.Name, s.Spec.Type)
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
	if got := o.Metadata.Annotations["traefik.ingress.kubernetes.io/router.middlewares"]; got != want {
		t.Errorf("the Ingress uses middlewares %q, want %q", got, want)
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
