package kubernetes

import (
	"slices"
	"testing"
)

// region: reconcile-test

// TestReconcile checks the actions for each difference between the
// desired state and the pods that run, then applies them and runs the loop
// a second time, which must find nothing left to do.
func TestReconcile(t *testing.T) {
	tests := []struct {
		name    string
		desired Spec
		actual  []Pod
		want    []Action
	}{
		{
			name:    "three wanted, one running",
			desired: Spec{Replicas: 3, Template: "api-v2"},
			actual:  []Pod{{"api-v2-1", "api-v2"}},
			want:    []Action{{"create", "api-v2-2"}, {"create", "api-v2-3"}},
		},
		{
			name:    "one wanted, three running",
			desired: Spec{Replicas: 1, Template: "api-v2"},
			actual:  []Pod{{"api-v2-1", "api-v2"}, {"api-v2-2", "api-v2"}, {"api-v2-3", "api-v2"}},
			want:    []Action{{"delete", "api-v2-2"}, {"delete", "api-v2-3"}},
		},
		{
			name:    "old template",
			desired: Spec{Replicas: 1, Template: "api-v2"},
			actual:  []Pod{{"api-v1-1", "api-v1"}},
			want:    []Action{{"create", "api-v2-1"}, {"delete", "api-v1-1"}},
		},
		{
			name:    "already right",
			desired: Spec{Replicas: 2, Template: "api-v2"},
			actual:  []Pod{{"api-v2-1", "api-v2"}, {"api-v2-2", "api-v2"}},
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Reconcile(tt.desired, tt.actual)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Reconcile = %v, want %v", got, tt.want)
			}
			after := apply(tt.actual, got, tt.desired.Template)
			if again := Reconcile(tt.desired, after); len(again) != 0 {
				t.Errorf("second pass over %v = %v, want no actions", after, again)
			}
		})
	}
}

// endregion: reconcile-test

// TestReconcileEdges covers a mix of old and new pods, scaling to zero, a
// negative count and a name that a new pod must not reuse.
func TestReconcileEdges(t *testing.T) {
	tests := []struct {
		name    string
		desired Spec
		actual  []Pod
		want    []Action
	}{
		{
			name:    "rollout halfway",
			desired: Spec{Replicas: 2, Template: "api-v2"},
			actual:  []Pod{{"api-v1-1", "api-v1"}, {"api-v2-1", "api-v2"}},
			want:    []Action{{"create", "api-v2-2"}, {"delete", "api-v1-1"}},
		},
		{
			name:    "scale to zero",
			desired: Spec{Replicas: 0, Template: "api-v2"},
			actual:  []Pod{{"api-v2-1", "api-v2"}, {"api-v1-1", "api-v1"}},
			want:    []Action{{"delete", "api-v1-1"}, {"delete", "api-v2-1"}},
		},
		{
			name:    "negative count means zero",
			desired: Spec{Replicas: -1, Template: "api-v2"},
			actual:  nil,
			want:    nil,
		},
		{
			name:    "a new name skips the names in use",
			desired: Spec{Replicas: 2, Template: "api-v2"},
			actual:  []Pod{{"api-v2-1", "api-v1"}},
			want:    []Action{{"create", "api-v2-2"}, {"create", "api-v2-3"}, {"delete", "api-v2-1"}},
		},
		{
			name:    "order of the pods does not matter",
			desired: Spec{Replicas: 1, Template: "api-v2"},
			actual:  []Pod{{"api-v2-3", "api-v2"}, {"api-v2-1", "api-v2"}},
			want:    []Action{{"delete", "api-v2-3"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Reconcile(tt.desired, tt.actual)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Reconcile = %v, want %v", got, tt.want)
			}
			after := apply(tt.actual, got, tt.desired.Template)
			if again := Reconcile(tt.desired, after); len(again) != 0 {
				t.Errorf("second pass over %v = %v, want no actions", after, again)
			}
		})
	}
}

// TestReconcileDoesNotChangeItsInput checks that Reconcile reads the pod
// list and leaves it as it was: the caller owns it.
func TestReconcileDoesNotChangeItsInput(t *testing.T) {
	actual := []Pod{{"api-v2-3", "api-v2"}, {"api-v1-1", "api-v1"}, {"api-v2-1", "api-v2"}}
	before := slices.Clone(actual)
	Reconcile(Spec{Replicas: 1, Template: "api-v2"}, actual)
	if !slices.Equal(actual, before) {
		t.Errorf("Reconcile changed its input to %v, was %v", actual, before)
	}
}

// apply plays the actions against a pod list, the way the kubelet and the
// API server carry them out in a cluster.
func apply(pods []Pod, actions []Action, template string) []Pod {
	out := slices.Clone(pods)
	for _, a := range actions {
		switch a.Kind {
		case "create":
			out = append(out, Pod{Name: a.Pod, Template: template})
		case "delete":
			out = slices.DeleteFunc(out, func(p Pod) bool { return p.Name == a.Pod })
		}
	}
	return out
}
