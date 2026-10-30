// Package web embeds templates and static assets.
package web

import "embed"

// FS holds templates and static assets.
//
//go:embed all:templates static
var FS embed.FS
