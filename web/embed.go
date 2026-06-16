package web

import "embed"

// FS holds the compiled React app. web/dist must be built before compiling cmd/server.
// web/dist/placeholder is a sentinel that keeps the embed valid during Go-only development
// (Go embed ignores dotfiles, so a regular file is required).
//
//go:embed dist
var FS embed.FS
