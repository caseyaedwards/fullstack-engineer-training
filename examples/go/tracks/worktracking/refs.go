// Package worktracking holds the example for the Ticket and Work
// Tracking track: finding ticket keys in commit messages, branch names
// and pull request titles.
package worktracking

import "regexp"

// region: refs

// ticketKey matches a key such as MON-42: an uppercase letter, one or
// more uppercase letters or digits, a hyphen and a number, with a word
// boundary on each side. \b in Go is an ASCII word boundary.
var ticketKey = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-[0-9]+\b`)

// TicketRefs returns the ticket keys in text, in the order they first
// appear, each once. It returns an empty slice, not nil, when text
// holds no key, which encodes as [] in JSON.
func TicketRefs(text string) []string {
	refs := []string{}
	seen := map[string]bool{}
	for _, key := range ticketKey.FindAllString(text, -1) {
		if !seen[key] {
			seen[key] = true
			refs = append(refs, key)
		}
	}
	return refs
}

// endregion: refs
