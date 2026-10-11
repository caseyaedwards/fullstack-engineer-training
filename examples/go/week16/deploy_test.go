package week16

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type step struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

type workflow struct {
	Jobs map[string]struct {
		Environment string `yaml:"environment"`
		Steps       []step `yaml:"steps"`
	} `yaml:"jobs"`
}

func deploySteps(t *testing.T) []step {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".github/workflows/deploy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	job, ok := w.Jobs["deploy"]
	if !ok {
		t.Fatal("deploy.yml has no deploy job")
	}
	return job.Steps
}

func stepIndex(steps []step, name string) int {
	return slices.IndexFunc(steps, func(s step) bool { return s.Name == name })
}

// region: tunnel-test

// TestTunnelOpensBeforeKubectl checks the step that Week 16 adds to
// deploy.yml. It runs after the kubeconfig exists and before anything
// talks to the cluster. It forwards one port, to the API server on the
// VPS, refuses a host key it does not know, and fails the step when the
// forward does not open. The secrets reach it through env, never through
// ${{ }} inside the script.
func TestTunnelOpensBeforeKubectl(t *testing.T) {
	steps := deploySteps(t)
	kubeconfig := stepIndex(steps, "Write the kubeconfig")
	tunnel := stepIndex(steps, "Open the tunnel to the API server")
	apply := stepIndex(steps, "Apply and wait")
	if kubeconfig < 0 || tunnel < 0 || apply < 0 || !(kubeconfig < tunnel && tunnel < apply) {
		t.Fatalf("steps at %d, %d, %d: want Write the kubeconfig, then Open the tunnel to the API server, then Apply and wait", kubeconfig, tunnel, apply)
	}

	s := steps[tunnel]
	for _, want := range []string{
		"-L 127.0.0.1:6443:127.0.0.1:6443",
		"-o StrictHostKeyChecking=yes",
		"-o ExitOnForwardFailure=yes",
		"-o BatchMode=yes",
		`"tunnel@$DEPLOY_HOST"`,
		"umask 077",
		"kubectl auth can-i patch deployments",
	} {
		if !strings.Contains(s.Run, want) {
			t.Errorf("the tunnel step does not run %q", want)
		}
	}
	wantEnv := map[string]string{
		"SSH_KEY":     "${{ secrets.DEPLOY_SSH_KEY }}",
		"KNOWN_HOSTS": "${{ secrets.DEPLOY_KNOWN_HOSTS }}",
		"DEPLOY_HOST": "${{ vars.DEPLOY_HOST }}",
	}
	for k, v := range wantEnv {
		if s.Env[k] != v {
			t.Errorf("tunnel step env %s is %q, want %q", k, s.Env[k], v)
		}
	}
	for _, st := range steps {
		if strings.Contains(st.Run, "${{") {
			t.Errorf("step %q puts a ${{ }} expression inside its script", st.Name)
		}
	}
}

// endregion: tunnel-test

// TestRolloutWaitsForEveryDeployment reads the deployment names that the
// Apply and wait step passes to kubectl rollout status, and checks them
// against the Deployments that the prod overlay holds.
func TestRolloutWaitsForEveryDeployment(t *testing.T) {
	steps := deploySteps(t)
	apply := steps[stepIndex(steps, "Apply and wait")]
	m := regexp.MustCompile(`for d in ([a-z ]+); do`).FindStringSubmatch(apply.Run)
	if m == nil {
		t.Fatalf("no for d in ... loop in:\n%s", apply.Run)
	}
	waited := strings.Fields(m[1])
	var rendered []string
	for _, d := range ofKind(render(t, filepath.Join(repo, "deploy/k8s/overlays/prod")), "Deployment") {
		rendered = append(rendered, d.Metadata.Name)
	}
	slices.Sort(waited)
	slices.Sort(rendered)
	if !slices.Equal(waited, rendered) {
		t.Errorf("deploy.yml waits for %v, the prod overlay holds Deployments %v", waited, rendered)
	}
}

// TestReleaseRendersTheGHCRImages runs the Pick the images step of
// deploy.yml with bash on a copy of deploy/, then renders the release
// Kustomization it writes, the way kubectl apply -k sees it. Every
// program runs its GHCR image at the commit SHA. Postgres keeps its
// image from Docker Hub.
func TestReleaseRendersTheGHCRImages(t *testing.T) {
	out, err := exec.Command("bash", "-c", "echo ${BASH_VERSINFO[0]}").Output()
	if err != nil || strings.TrimSpace(string(out)) < "4" {
		// ${GITHUB_REPOSITORY,,} needs bash 4 or newer. macOS ships 3.2.
		t.Skip("bash 4 or newer is not installed")
	}
	dir := t.TempDir()
	if err := os.CopyFS(filepath.Join(dir, "deploy"), os.DirFS(filepath.Join(repo, "deploy"))); err != nil {
		t.Fatal(err)
	}
	steps := deploySteps(t)
	pick := steps[stepIndex(steps, "Pick the images")]
	const sha = "0123456789abcdef0123456789abcdef01234567"
	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-c", pick.Run)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GITHUB_REPOSITORY=Ada-Lovelace/your-monitor", "SHA="+sha)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Pick the images: %v\n%s", err, b)
	}

	images := map[string]string{}
	for _, o := range render(t, filepath.Join(dir, "deploy/k8s/release")) {
		if o.Kind != "Deployment" && o.Kind != "StatefulSet" {
			continue
		}
		var spec struct {
			Template struct {
				Spec struct{ Containers []struct{ Image string } }
			}
		}
		o.decodeSpec(t, &spec)
		images[o.Metadata.Name] = spec.Template.Spec.Containers[0].Image
	}
	for _, p := range []string{"api", "worker", "scheduler", "alerter", "web"} {
		if want := "ghcr.io/ada-lovelace/your-monitor/" + p + ":" + sha; images[p] != want {
			t.Errorf("%s runs image %q, want %q", p, images[p], want)
		}
	}
	if images["postgres"] != "postgres:17" {
		t.Errorf("postgres runs image %q, want postgres:17", images["postgres"])
	}
}
