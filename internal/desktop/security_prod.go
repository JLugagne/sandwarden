//go:build production

package desktop

import "net/http"

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; " +
	"connect-src 'self'; form-action 'self'; base-uri 'self'; " +
	"frame-ancestors 'none'; object-src 'none'"

// setSecurityHeaders hardens every asset-server response. Only production
// builds get the strict policy: the Vite dev server needs inline scripts and
// websocket connections for HMR.
func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
}
