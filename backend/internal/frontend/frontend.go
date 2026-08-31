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
			file := strings.TrimPrefix(reqPath, "/_next/static/")
			staticFile := "dist/.next/static/" + file

			if _, err := distFS.Open(staticFile); err != nil {
				if _, err2 := distFS.Open("dist/.next/" + file); err2 == nil {
					content, _ := fs.ReadFile(distFS, "dist/.next/"+file)
					setContentType(w, "dist/.next/"+file)
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					w.Write(content)
					return
				}
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

		// SPA Fallback: serve index.html with rewritten urlParts and initialTree for the actual URL.
		// This allows the Next.js client-side router to render the correct page for dynamic routes.
		content, err = fs.ReadFile(distFS, "dist/.next/server/app/index.html")
		if err == nil {
			html := string(content)

			// Build the correct urlParts JSON array from the request path.
			// e.g. /artists/1/manage -> ["", "artists", "1", "manage"]
			cleanPath := strings.TrimPrefix(reqPath, "/")
			urlParts := []string{""}
			if cleanPath != "" {
				urlParts = append(urlParts, strings.Split(cleanPath, "/")...)
			}
			urlPartsJSON, _ := json.Marshal(urlParts)

			// Build the correct initialTree as a nested structure.
			// For /artists/1/manage:
			//   [["",{"children":[["artists",{"children":[["1",{"children":[["manage",{"children":[[\"__PAGE__\",{}]]}]]}]]}]]}]]
			type TreeNode struct {
				Key      string     `json:"key"`
				Children []TreeNode `json:"children"`
			}
			// Build tree bottom-up. Leaf = ["__PAGE__", {}]
			leaf := TreeNode{Key: "__PAGE__"}
			// Wrap each path segment
			current := leaf
			// Reverse the path parts to wrap from the deepest up
			parts := []string{}
			if cleanPath != "" {
				parts = strings.Split(cleanPath, "/")
			}
			// Build from innermost outward: for parts ["artists","1","manage"]
			// innermost is "manage" (wraps __PAGE__), then "1" wraps that, etc.
			for i := len(parts) - 1; i >= 0; i-- {
				current = TreeNode{Key: parts[i], Children: []TreeNode{current}}
			}
			root := TreeNode{Key: "", Children: []TreeNode{current}}
			initialTree, _ := json.Marshal([]TreeNode{root})

			// Replace the urlParts and initialTree in the __next_f.push[0] block.
			html = strings.Replace(html, `"urlParts":["",""]`, `"urlParts":`+string(urlPartsJSON), 1)
			html = strings.Replace(html, `"initialTree":["",{"children":["__PAGE__",{}]}]`, `"initialTree":`+string(initialTree), 1)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			w.Write([]byte(html))
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

	cleanPath := strings.TrimPrefix(urlPath, "/")

	// Handle Next.js data manifest
	if strings.HasPrefix(cleanPath, "_next/data/") {
		parts := strings.Split(cleanPath, "/")
		if len(parts) >= 4 {
			return "dist/.next/server/app/" + strings.Join(parts[3:], "/")
		}
	}

	// Handle static pages - try both flat .html format and subdirectory/index.html format
	flatPath := "dist/.next/server/app/" + cleanPath + ".html"
	subPath := "dist/.next/server/app/" + cleanPath + "/index.html"

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

// keep json import used
var _ = json.Marshal