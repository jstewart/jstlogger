// Package migrations embeds the logbook's SQL schema migrations.
//
// Each file is named NNNN_description.sql and is applied once, in order. The
// logbook records the highest applied number in SQLite's user_version. Never
// edit a migration that has shipped; add a new one instead.
package migrations

import "embed"

// FS holds every migration file.
//
//go:embed *.sql
var FS embed.FS
