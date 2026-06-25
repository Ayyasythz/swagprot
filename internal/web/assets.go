package web

import "embed"

// assetsFS holds the static browser UI. It is served at the root path.
//
//go:embed assets
var assetsFS embed.FS
