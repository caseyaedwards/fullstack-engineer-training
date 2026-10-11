// Package week12 checks the Week 12 exercise itself. testdata/teammate.patch
// is the branch the reader applies with git am: four commits that pass the
// Week 11 tests and carry one bug each. testdata/planted holds every file
// the patch touches, as the patch leaves it. your-monitor is the fixed
// tree, where the reader ends the week.
//
// The tests build the planted tree as an overlay on the fixed one: the go
// command reads the planted files in place of the fixed ones, without
// copying the module anywhere.
package week12

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// regressionTests are the tests the reader adds or fixes this week, one
// per bug, by the file that holds each.
var regressionTests = map[string]string{
	"internal/api/headers_test.go": "TestSecureHeaders",
	"internal/api/cache_test.go":   "TestAnswersAreNotStored",
	"internal/probe/tls_test.go":   "TestTLSRowIsTheHandshakeOnly",
	"cmd/worker/poll_test.go":      "TestIdleWorkersWaitForPoll",
}

// marker matches the region lines that CodeFrom reads. The page never
// shows them, which leaves them out of the reader's files.
var marker = regexp.MustCompile(`(?m)^[ \t]*(//|#|--)[ \t]*(region|endregion):[ \t]*\S+[ \t]*\n`)

func abs(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// touched lists the files the patch changes, from its diff headers.
func touched(t *testing.T) []string {
	t.Helper()
	patch, err := os.ReadFile("testdata/teammate.patch")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, m := range regexp.MustCompile(`(?m)^diff --git a/(\S+) b/`).FindAllStringSubmatch(string(patch), -1) {
		if !slices.Contains(files, m[1]) {
			files = append(files, m[1])
		}
	}
	slices.Sort(files)
	return files
}

func TestPlantedFilesAreThePatchFiles(t *testing.T) {
	var planted []string
	err := filepath.WalkDir("testdata/planted", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel("testdata/planted", path)
			planted = append(planted, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(planted)
	if got := touched(t); !slices.Equal(got, planted) {
		t.Errorf("the patch touches %v, testdata/planted holds %v", got, planted)
	}
}

// TestPatchAppliesWithoutRegionMarkers undoes the patch with git apply -R
// on the planted files, stripped of region markers as the reader has them,
// then applies it again. Both work only when every hunk matches, which
// keeps the patch and testdata/planted in step.
func TestPatchAppliesWithoutRegionMarkers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, f := range touched(t) {
		b, err := os.ReadFile(filepath.Join("testdata/planted", f))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, marker.ReplaceAll(b, nil), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	patch := abs(t, "testdata/teammate.patch")
	for _, args := range [][]string{{"apply", "-R", patch}, {"apply", patch}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// The other way round: the Week 11 tree, once it is on main, takes the
	// patch as the reader's copy does.
	week11 := "../week11/your-monitor"
	if _, err := os.Stat(week11); err != nil {
		return
	}
	for _, f := range touched(t) {
		b, err := os.ReadFile(filepath.Join(week11, f))
		if os.IsNotExist(err) {
			continue // a file the patch adds
		}
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, f), marker.ReplaceAll(b, nil), 0o644)
	}
	for _, f := range []string{"internal/api/headers.go", "internal/api/headers_test.go", "internal/api/cache.go"} {
		os.Remove(filepath.Join(dir, f))
	}
	cmd := exec.Command("git", "apply", "--check", patch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the patch does not apply to the Week 11 tree: %v\n%s", err, out)
	}
}

// goTest runs go test on the packages the patch touches, with the planted
// files in place of the fixed ones. keepRegression keeps the regression
// tests of the fixed tree; without it, the overlay removes the ones the
// planted tree does not have, which leaves the Week 11 suite plus the
// teammate's test. GOFLAGS carries the overlay, which means a test that
// runs go build itself builds the planted code too.
func goTest(t *testing.T, keepRegression bool, args ...string) (string, error) {
	t.Helper()
	replace := map[string]string{}
	pkgs := map[string]bool{}
	for _, f := range touched(t) {
		pkgs["./your-monitor/"+filepath.Dir(f)] = true
		if keepRegression && regressionTests[f] != "" {
			continue // the fixed version of a test the teammate wrote
		}
		replace[abs(t, filepath.Join("your-monitor", f))] = abs(t, filepath.Join("testdata/planted", f))
	}
	if !keepRegression {
		for f := range regressionTests {
			if _, err := os.Stat(filepath.Join("testdata/planted", f)); os.IsNotExist(err) {
				replace[abs(t, filepath.Join("your-monitor", f))] = "" // not in the planted tree
			}
		}
	}
	overlay, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "overlay.json")
	if err := os.WriteFile(path, overlay, 0o644); err != nil {
		t.Fatal(err)
	}
	cmdArgs := append([]string{"test", "-count=1"}, args...)
	for p := range pkgs {
		cmdArgs = append(cmdArgs, p)
	}
	cmd := exec.Command("go", cmdArgs...)
	cmd.Env = append(os.Environ(), "GOFLAGS=-overlay="+path)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return out.String(), err
}

// TestPlantedBugsPassTheWeek11Tests is the start of the exercise: after
// git am, every test the reader has passes.
func TestPlantedBugsPassTheWeek11Tests(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on three packages")
	}
	out, err := goTest(t, false)
	if err != nil {
		t.Fatalf("the Week 11 tests fail on the planted tree: %v\n%s", err, out)
	}
}

// TestRegressionTestsCatchThePlantedBugs runs the four regression tests
// against the planted code. Each must fail there. The normal run of
// ./your-monitor/... shows that each passes on the fixed code.
func TestRegressionTestsCatchThePlantedBugs(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on three packages")
	}
	var names []string
	for _, name := range regressionTests {
		names = append(names, name)
	}
	slices.Sort(names)
	out, err := goTest(t, true, "-run", "^("+strings.Join(names, "|")+")$")
	if err == nil {
		t.Fatalf("the regression tests pass on the planted code:\n%s", out)
	}
	for _, name := range names {
		if !strings.Contains(out, "--- FAIL: "+name+" ") {
			t.Errorf("%s does not fail on the planted code:\n%s", name, out)
		}
	}
}
