// Package web embeds the admin GUI's static frontend.
package web

import "embed"

// FS holds the frontend files at its root.
//
//go:embed index.html app.js style.css
var FS embed.FS
