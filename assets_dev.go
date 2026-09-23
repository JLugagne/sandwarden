//go:build !production

package main

import "os"

// assets serves web/dist from disk in development builds, so vet and tests
// compile without a frontend build. `wails3 dev` loads assets from the Vite
// dev server instead.
var assets = os.DirFS("web/dist")
