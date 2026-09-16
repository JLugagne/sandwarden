package desktop

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AssetMiddleware applies the security headers and serves the SPA shell for
// client-side routes that do not map to a file.
func AssetMiddleware(assets fs.FS) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setSecurityHeaders(w.Header())
			if isSPANavigation(r) && !assetExists(assets, r.URL.Path) {
				shell := r.Clone(r.Context())
				shell.URL.Path = "/"
				next.ServeHTTP(w, shell)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isSPANavigation reports whether the request is a webview navigation that
// should fall back to index.html. Bindings and event calls under /wails/ are
// never rewritten.
func isSPANavigation(r *http.Request) bool {
	if r.Method != http.MethodGet || path.Ext(r.URL.Path) != "" {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/wails/") {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func assetExists(assets fs.FS, urlPath string) bool {
	name := strings.TrimPrefix(path.Clean(urlPath), "/")
	if name == "" {
		name = "index.html"
	}
	_, err := fs.Stat(assets, name)
	return err == nil
}
