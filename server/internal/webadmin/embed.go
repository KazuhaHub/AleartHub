// Package webadmin embeds the built React admin SPA (Vite output) into the Go
// binary, served at /admin with SPA fallback. Run `make web-build` (vite build)
// before `go build` to refresh dist/.
package webadmin

import "embed"

//go:embed all:dist
var DistFS embed.FS
