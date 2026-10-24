// Package migrations embeds the versioned PostgreSQL schema migrations.
package migrations

import "embed"

// FS holds every *.sql migration in this directory (testdata is excluded).
//
//go:embed *.sql
var FS embed.FS
