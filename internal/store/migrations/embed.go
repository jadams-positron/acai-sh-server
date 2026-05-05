// Package migrations embeds the goose SQL migration files.
package migrations

import "embed"

// FS holds all *.sql migration files for use with goose.SetBaseFS.
//
//go:embed *.sql
var FS embed.FS
