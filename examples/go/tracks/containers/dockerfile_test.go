// Package containers holds the tests of the Containers track example: a
// two-stage Dockerfile that builds hello/main.go with the Go toolchain and
// copies the binary alone into a distroless runtime image. Docker does not
// run here, so the tests read the Dockerfile as text, and build and run
// hello with the go command, the way the build stage does.
package containers

import (
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// region: checks

// stage is one FROM line of a Dockerfile and the instructions under it.
type stage struct {
	image, name  string
	instructions []string
}

// parse splits a Dockerfile into stages. It joins lines that end in a
// backslash and drops comments, which is enough for the files of this site.
func parse(src string) []stage {
	src = strings.ReplaceAll(src, "\\\n", " ")
	var stages []stage
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if strings.EqualFold(fields[0], "FROM") {
			args := slices.DeleteFunc(fields[1:], func(f string) bool { return strings.HasPrefix(f, "--") })
			s := stage{image: args[0]}
			if len(args) == 3 && strings.EqualFold(args[1], "AS") {
				s.name = args[2]
			}
			stages = append(stages, s)
			continue
		}
		if len(stages) > 0 {
			last := &stages[len(stages)-1]
			last.instructions = append(last.instructions, line)
		}
	}
	return stages
}

// problems returns every rule the Dockerfile in src breaks.
func problems(src string) []string {
	stages := parse(src)
	if len(stages) != 2 {
		return []string{fmt.Sprintf("the Dockerfile has %d FROM stages, want 2: build and runtime", len(stages))}
	}
	var out []string
	names := map[string]bool{}
	for _, s := range stages {
		if !names[s.image] { // FROM an earlier stage needs no tag
			out = append(out, tagProblems(s.image)...)
		}
		names[s.name] = true
	}

	build, final := stages[0], stages[1]
	var built bool
	for _, in := range build.instructions {
		if strings.HasPrefix(in, "RUN ") && strings.Contains(in, "go build") {
			built = true
			if !strings.Contains(in, "CGO_ENABLED=0") {
				out = append(out, "the go build line does not set CGO_ENABLED=0")
			}
		}
	}
	if !built {
		out = append(out, "the build stage runs no go build")
	}

	user := ""
	for _, in := range final.instructions {
		if f := strings.Fields(in); strings.EqualFold(f[0], "USER") && len(f) > 1 {
			user = f[1]
		}
	}
	switch {
	case user == "" && !strings.HasSuffix(final.image, ":nonroot"):
		out = append(out, "the runtime stage runs as root: no USER line and no nonroot tag")
	case user == "root" || user == "0" || strings.HasPrefix(user, "0:") || strings.HasPrefix(user, "root:"):
		out = append(out, "the runtime stage sets USER "+user)
	}
	slices.Sort(out)
	return out
}

// tagProblems checks that an image reference names a version: a tag other
// than latest, or a digest. A reference without either means latest.
func tagProblems(image string) []string {
	if strings.Contains(image, "@sha256:") {
		return nil
	}
	name := image[strings.LastIndex(image, "/")+1:]
	_, tag, ok := strings.Cut(name, ":")
	switch {
	case !ok:
		return []string{image + " has no tag, which means latest"}
	case tag == "latest":
		return []string{image + " uses the latest tag"}
	}
	return nil
}

// endregion: checks

func readDockerfile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// region: dockerfile-test

// TestDockerfile checks the Dockerfile itself: it must break no rule.
func TestDockerfile(t *testing.T) {
	for _, p := range problems(readDockerfile(t)) {
		t.Error("Dockerfile: " + p)
	}
}

// endregion: dockerfile-test

// TestChecksCatchMistakes breaks a copy of the Dockerfile once per rule
// and expects that rule, and only that rule, to fail.
func TestChecksCatchMistakes(t *testing.T) {
	src := readDockerfile(t)
	tests := []struct {
		name, old, new, want string
	}{
		{"latest tag", "golang:1.24 ", "golang:latest ", "golang:latest uses the latest tag"},
		{"no tag", "golang:1.24 ", "golang ", "golang has no tag, which means latest"},
		{"cgo", "CGO_ENABLED=0 go build", "go build", "the go build line does not set CGO_ENABLED=0"},
		{"root", "USER 65532:65532", "USER root", "the runtime stage sets USER root"},
		{"latest runtime tag", "static-debian12:nonroot", "static-debian12:latest", "gcr.io/distroless/static-debian12:latest uses the latest tag"},
		{"one stage", "FROM gcr.io/distroless/static-debian12:nonroot", "", "the Dockerfile has 1 FROM stages, want 2: build and runtime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(src, tt.old) {
				t.Fatalf("the Dockerfile holds no %q to replace", tt.old)
			}
			got := problems(strings.Replace(src, tt.old, tt.new, 1))
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("problems = %q, want [%q]", got, tt.want)
			}
		})
	}

	// Without a USER line, the runtime stage needs the nonroot tag.
	noUser := regexp.MustCompile(`(?m)^USER .*\n`).ReplaceAllString(src, "")
	if got := problems(noUser); len(got) != 0 {
		t.Errorf("the nonroot tag without a USER line gives %q, want none", got)
	}
	noUser = strings.Replace(noUser, "static-debian12:nonroot", "static-debian12:debug", 1)
	if want := []string{"the runtime stage runs as root: no USER line and no nonroot tag"}; !slices.Equal(problems(noUser), want) {
		t.Errorf("a root base without a USER line gives %q, want %q", problems(noUser), want)
	}
	if got := problems(strings.Replace(src, "golang:1.24 ", "golang:1.24@sha256:"+strings.Repeat("ab", 32)+" ", 1)); len(got) != 0 {
		t.Errorf("a digest gives %q, want none", got)
	}
}

// region: hello-test

// TestHelloBuildsStatic builds hello the way the build stage does, runs it,
// and checks its output. On Linux, it also checks that the binary is static:
// an ELF file with no interpreter, which is the dynamic loader that a
// binary linked against the C library asks the kernel for. The distroless
// static image has no C library and no loader.
func TestHelloBuildsStatic(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "hello")
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", bin, ".")
	build.Dir = "hello"
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running hello: %v", err)
	}
	if got, want := string(out), "hello from a distroless container\n"; got != want {
		t.Errorf("hello prints %q, want %q", got, want)
	}

	if runtime.GOOS != "linux" {
		return
	}
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			t.Error("hello asks for a dynamic loader: it is not a static binary")
		}
	}
}

// endregion: hello-test

// TestDockerBuild builds the image and runs it when TRACKS_DOCKER=1 and a
// Docker daemon answers. It pulls golang:1.24 and the distroless image over
// the network, which is why it does not run by default.
func TestDockerBuild(t *testing.T) {
	if os.Getenv("TRACKS_DOCKER") != "1" {
		t.Skip("set TRACKS_DOCKER=1 to build and run the image with Docker")
	}
	tag := "fsp-tracks-hello:test"
	if out, err := exec.Command("docker", "build", "-t", tag, ".").CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, out)
	}
	out, err := exec.Command("docker", "run", "--rm", tag).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	if got, want := string(out), "hello from a distroless container\n"; got != want {
		t.Errorf("the container prints %q, want %q", got, want)
	}
	user, err := exec.Command("docker", "image", "inspect", "--format", "{{.Config.User}}", tag).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(user)); got != "65532:65532" {
		t.Errorf("the image runs as user %q, want 65532:65532", got)
	}
}
