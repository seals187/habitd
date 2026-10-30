// Package migrations embeds the numbered SQL migrations.
package migrations

import "embed"

// FS holds all *.sql migrations.
//
//go:embed *.sql
var FS embed.FS
