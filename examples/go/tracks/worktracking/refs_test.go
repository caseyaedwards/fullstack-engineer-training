package worktracking

import (
	"slices"
	"testing"
)

// region: test

func TestTicketRefs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"in order without duplicates", "MON-42 fix retry, see MON-7 and MON-42", []string{"MON-42", "MON-7"}},
		{"lowercase is not a key", "mon-42 fix retry", []string{}},
		{"empty text", "", []string{}},
		{"branch name", "feature/MON-42-retry-once", []string{"MON-42"}},
		{"commit subject", "MON-108: Retry a failed check once (#57)", []string{"MON-108"}},
		{"digits in the project key", "OPS2-3 and K8S-14", []string{"OPS2-3", "K8S-14"}},
		{"one letter is not a key", "A-1 and V-2", []string{}},
		{"part of a longer word", "XMON-42a and MON-42b", []string{}},
		{"anything shaped like a key matches", "UTF-8 and SHA-256", []string{"UTF-8", "SHA-256"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TicketRefs(tt.text)
			if got == nil {
				t.Fatalf("TicketRefs(%q) = nil, want an empty slice", tt.text)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("TicketRefs(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// endregion: test
