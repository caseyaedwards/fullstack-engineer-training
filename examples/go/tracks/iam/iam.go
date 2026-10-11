// Package iam evaluates a small access policy the way cloud IAM systems do:
// deny by default, allow when a statement matches, and let an explicit deny
// beat every allow.
package iam

import "strings"

// region: types

// Effect says what a matching statement does.
type Effect string

const (
	Allow Effect = "Allow"
	Deny  Effect = "Deny"
)

// Statement allows or denies a set of actions on a set of resources.
// A pattern that ends in * matches every value that starts with the rest.
type Statement struct {
	Effect    Effect
	Actions   []string
	Resources []string
}

// endregion: types

// region: allowed

// Allowed reports whether policy allows action on resource.
func Allowed(policy []Statement, action, resource string) bool {
	allowed := false
	for _, s := range policy {
		if !matchAny(s.Actions, action) || !matchAny(s.Resources, resource) {
			continue
		}
		switch s.Effect {
		case Deny:
			return false
		case Allow:
			allowed = true
		}
	}
	return allowed
}

// endregion: allowed

// region: match

func matchAny(patterns []string, value string) bool {
	for _, p := range patterns {
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			if strings.HasPrefix(value, prefix) {
				return true
			}
		} else if p == value {
			return true
		}
	}
	return false
}

// endregion: match
