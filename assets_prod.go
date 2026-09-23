//go:build production

package main

import "embed"

// assets is the built frontend. Production builds require web/dist, so run
// `npm run build` in web/ first; a missing build fails compilation.
//
//go:embed all:web/dist
var assets embed.FS
