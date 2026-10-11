package week14

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// runStep runs the run: script of the named step the way a GitHub-hosted
// Linux runner runs a step without shell:, with bash -e, in dir and with
// the step env plus env.
func runStep(t *testing.T, file, jobName, stepName, dir string, env map[string]string) (string, error) {
	t.Helper()
	out, err := exec.Command("bash", "-c", "echo ${BASH_VERSINFO[0]}").Output()
	if err != nil || strings.TrimSpace(string(out)) < "4" {
		// ${GITHUB_REPOSITORY,,} needs bash 4 or newer. macOS ships 3.2.
		t.Skip("bash 4 or newer is not installed")
	}
	var s *step
	for _, st := range load(t, file).Jobs[jobName].Steps {
		if st.Name == stepName {
			s = &st
		}
	}
	if s == nil {
		t.Fatalf("%s has no step %q in job %s", file, stepName, jobName)
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-c", s.Run)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GITHUB_REPOSITORY=Ada-Lovelace/your-monitor")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// region: image-name-test

// TestImageNameIsLowercase runs the Name the image step for an owner with
// capitals. GHCR refuses an image name with capitals.
func TestImageNameIsLowercase(t *testing.T) {
	ghEnv := filepath.Join(t.TempDir(), "github_env")
	if out, err := runStep(t, "ci.yml", "images", "Name the image", t.TempDir(), map[string]string{
		"TARGET": "api", "GITHUB_ENV": ghEnv,
	}); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, err := os.ReadFile(ghEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "IMAGE=ghcr.io/ada-lovelace/your-monitor/api\n"; got != want {
		t.Errorf("GITHUB_ENV holds %q, want %q", got, want)
	}
}

// endregion: image-name-test

// region: deploy-test

const commit = "0123456789abcdef0123456789abcdef01234567"

// TestDeployPicksTheImagesOfOneCommit runs the Pick the images step and
// reads the kustomization it writes: the prod overlay, with each image
// of the Compose project replaced by the GHCR image for the commit.
func TestDeployPicksTheImagesOfOneCommit(t *testing.T) {
	dir := t.TempDir()
	if out, err := runStep(t, "deploy.yml", "deploy", "Pick the images", dir, map[string]string{"SHA": commit}); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "deploy/k8s/release/kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Resources []string
		Images    []struct {
			Name    string
			NewName string `yaml:"newName"`
			NewTag  string `yaml:"newTag"`
		}
	}
	if err := yaml.Unmarshal(b, &k); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if len(k.Resources) != 1 || k.Resources[0] != "../overlays/prod" {
		t.Errorf("resources are %v, want [../overlays/prod]", k.Resources)
	}
	targets := []string{"api", "worker", "scheduler", "alerter", "web"}
	if len(k.Images) != len(targets) {
		t.Fatalf("%d images, want %d:\n%s", len(k.Images), len(targets), b)
	}
	for i, target := range targets {
		img := k.Images[i]
		if img.Name != "your-monitor-"+target || img.NewName != "ghcr.io/ada-lovelace/your-monitor/"+target || img.NewTag != commit {
			t.Errorf("image %d is %+v, want your-monitor-%s as ghcr.io/ada-lovelace/your-monitor/%s:%s", i, img, target, target, commit)
		}
	}
}

// TestDeployRefusesAnythingButASHA runs the same step with inputs that are
// not a full commit SHA. The step must fail before it writes a file.
func TestDeployRefusesAnythingButASHA(t *testing.T) {
	for _, sha := range []string{"", "main", commit[:7], strings.ToUpper(commit), commit + "; touch pwned", "$(touch pwned)"} {
		dir := t.TempDir()
		out, err := runStep(t, "deploy.yml", "deploy", "Pick the images", dir, map[string]string{"SHA": sha})
		if err == nil {
			t.Errorf("SHA %q: the step succeeded, want an error", sha)
		}
		if !strings.Contains(out, "::error::sha must be a full 40-character commit SHA") {
			t.Errorf("SHA %q: output %q holds no error annotation", sha, out)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Errorf("SHA %q: the step wrote %v", sha, entries)
		}
	}
}

// endregion: deploy-test
