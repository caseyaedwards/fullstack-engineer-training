// Package cicd holds the tests of ci.yml, the GitHub Actions workflow of
// the CI/CD track. GitHub Actions does not run here, so the tests read the
// workflow as YAML and check the rules the page names: it runs on pull
// requests, its token can only read, every action is pinned, the build job
// waits for the test job, and the test job runs go vet and the race
// detector.
package cicd

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// workflow holds the parts of a GitHub Actions workflow that the checks read.
type workflow struct {
	On          map[string]yaml.Node `yaml:"on"`
	Permissions yaml.Node            `yaml:"permissions"`
	Jobs        map[string]job       `yaml:"jobs"`
}

type job struct {
	Needs       yaml.Node `yaml:"needs"`
	Permissions yaml.Node `yaml:"permissions"`
	Steps       []step    `yaml:"steps"`
}

type step struct {
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

// region: checks

// pinned matches an action reference with a version tag that starts with v
// and a digit, or with a full 40-character commit SHA. A branch, such as
// @main, or no ref at all, does not match.
var pinned = regexp.MustCompile(`^[\w.-]+/[\w./-]+@(v\d[\w.-]*|[0-9a-f]{40})$`)

// problems returns every rule that the workflow in src breaks.
func problems(src []byte) []string {
	var w workflow
	if err := yaml.Unmarshal(src, &w); err != nil {
		return []string{err.Error()}
	}
	var out []string
	if _, ok := w.On["pull_request"]; !ok {
		out = append(out, "the workflow does not run on pull_request")
	}
	out = append(out, readOnly("the workflow", w.Permissions, true)...)
	for name, j := range w.Jobs {
		out = append(out, readOnly("job "+name, j.Permissions, false)...)
		for _, s := range j.Steps {
			if s.Uses != "" && !pinned.MatchString(s.Uses) {
				out = append(out, fmt.Sprintf("job %s: %s is not pinned to a version tag or a commit SHA", name, s.Uses))
			}
		}
	}
	if !slices.Contains(needs(w.Jobs["build"]), "test") {
		out = append(out, "job build does not need test")
	}
	for _, cmd := range []string{"go vet ./...", "go test -race ./..."} {
		if !runs(w.Jobs["test"], cmd) {
			out = append(out, "job test does not run "+cmd)
		}
	}
	slices.Sort(out)
	return out
}

// readOnly checks a permissions block: a mapping in which every scope is
// read or none. A missing block passes for a job, which then inherits the
// workflow block, and fails for the workflow, whose token then gets the
// repository default, which can be read and write.
func readOnly(where string, p yaml.Node, required bool) []string {
	switch {
	case p.Kind == 0 && !required:
		return nil
	case p.Kind == 0:
		return []string{where + " sets no permissions"}
	case p.Kind != yaml.MappingNode:
		return []string{fmt.Sprintf("%s sets permissions to %q, want a list of read scopes", where, p.Value)}
	}
	var out []string
	for i := 0; i+1 < len(p.Content); i += 2 {
		scope, level := p.Content[i].Value, p.Content[i+1].Value
		if level != "read" && level != "none" {
			out = append(out, fmt.Sprintf("%s grants %s: %s", where, scope, level))
		}
	}
	return out
}

// endregion: checks

// needs returns the jobs that j waits for. needs: takes one name or a list.
func needs(j job) []string {
	if j.Needs.Kind == yaml.ScalarNode {
		return []string{j.Needs.Value}
	}
	var list []string
	_ = j.Needs.Decode(&list)
	return list
}

// runs reports whether a run: script of j holds cmd as a whole line.
func runs(j job, cmd string) bool {
	for _, s := range j.Steps {
		for _, line := range strings.Split(s.Run, "\n") {
			if strings.TrimSpace(line) == cmd {
				return true
			}
		}
	}
	return false
}

// region: test

// TestWorkflow checks ci.yml itself: it must break no rule.
func TestWorkflow(t *testing.T) {
	src, err := os.ReadFile("ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems(src) {
		t.Error("ci.yml: " + p)
	}
}

// endregion: test

// TestChecksCatchMistakes feeds the checks a broken copy of ci.yml for
// each rule, and expects that rule, and only that rule, to fail. A check
// that passes everything proves nothing about ci.yml.
func TestChecksCatchMistakes(t *testing.T) {
	good, err := os.ReadFile("ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	src := string(good)
	tests := []struct {
		name, old, new, want string
	}{
		{"push only", "on:\n  pull_request:\n", "on:\n  push:\n", "the workflow does not run on pull_request"},
		{"write token", "  contents: read\n", "  contents: write\n", "the workflow grants contents: write"},
		{"write-all", "permissions:\n  contents: read\n", "permissions: write-all\n", `the workflow sets permissions to "write-all", want a list of read scopes`},
		{"no permissions", "permissions:\n  contents: read\n", "", "the workflow sets no permissions"},
		{"branch ref", "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a", "actions/upload-artifact@main", "job build: actions/upload-artifact@main is not pinned to a version tag or a commit SHA"},
		{"short SHA", "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a", "actions/upload-artifact@043fb46", "job build: actions/upload-artifact@043fb46 is not pinned to a version tag or a commit SHA"},
		{"no needs", "    needs: test\n", "", "job build does not need test"},
		{"no race", "go test -race ./...", "go test ./...", "job test does not run go test -race ./..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(src, tt.old) {
				t.Fatalf("ci.yml holds no %q to replace", tt.old)
			}
			got := problems([]byte(strings.Replace(src, tt.old, tt.new, 1)))
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("problems = %q, want [%q]", got, tt.want)
			}
		})
	}

	// A job may set its own read-only permissions, needs: takes a list, and a
	// version tag passes too.
	ok := strings.Replace(src, "    needs: test\n", "    needs: [test]\n    permissions:\n      contents: read\n", 1)
	ok = strings.Replace(ok, "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a", "actions/upload-artifact@v7", 1)
	if got := problems([]byte(ok)); len(got) != 0 {
		t.Errorf("a list in needs, a job token of its own and a tag pin give problems %q, want none", got)
	}
}
