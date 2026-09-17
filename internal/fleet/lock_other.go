//go:build !unix

package fleet

import "os"

// flockTry is a no-op where flock is unavailable: the in-process gate still
// serializes goroutines and the package keeps compiling.
func flockTry(*os.File) error { return nil }

// flockUnlock is a no-op where flock is unavailable.
func flockUnlock(*os.File) error { return nil }
