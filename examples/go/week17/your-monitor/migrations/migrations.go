// region: embed

// Package migrations holds the SQL files that build the database schema,
// embedded into every program that imports it. Name each file
// NNN_name.sql: Migrate runs them in order of NNN.
package migrations

import "embed"

// FS holds every .sql file in this directory, read at compile time.
//
//go:embed *.sql
var FS embed.FS

// endregion: embed
