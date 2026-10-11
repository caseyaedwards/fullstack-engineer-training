// Package week06 holds the tests of the Week 6 repository layout. The code
// itself lives in your-monitor/, laid out as the reader's repository.
package week06

import (
	"bytes"
	"debug/elf"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestMovedCodeOnlyChangesImportPaths checks that moving the Week 5 code
// into cmd/worker and internal/ changes nothing but import paths and the
// probe package name.
func TestMovedCodeOnlyChangesImportPaths(t *testing.T) {
	rewrite := strings.NewReplacer(
		`"fsp/examples/week05/your-monitor/check"`, `"fsp/examples/week06/your-monitor/internal/check"`,
		`"fsp/examples/week05/your-monitor/pool"`, `"fsp/examples/week06/your-monitor/internal/pool"`,
		`probe "fsp/examples/week04"`, `"fsp/examples/week06/your-monitor/internal/probe"`,
		"package week04\n", "package probe\n",
		"// Package week04 ", "// Package probe ",
	)
	moves := map[string]string{
		"../week05/your-monitor/main.go":        "your-monitor/cmd/worker/main.go",
		"../week05/your-monitor/run_test.go":    "your-monitor/cmd/worker/run_test.go",
		"../week05/your-monitor/signal_test.go": "your-monitor/cmd/worker/signal_test.go",
		"../week05/your-monitor/check/check.go": "your-monitor/internal/check/check.go",
		"../week05/your-monitor/check/list.go":  "your-monitor/internal/check/list.go",
		"../week05/your-monitor/pool/pool.go":   "your-monitor/internal/pool/pool.go",
		"../week04/trace.go":                    "your-monitor/internal/probe/probe.go",
	}
	for from, to := range moves {
		old, err := format.Source([]byte(rewrite.Replace(read(t, from))))
		if err != nil {
			t.Fatalf("%s: %v", from, err)
		}
		// The worker gets a new doc comment above its package clause.
		if got, want := fromPackage(read(t, to)), fromPackage(string(old)); got != want {
			t.Errorf("%s differs from %s by more than its import paths", to, from)
		}
	}
}

// region: internal

// TestInternalIsPrivate builds testdata/outside, a program outside
// your-monitor that imports your-monitor/internal/check. The go command
// must refuse it.
func TestInternalIsPrivate(t *testing.T) {
	out, err := exec.Command("go", "build", "-o", os.DevNull, "./testdata/outside").CombinedOutput()
	if err == nil {
		t.Fatal("go build succeeded, want it to refuse the internal import")
	}
	want := "use of internal package fsp/examples/week06/your-monitor/internal/check not allowed"
	if !strings.Contains(string(out), want) {
		t.Errorf("go build output:\n%s\nwant it to contain %q", out, want)
	}
}

// endregion: internal

// TestDockerfileBuildsStaticBinaries runs the go build command from the
// Dockerfile, for Linux, and checks that each binary is static: it names
// no dynamic loader, which an image built FROM scratch does not have.
func TestDockerfileBuildsStaticBinaries(t *testing.T) {
	dockerfile := read(t, "your-monitor/Dockerfile")
	m := regexp.MustCompile(`(?m)^RUN CGO_ENABLED=0 (go build .*) -o /out/ (.+)$`).FindStringSubmatch(dockerfile)
	if m == nil {
		t.Fatal("no RUN CGO_ENABLED=0 go build ... -o /out/ line in the Dockerfile")
	}
	buildArgs, packages := m[1], strings.Fields(m[2])

	// make build must use the same flags as the Dockerfile.
	if makefile := read(t, "your-monitor/Makefile"); !strings.Contains(makefile, "CGO_ENABLED=0 "+buildArgs+" -o bin/") {
		t.Errorf("the Makefile build target does not use the Dockerfile flags %q", buildArgs)
	}

	out := t.TempDir()
	args := append(shellFields(t, buildArgs)[1:], "-o", out+string(filepath.Separator))
	cmd := exec.Command("go", append(args, packages...)...)
	cmd.Dir = "your-monitor"
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", cmd.Args, err, b)
	}
	for _, pkg := range packages {
		bin := filepath.Join(out, filepath.Base(pkg))
		f, err := elf.Open(bin)
		if err != nil {
			t.Fatalf("%s: %v", bin, err)
		}
		defer f.Close()
		for _, p := range f.Progs {
			if p.Type == elf.PT_INTERP {
				t.Errorf("%s names a dynamic loader; it would not start in a scratch image", pkg)
			}
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fromPackage returns src from its package clause on, with region
// markers removed.
func fromPackage(src string) string {
	src = src[strings.Index(src, "\npackage ")+1:]
	var b bytes.Buffer
	for _, line := range strings.SplitAfter(src, "\n") {
		if !regexp.MustCompile(`^\s*// (end)?region: `).MatchString(line) {
			b.WriteString(line)
		}
	}
	return b.String()
}

// shellFields splits a command line on spaces, removing the double quotes
// the shell removes: -ldflags="-s -w" becomes -ldflags=-s -w.
func shellFields(t *testing.T, s string) []string {
	t.Helper()
	var fields []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quoted {
		t.Fatalf("unbalanced quotes in %q", s)
	}
	return append(fields, cur.String())
}
