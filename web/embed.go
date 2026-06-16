package web

import "embed"

// FS holds the compiled React app. web/dist must be built before compiling cmd/server.
// A .gitkeep placeholder in web/dist/ keeps the embed valid during Go-only development.
//
//go:embed dist
var FS embed.FS
