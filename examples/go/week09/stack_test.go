// Package week09 holds the tests of the Week 9 build files. The code
// itself lives in your-monitor/, laid out as the reader's repository.
package week09

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestEveryProgramHasAnImage checks that the Dockerfile builds every
// program under cmd/ as a static binary with a target of its own, and that
// compose.yaml starts each one from that target.
func TestEveryProgramHasAnImage(t *testing.T) {
	dockerfile := read(t, "your-monitor/Dockerfile")
	compose := read(t, "your-monitor/compose.yaml")
	m := regexp.MustCompile(`(?m)^RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ (.+)$`).FindStringSubmatch(dockerfile)
	if m == nil {
		t.Fatal("no go build line in the Dockerfile")
	}
	built := strings.Fields(m[1])

	dirs, err := os.ReadDir("your-monitor/cmd")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		name := d.Name()
		if !strings.Contains(" "+m[1]+" ", " ./cmd/"+name+" ") {
			t.Errorf("the Dockerfile does not build ./cmd/%s", name)
		}
		if !strings.Contains(dockerfile, "FROM runtime AS "+name+"\nCOPY --from=build /out/"+name+" /"+name+"\n") {
			t.Errorf("the Dockerfile has no %s target", name)
		}
		if !regexp.MustCompile(`(?m)^  ` + name + `:\n    build:\n      context: \.\n      target: ` + name + `\n`).MatchString(compose) {
			t.Errorf("compose.yaml has no %s service built from the %s target", name, name)
		}
	}

	out := t.TempDir()
	cmd := exec.Command("go", append([]string{"build", "-trimpath", "-ldflags=-s -w", "-o", out + "/"}, built...)...)
	cmd.Dir = "your-monitor"
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", cmd.Args, err, b)
	}
	for _, pkg := range built {
		f, err := elf.Open(filepath.Join(out, filepath.Base(pkg)))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range f.Progs {
			if p.Type == elf.PT_INTERP {
				t.Errorf("%s is not static", pkg)
			}
		}
		f.Close()
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
