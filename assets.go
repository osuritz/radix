// Package radix provides embedded static assets for the radix binary.
// This file exists solely to hold the go:embed directive for the compiled
// React SPA so that the embed path "ui/dist" resolves correctly relative to
// the module root — Go's embed does not allow ".." in paths, so the directive
// must live in a file at the same directory level as the "ui/" directory.
package radix

import "embed"

// UIAssets holds the compiled React SPA embedded from ui/dist at build time.
// When only the placeholder ui/dist/.gitkeep is present (no npm build), the FS
// contains just that file; serveUIFromFS detects the missing index.html and
// serves a developer-friendly placeholder page instead.
//
//go:embed all:ui/dist
var UIAssets embed.FS
