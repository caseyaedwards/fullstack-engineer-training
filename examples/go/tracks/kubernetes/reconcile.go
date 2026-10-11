// Package kubernetes holds the example of the Kubernetes track: Reconcile,
// one pass of a control loop like the one that keeps a Deployment running.
// It compares the desired state with the pods that run, and returns the
// actions that close the gap. It does not act itself: in a cluster, the
// controller sends the actions to the API server, and the kubelets carry
// them out.
package kubernetes

import (
	"fmt"
	"slices"
	"strings"
)

// region: types

// Spec is the desired state: how many pods, made from which template.
type Spec struct {
	Replicas int
	Template string
}

// Pod is one running pod, and the template it was made from.
type Pod struct {
	Name, Template string
}

// Action is one change to make: Kind is "create" or "delete", and Pod
// names the pod.
type Action struct {
	Kind, Pod string
}

// endregion: types

// region: reconcile

// Reconcile returns the actions that turn actual into desired. Pods made
// from another template are outdated, and each one gets deleted. Pods made
// from the desired template count toward Replicas: missing ones get
// created, extra ones deleted. Creates come first, the way a rolling update
// with maxUnavailable 0 starts new pods before it stops old ones. Run on a
// cluster that already matches, it returns no actions.
func Reconcile(desired Spec, actual []Pod) []Action {
	var current, outdated []Pod
	taken := map[string]bool{}
	for _, p := range actual {
		taken[p.Name] = true
		if p.Template == desired.Template {
			current = append(current, p)
		} else {
			outdated = append(outdated, p)
		}
	}
	byName := func(a, b Pod) int { return strings.Compare(a.Name, b.Name) }
	slices.SortFunc(current, byName)
	slices.SortFunc(outdated, byName)

	want := max(desired.Replicas, 0)
	var actions []Action
	missing := want - len(current)
	for n := 1; missing > 0; n++ {
		name := fmt.Sprintf("%s-%d", desired.Template, n)
		if !taken[name] {
			actions = append(actions, Action{"create", name})
			missing--
		}
	}
	for _, p := range outdated {
		actions = append(actions, Action{"delete", p.Name})
	}
	if len(current) > want {
		for _, p := range current[want:] {
			actions = append(actions, Action{"delete", p.Name})
		}
	}
	return actions
}

// endregion: reconcile
