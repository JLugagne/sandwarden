//go:build darwin

package terminal

import "strings"

const openPath = "/usr/bin/open"

// A macOS app bundle must be started through open; running the binary inside it
// is unsupported and leaves the command unspawned.
func bundleLaunch(bin string, argv []string) (string, []string) {
	app, _, ok := strings.Cut(bin, ".app/Contents/MacOS/")
	if !ok {
		return bin, argv
	}
	return openPath, append([]string{openPath, "-na", app + ".app", "--args"}, argv[1:]...)
}
