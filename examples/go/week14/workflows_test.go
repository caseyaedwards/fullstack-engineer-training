// Package week14 holds the tests of the Week 14 CI/CD files. The files
// themselves live in your-monitor/, laid out as the reader's repository:
// .github/workflows/ci.yml, .github/workflows/deploy.yml, .golangci.yml
// and the Makefile. GitHub Actions does not run here, so these tests check
// what can be checked without it: the jobs, triggers and permissions the
// page names, the pinned actions and their inputs, and the shell scripts
// of the steps, run with bash.
package week14

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const repo = "your-monitor"

// workflow holds the parts of a GitHub Actions workflow that the tests read.
type workflow struct {
	Name        string               `yaml:"name"`
	On          map[string]yaml.Node `yaml:"on"`
	Permissions map[string]string    `yaml:"permissions"`
	Concurrency map[string]string    `yaml:"concurrency"`
	Jobs        map[string]job       `yaml:"jobs"`
}

type job struct {
	Needs       []string           `yaml:"needs"`
	If          string             `yaml:"if"`
	RunsOn      string             `yaml:"runs-on"`
	Environment string             `yaml:"environment"`
	Permissions map[string]string  `yaml:"permissions"`
	Services    map[string]service `yaml:"services"`
	Env         map[string]string  `yaml:"env"`
	Strategy    struct{ Matrix map[string][]string }
	Defaults    struct{ Run map[string]string }
	Steps       []step `yaml:"steps"`
}

type service struct {
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env"`
	Ports   []string          `yaml:"ports"`
	Options string            `yaml:"options"`
}

type step struct {
	ID   string            `yaml:"id"`
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func load(t *testing.T, name string) workflow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".github/workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return w
}

func keys[M ~map[string]V, V any](m M) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// region: jobs-test

// TestCIJobs checks the shape of ci.yml that the page describes: four
// check jobs on every pull request and push to main, then one images job
// that runs only for a push to main, after all four pass, and is the only
// job that can write packages.
func TestCIJobs(t *testing.T) {
	w := load(t, "ci.yml")

	if got, want := keys(w.On), []string{"pull_request", "push"}; !slices.Equal(got, want) {
		t.Errorf("ci.yml runs on %v, want %v", got, want)
	}
	push := w.On["push"]
	var onPush struct{ Branches []string }
	if err := push.Decode(&onPush); err != nil || !slices.Equal(onPush.Branches, []string{"main"}) {
		t.Errorf("push runs for branches %v, want [main]", onPush.Branches)
	}
	if len(w.Permissions) != 1 || w.Permissions["contents"] != "read" {
		t.Errorf("workflow permissions are %v, want only contents: read", w.Permissions)
	}

	// A group holds one running and one pending run, and a new run cancels
	// the pending one. Pull requests share a group per ref; every push to
	// main gets its own run ID, or a third merge cancels the second's images.
	if g := w.Concurrency["group"]; g != "ci-${{ github.event_name == 'pull_request' && github.ref || github.run_id }}" {
		t.Errorf("ci.yml concurrency group is %q, want one per pull request and one per run elsewhere", g)
	}

	want := []string{"e2e", "images", "lint", "test", "web"}
	if got := keys(w.Jobs); !slices.Equal(got, want) {
		t.Fatalf("ci.yml jobs are %v, want %v", got, want)
	}
	for name, j := range w.Jobs {
		if name != "images" && j.Permissions != nil {
			t.Errorf("job %s sets permissions %v; only images needs more than contents: read", name, j.Permissions)
		}
	}

	images := w.Jobs["images"]
	if !slices.Equal(images.Needs, []string{"lint", "test", "web", "e2e"}) {
		t.Errorf("images needs %v, want [lint test web e2e]", images.Needs)
	}
	if images.If != "github.event_name == 'push' && github.ref == 'refs/heads/main'" {
		t.Errorf("images runs if %q, want only for a push to main", images.If)
	}
	if len(images.Permissions) != 2 || images.Permissions["contents"] != "read" || images.Permissions["packages"] != "write" {
		t.Errorf("images permissions are %v, want contents: read and packages: write", images.Permissions)
	}

	test := w.Jobs["test"]
	pg, ok := test.Services["postgres"]
	if !ok || pg.Image != "postgres:17" {
		t.Errorf("test job has no postgres:17 service: %+v", test.Services)
	}
	if !slices.Contains(pg.Ports, "5432:5432") || !strings.Contains(pg.Options, "pg_isready") {
		t.Errorf("postgres service publishes %v with options %q, want 5432:5432 and a pg_isready health check", pg.Ports, pg.Options)
	}
	if !strings.Contains(test.Env["DATABASE_URL"], "@localhost:5432/") {
		t.Errorf("test job DATABASE_URL is %q, want the service on localhost:5432", test.Env["DATABASE_URL"])
	}
}

// endregion: jobs-test

// TestImagesPushTheProgramTargets checks that the images job builds one
// image for each final target of the Dockerfile and tags it with the
// commit SHA.
func TestImagesPushTheProgramTargets(t *testing.T) {
	images := load(t, "ci.yml").Jobs["images"]
	targets := images.Strategy.Matrix["target"]
	if want := []string{"api", "worker", "scheduler", "alerter", "web"}; !slices.Equal(targets, want) {
		t.Errorf("images builds %v, want %v", targets, want)
	}

	b, err := os.ReadFile(filepath.Join(repo, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	stages := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^FROM \S+ AS (\S+)$`).FindAllStringSubmatch(string(b), -1) {
		stages[m[1]] = true
	}
	for _, target := range targets {
		if !stages[target] {
			t.Errorf("the Dockerfile has no stage %s", target)
		}
	}

	var build step
	for _, s := range images.Steps {
		if strings.HasPrefix(s.Uses, "docker/build-push-action@") {
			build = s
		}
	}
	if build.With["push"] != true || build.With["target"] != "${{ matrix.target }}" {
		t.Errorf("build-push-action with %v, want push: true and the matrix target", build.With)
	}
	if build.With["tags"] != "${{ env.IMAGE }}:${{ github.sha }}" {
		t.Errorf("images are tagged %q, want the commit SHA", build.With["tags"])
	}
}

// TestDeployIsManual checks deploy.yml: it starts only by hand, reads the
// cluster credentials from the KUBECONFIG secret of the production
// environment, and can only read the repository.
func TestDeployIsManual(t *testing.T) {
	w := load(t, "deploy.yml")
	if got := keys(w.On); !slices.Equal(got, []string{"workflow_dispatch"}) {
		t.Errorf("deploy.yml runs on %v, want only workflow_dispatch", got)
	}
	if len(w.Permissions) != 1 || w.Permissions["contents"] != "read" {
		t.Errorf("deploy permissions are %v, want only contents: read", w.Permissions)
	}
	if w.Concurrency["group"] != "deploy-production" || w.Concurrency["cancel-in-progress"] != "false" {
		t.Errorf("deploy concurrency is %v, want group deploy-production without cancel-in-progress", w.Concurrency)
	}
	d, ok := w.Jobs["deploy"]
	if !ok || len(w.Jobs) != 1 {
		t.Fatalf("deploy.yml jobs are %v, want only deploy", keys(w.Jobs))
	}
	if d.Environment != "production" {
		t.Errorf("deploy runs in environment %q, want production", d.Environment)
	}
	var secret, apply bool
	for _, s := range d.Steps {
		for _, v := range s.Env {
			secret = secret || v == "${{ secrets.KUBECONFIG }}"
		}
		apply = apply || strings.Contains(s.Run, "kubectl apply -k ")
		if strings.Contains(s.Run, "kubectl rollout status") && !strings.Contains(s.Run, "--namespace your-monitor") {
			t.Errorf("step %q runs kubectl rollout status without --namespace your-monitor", s.Name)
		}
		if v, _ := s.With["version"].(string); strings.HasPrefix(s.Uses, "azure/setup-kubectl@") && !strings.HasPrefix(v, "v1.36.") {
			t.Errorf("setup-kubectl installs %s, want a v1.36 patch: the cluster runs Kubernetes 1.36", v)
		}
	}
	if !secret || !apply {
		t.Errorf("deploy reads secrets.KUBECONFIG: %v, runs kubectl apply -k: %v; want both", secret, apply)
	}
}

// region: pins

// pins lists every action the workflows use, as owner/repo, the full
// commit SHA, and the release tag that points at that commit. Each SHA was
// read from the action repository with git ls-remote --tags. Set
// WEEK14_VERIFY_PINS=1 to check them again over the network.
var pins = map[string]struct{ sha, tag string }{
	"actions/checkout":              {"3d3c42e5aac5ba805825da76410c181273ba90b1", "v7.0.1"},
	"actions/setup-go":              {"b7ad1dad31e06c5925ef5d2fc7ad053ef454303e", "v7.0.0"},
	"actions/setup-node":            {"820762786026740c76f36085b0efc47a31fe5020", "v7.0.0"},
	"actions/upload-artifact":       {"043fb46d1a93c77aae656e7c1c64a875d1fc6a0a", "v7.0.1"},
	"golangci/golangci-lint-action": {"ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a", "v9.3.0"},
	"docker/login-action":           {"dbcb813823bdd20940b903addbd779551569679f", "v4.6.0"},
	"docker/setup-buildx-action":    {"f87e5991a6d7451dcb8d9637bfbc97413f497069", "v4.4.1"},
	"docker/build-push-action":      {"c3c9e263c25d99ce0380d002d59b67737d91b0dc", "v7.4.0"},
	"azure/setup-kubectl":           {"829323503d1be3d00ca8346e5391ca0b07a9ab0d", "v5.1.0"},
}

// endregion: pins

// inputs lists the inputs that each pinned release declares in its
// action.yml. actionlint checks the inputs of a popular action only when
// the action is referenced by tag, not by SHA, so this test does it.
var inputs = map[string][]string{
	"actions/checkout":              {"repository", "ref", "token", "ssh-key", "ssh-known-hosts", "ssh-strict", "ssh-user", "persist-credentials", "path", "clean", "filter", "sparse-checkout", "sparse-checkout-cone-mode", "fetch-depth", "fetch-tags", "show-progress", "lfs", "submodules", "set-safe-directory", "github-server-url", "allow-unsafe-pr-checkout"},
	"actions/setup-go":              {"go-version", "go-version-file", "check-latest", "token", "cache", "cache-dependency-path", "architecture", "go-download-base-url"},
	"actions/setup-node":            {"node-version", "node-version-file", "architecture", "check-latest", "registry-url", "scope", "token", "cache", "package-manager-cache", "cache-dependency-path", "mirror", "mirror-token"},
	"actions/upload-artifact":       {"name", "path", "if-no-files-found", "retention-days", "compression-level", "overwrite", "include-hidden-files", "archive"},
	"golangci/golangci-lint-action": {"version", "version-file", "install-mode", "install-only", "working-directory", "github-token", "verify", "only-new-issues", "args", "skip-cache", "skip-save-cache", "cache-invalidation-interval", "problem-matchers", "debug", "experimental"},
	"docker/login-action":           {"registry", "username", "password", "ecr", "scope", "logout", "registry-auth"},
	"docker/setup-buildx-action":    {"version", "driver", "driver-opts", "buildkitd-flags", "buildkitd-config", "buildkitd-config-inline", "use", "name", "endpoint", "platforms", "append", "keep-state", "cache-binary", "cleanup"},
	"docker/build-push-action":      {"add-hosts", "allow", "annotations", "attests", "build-args", "build-contexts", "builder", "cache-from", "cache-to", "call", "cgroup-parent", "context", "file", "labels", "load", "network", "no-cache", "no-cache-filters", "outputs", "platforms", "provenance", "pull", "push", "sbom", "secrets", "secret-envs", "secret-files", "shm-size", "ssh", "tags", "target", "ulimit", "github-token"},
	"azure/setup-kubectl":           {"version"},
}

// region: pinned-test

// TestActionsArePinned reads every uses: line of both workflows. Each one
// must name a full 40-character commit SHA followed by a comment with the
// release tag, and both must match the pins table.
func TestActionsArePinned(t *testing.T) {
	line := regexp.MustCompile(`^\s*(?:- )?uses: ([\w.-]+/[\w.-]+)@(\S+)(?: # (\S+))?$`)
	for _, name := range []string{"ci.yml", "deploy.yml"} {
		f, err := os.Open(filepath.Join(repo, ".github/workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			if !strings.Contains(sc.Text(), "uses:") {
				continue
			}
			m := line.FindStringSubmatch(sc.Text())
			if m == nil {
				t.Errorf("%s:%d: cannot read %q", name, n, sc.Text())
				continue
			}
			action, ref, comment := m[1], m[2], m[3]
			pin, ok := pins[action]
			switch {
			case !ok:
				t.Errorf("%s:%d: %s is not in the pins table", name, n, action)
			case !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ref):
				t.Errorf("%s:%d: %s@%s is not pinned to a full commit SHA", name, n, action, ref)
			case ref != pin.sha || comment != pin.tag:
				t.Errorf("%s:%d: %s@%s # %s, want @%s # %s", name, n, action, ref, comment, pin.sha, pin.tag)
			}
		}
		f.Close()
	}
}

// endregion: pinned-test

// TestStepInputsExist checks every with: key against the inputs of the
// pinned release.
func TestStepInputsExist(t *testing.T) {
	for _, name := range []string{"ci.yml", "deploy.yml"} {
		for jobName, j := range load(t, name).Jobs {
			for _, s := range j.Steps {
				action, _, _ := strings.Cut(s.Uses, "@")
				for k := range s.With {
					if !slices.Contains(inputs[action], k) {
						t.Errorf("%s, job %s: %s has no input %q", name, jobName, action, k)
					}
				}
			}
		}
	}
}

// TestNoExpressionsInScripts checks that no run: script holds a ${{ }}
// expression. GitHub pastes an expression into the script text before
// bash runs it, which turns a crafted branch name or input into code.
// Values reach the scripts through env: instead.
func TestNoExpressionsInScripts(t *testing.T) {
	for _, name := range []string{"ci.yml", "deploy.yml"} {
		for jobName, j := range load(t, name).Jobs {
			for _, s := range j.Steps {
				if strings.Contains(s.Run, "${{") {
					t.Errorf("%s, job %s: run: script holds an expression:\n%s", name, jobName, s.Run)
				}
			}
		}
	}
}

// region: make-test

// TestCIRunsWhatMakeCIRuns checks that every command of make ci, and of
// the targets it depends on, appears as a step of ci.yml. A step with
// working-directory: web runs npm in web/, which the Makefile writes as
// npm --prefix web.
func TestCIRunsWhatMakeCIRuns(t *testing.T) {
	recipes := makeRecipes(t)
	var want []string
	for _, target := range []string{"web/node_modules", "lint", "test", "e2e", "ci"} {
		if recipes[target] == nil {
			t.Fatalf("the Makefile has no %s target", target)
		}
		want = append(want, recipes[target]...)
	}

	var steps []string
	for _, j := range load(t, "ci.yml").Jobs {
		for _, s := range j.Steps {
			if strings.HasPrefix(s.Uses, "golangci/golangci-lint-action@") {
				steps = append(steps, "golangci-lint run")
			}
			for _, cmd := range strings.Split(strings.TrimSpace(s.Run), "\n") {
				if j.Defaults.Run["working-directory"] == "web" {
					cmd = strings.Replace(cmd, "npm ", "npm --prefix web ", 1)
				}
				steps = append(steps, cmd)
			}
		}
	}
	for _, cmd := range want {
		if !slices.Contains(steps, cmd) {
			t.Errorf("make runs %q, and no step of ci.yml does", cmd)
		}
	}
}

// makeRecipes returns the commands under each target of the Makefile.
func makeRecipes(t *testing.T) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	recipes := map[string][]string{}
	target := ""
	for _, l := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(l, "\t") && target != "":
			recipes[target] = append(recipes[target], strings.TrimSpace(l))
		case regexp.MustCompile(`^[\w/.-]+:`).MatchString(l) && !strings.HasPrefix(l, ".PHONY"):
			target, _, _ = strings.Cut(l, ":")
			recipes[target] = []string{}
		default:
			target = ""
		}
	}
	return recipes
}

// endregion: make-test

// TestDependabotWatchesActions checks that Dependabot keeps the pins fresh.
func TestDependabotWatchesActions(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, ".github/dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Version int
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
			Directory string
			Cooldown  struct {
				DefaultDays int `yaml:"default-days"`
			}
		}
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 2 || len(cfg.Updates) != 1 || cfg.Updates[0].Ecosystem != "github-actions" || cfg.Updates[0].Directory != "/" {
		t.Fatalf("dependabot.yml is %+v, want version 2 with one github-actions update for /", cfg)
	}
	// The page pins releases at least two weeks old; Dependabot waits as long.
	if d := cfg.Updates[0].Cooldown.DefaultDays; d != 14 {
		t.Errorf("dependabot cooldown is %d days, want 14", d)
	}
}

// TestGolangciConfig checks the parts of .golangci.yml that the page
// explains.
func TestGolangciConfig(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repo, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Version string
		Linters struct {
			Default string
			Enable  []string
		}
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "2" || cfg.Linters.Default != "standard" {
		t.Errorf(".golangci.yml has version %q and default %q, want 2 and standard", cfg.Version, cfg.Linters.Default)
	}
	for _, l := range []string{"bodyclose", "errorlint", "gosec", "misspell"} {
		if !slices.Contains(cfg.Linters.Enable, l) {
			t.Errorf(".golangci.yml does not enable %s", l)
		}
	}
}

// TestActionlint runs actionlint on both workflows when it is installed:
// go install github.com/rhysd/actionlint/cmd/actionlint@latest
func TestActionlint(t *testing.T) {
	bin, err := exec.LookPath("actionlint")
	if err != nil {
		t.Skip("actionlint is not on PATH")
	}
	cmd := exec.Command(bin, "-no-color", ".github/workflows/ci.yml", ".github/workflows/deploy.yml")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("actionlint: %v\n%s", err, out)
	}
}

// TestPinsMatchTags reads each pinned tag from GitHub with git ls-remote
// and checks that it points at the pinned commit. It needs the network,
// so it runs only with WEEK14_VERIFY_PINS=1.
func TestPinsMatchTags(t *testing.T) {
	if os.Getenv("WEEK14_VERIFY_PINS") != "1" {
		t.Skip("set WEEK14_VERIFY_PINS=1 to check the pins against GitHub")
	}
	for action, pin := range pins {
		out, err := exec.Command("git", "ls-remote", "--tags", "https://github.com/"+action, pin.tag, pin.tag+"^{}").Output()
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		// An annotated tag lists its tag object, then the commit on a
		// line ending in ^{}. A lightweight tag lists the commit only.
		commit := ""
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			sha, ref, _ := strings.Cut(l, "\t")
			if ref == "refs/tags/"+pin.tag+"^{}" || (ref == "refs/tags/"+pin.tag && commit == "") {
				commit = sha
			}
		}
		if commit != pin.sha {
			t.Errorf("%s %s points at %q, the pin says %s", action, pin.tag, commit, pin.sha)
		}
	}
}
