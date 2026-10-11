package iam

import (
	"fmt"
	"testing"
)

// region: tests
func TestAllowed(t *testing.T) {
	editor := []Statement{
		{Effect: Allow, Actions: []string{"monitors:*"}, Resources: []string{"monitor/*"}},
		{Effect: Deny, Actions: []string{"monitors:delete"}, Resources: []string{"monitor/1"}},
	}
	tests := []struct {
		name     string
		policy   []Statement
		action   string
		resource string
		want     bool
	}{
		{"no statement denies", nil, "monitors:read", "monitor/42", false},
		{"a matching allow allows", editor, "monitors:read", "monitor/42", true},
		{"monitors:* matches monitors:read", editor, "monitors:read", "monitor/7", true},
		{"an explicit deny beats an allow", editor, "monitors:delete", "monitor/1", false},
		{"the deny covers only its resource", editor, "monitors:delete", "monitor/2", true},
		{"an allow for another service does not match", editor, "users:read", "monitor/42", false},
		{"the action must match", []Statement{
			{Effect: Allow, Actions: []string{"monitors:read"}, Resources: []string{"*"}},
		}, "monitors:write", "monitor/42", false},
		{"the resource must match", []Statement{
			{Effect: Allow, Actions: []string{"*"}, Resources: []string{"monitor/42"}},
		}, "monitors:read", "monitor/43", false},
		{"only a trailing star is a wildcard", []Statement{
			{Effect: Allow, Actions: []string{"monitors:*:own"}, Resources: []string{"*"}},
		}, "monitors:read:own", "monitor/42", false},
		{"a deny wins in any order", []Statement{
			{Effect: Deny, Actions: []string{"*"}, Resources: []string{"*"}},
			{Effect: Allow, Actions: []string{"monitors:read"}, Resources: []string{"monitor/42"}},
		}, "monitors:read", "monitor/42", false},
		{"an unknown effect never allows", []Statement{
			{Effect: "allow", Actions: []string{"*"}, Resources: []string{"*"}},
		}, "monitors:read", "monitor/42", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Allowed(tt.policy, tt.action, tt.resource); got != tt.want {
				t.Errorf("Allowed(%q, %q) = %v, want %v", tt.action, tt.resource, got, tt.want)
			}
		})
	}
}

// endregion: tests

// region: example
func ExampleAllowed() {
	viewer := []Statement{
		{Effect: Allow, Actions: []string{"monitors:read"}, Resources: []string{"monitor/*"}},
	}
	fmt.Println(Allowed(viewer, "monitors:read", "monitor/42"))
	fmt.Println(Allowed(viewer, "monitors:delete", "monitor/42"))
	// Output:
	// true
	// false
}

// endregion: example
