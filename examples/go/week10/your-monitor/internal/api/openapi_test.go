package api

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// region: openapi-test

// TestOpenAPIMatchesRoutes reads api/openapi.yaml and checks that it
// documents every route the handler serves, and no route it does not.
// It reads two levels of the file by their indentation: a path at two
// spaces under paths:, and a method at four spaces under each path.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	spec, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pathLine := regexp.MustCompile(`^  (/\S*):$`)
	methodLine := regexp.MustCompile(`^    (get|put|post|patch|delete):$`)

	var documented []string
	path := ""
	for _, line := range strings.Split(string(spec), "\n") {
		if m := pathLine.FindStringSubmatch(line); m != nil {
			path = m[1]
		} else if m := methodLine.FindStringSubmatch(line); m != nil && path != "" {
			documented = append(documented, strings.ToUpper(m[1])+" "+path)
		}
	}

	var served []string
	for pattern := range (&handler{}).routes() {
		served = append(served, pattern)
	}
	slices.Sort(documented)
	slices.Sort(served)
	if !slices.Equal(documented, served) {
		t.Errorf("openapi.yaml documents:\n  %s\nthe handler serves:\n  %s",
			strings.Join(documented, "\n  "), strings.Join(served, "\n  "))
	}
}

// endregion: openapi-test
