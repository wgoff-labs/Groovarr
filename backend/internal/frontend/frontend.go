package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// NewHandler returns an HTTP handler that serves the Next.js static frontend
func NewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqPath := r.URL.Path

		// Handle Next.js static assets (/_next/static/*)
		if strings.HasPrefix(reqPath, "/_next/static/") {
			// Browser requests /_next/static/... but files are stored at .next/static/...
			// So strip the _next/ to get the actual file path
			filePath := "dist/.next/static/" + strings.TrimPrefix(reqPath, "/_next/static/")
			content, err := fs.ReadFile(distFS, filePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			setContentType(w, filePath)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write(content)
			return
		}

		// Handle public assets (/public/*)
		if strings.HasPrefix(reqPath, "/public/") {
			filePath := "dist/" + strings.TrimPrefix(reqPath, "/")
			content, err := fs.ReadFile(distFS, filePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			setContentType(w, filePath)
			w.Write(content)
			return
		}

		// Handle Next.js server-side pre-rendered pages
		pagePath := mapToServerPath(reqPath)
		content, err := fs.ReadFile(distFS, pagePath)
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			w.Write(content)
			return
		}

		// Fallback: try index.html (root)
		content, err = fs.ReadFile(distFS, "dist/.next/server/app/index.html")
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			w.Write(content)
			return
		}

		http.NotFound(w, r)
	}
}

// mapToServerPath converts a URL path to the embedded server file path
func mapToServerPath(urlPath string) string {
	if urlPath == "" || urlPath == "/" {
		return "dist/.next/server/app/index.html"
	}

	// Remove leading slash
	cleanPath := strings.TrimPrefix(urlPath, "/")

	// Handle Next.js data manifest
	if strings.HasPrefix(cleanPath, "_next/data/") {
		parts := strings.Split(cleanPath, "/")
		if len(parts) >= 4 {
			return "dist/.next/server/app/" + strings.Join(parts[3:], "/")
		}
	}

	// Handle static pages like /artists, /settings
	return "dist/.next/server/app/" + cleanPath + "/index.html"
}

func setContentType(w http.ResponseWriter, filePath string) {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".html", ".htm":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js", ".mjs":
		w.Header().Set("Content-Type", "application/javascript")
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".txt":
		w.Header().Set("Content-Type", "text/plain")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
}
