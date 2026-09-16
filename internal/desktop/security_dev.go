//go:build !production

package desktop

import "net/http"

// setSecurityHeaders is a no-op in dev builds so the Vite dev server (inline
// preamble, HMR websocket) keeps working. Production builds apply the strict
// policy; see security_prod.go.
func setSecurityHeaders(http.Header) {}
