package frontend

import (
	"embed"
	"encoding/json"
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
			// Browser requests /_next/static/... but some files live at .next/ not .next/static/
			// Map known manifests to their correct locations
			file := strings.TrimPrefix(reqPath, "/_next/static/")
			staticFile := "dist/.next/static/" + file

			// Some manifests live in .next/server/ or .next/ root, not .next/static/
			if _, err := distFS.Open(staticFile); err != nil {
				// Try .next/ root for manifest files
				if _, err2 := distFS.Open("dist/.next/" + file); err2 == nil {
					content, _ := fs.ReadFile(distFS, "dist/.next/"+file)
					setContentType(w, "dist/.next/"+file)
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					w.Write(content)
					return
				}
				// Try .next/server/ for SSR manifests
				if _, err3 := distFS.Open("dist/.next/server/" + file); err3 == nil {
					content, _ := fs.ReadFile(distFS, "dist/.next/server/"+file)
					setContentType(w, "dist/.next/server/"+file)
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					w.Write(content)
					return
				}
				http.NotFound(w, r)
				return
			}

			content, err := fs.ReadFile(distFS, staticFile)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			setContentType(w, staticFile)
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

		// Fallback: serve the SPA shell by stripping the not-found RSC data and
			// rewriting urlParts/initialTree to match the actual requested URL.
			// (Dynamic Next.js routes like /artists/[id]/manage don't get pre-rendered
			// as .html files. index.html hardcodes the dashboard's RSC stream which
			// would render the dashboard on every route.)
			content, err = fs.ReadFile(distFS, "dist/.next/server/app/_not-found.html")
			if err == nil {
				// Build URL parts from the request path
				// e.g. /artists/1/manage -> ["", "artists", "1", "manage"]
				cleanPath := strings.TrimPrefix(reqPath, "/")
				var urlParts []string
				urlParts = append(urlParts, "")
				if cleanPath != "" {
					for _, part := range strings.Split(cleanPath, "/") {
						urlParts = append(urlParts, part)
					}
				}
				urlPartsJSON := encodeJSONArray(urlParts)

				html := string(content)
				// Rewrite the not-found urlParts to match the actual URL
				// The literal string in the HTML is: "urlParts":["","_not-found"]
				html = strings.Replace(html, `"urlParts":["","_not-found"]`, `"urlParts":`+urlPartsJSON, 1)
				// Rewrite the not-found initialTree to be just __PAGE__ (no not-found wrapper)
				// The literal string is: "initialTree":[["",{"children":["/_not-found",{"children":["__PAGE__",{}]}]}]]
				// Replace the children array with just [["",{"children":[{"__PAGE__",{}}]}]]
				html = strings.Replace(html, `"initialTree":[["",{"children":["/_not-found",{"children":["__PAGE__",{}]}]}]]`, `"initialTree":[["",{"children":[{"__PAGE__",{}]}]]`, 1)

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
				w.Write([]byte(html))
				return
			}

		// Last resort: try index.html
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

// encodeJSONArray encodes a string slice as a JSON array (e.g. ["", "a", "b"])
func encodeJSONArray(parts []string) string {
	// Use encoding/json for safety
	b, _ := json.Marshal(parts)
	return string(b)
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

	// Handle static pages - try both flat .html format and subdirectory/index.html format
	// Next.js generates flat files like /artists.html, /settings.html, /index.html
	flatPath := "dist/.next/server/app/" + cleanPath + ".html"
	subPath := "dist/.next/server/app/" + cleanPath + "/index.html"

	// Try flat .html first, then subdirectory format
	if _, err := distFS.Open(flatPath); err == nil {
		return flatPath
	}
	return subPath
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
