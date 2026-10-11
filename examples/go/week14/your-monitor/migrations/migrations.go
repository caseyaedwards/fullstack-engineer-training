// contract stub: week 08
//
// Package migrations holds the SQL files that db.Migrate applies, in the
// order of their numbers. The go:embed line compiles them into every
// program that imports this package.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
