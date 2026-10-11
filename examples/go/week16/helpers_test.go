// Package week16 holds the tests of the Week 16 files that are not
// Kubernetes manifests: the deploy workflow and the VPS configuration.
// The files live in your-monitor/, laid out as the reader's repository.
// The tests of the manifests live next to them, in
// your-monitor/deploy/k8s/manifests_test.go, where the reader keeps them.
package week16

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"

	"go.yaml.in/yaml/v3"
)

const repo = "your-monitor"

// object holds the fields of a Kubernetes object that the tests read.
type object struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec yaml.Node `yaml:"spec"`
}

// decodeSpec decodes the spec of o into v.
func (o object) decodeSpec(t *testing.T, v any) {
	t.Helper()
	if err := o.Spec.Decode(v); err != nil {
		t.Fatalf("%s %s: %v", o.Kind, o.Metadata.Name, err)
	}
}

// render builds the Kustomize directory dir with kubectl kustomize, the
// way kubectl apply -k does. Without kubectl, the test fails in CI and
// skips elsewhere.
func render(t *testing.T, dir string) []object {
	t.Helper()
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("kubectl is not on PATH: CI must install it to render the overlays")
		}
		t.Skip("kubectl is not on PATH")
	}
	cmd := exec.Command(kubectl, "kustomize", dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("kubectl kustomize %s: %v\n%s", dir, err, stderr.String())
	}
	var objs []object
	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var o object
		err := dec.Decode(&o)
		if errors.Is(err, io.EOF) {
			return objs
		}
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		objs = append(objs, o)
	}
}

func ofKind(objs []object, kind string) []object {
	var out []object
	for _, o := range objs {
		if o.Kind == kind {
			out = append(out, o)
		}
	}
	return out
}
