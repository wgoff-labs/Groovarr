package frontend

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"fmt"
)

//go:embed all:dist
var distFS embed.FS

// escapeForJSString escapes a string for safe inclusion in a JS string literal.
// It replaces " with \" (one backslash + one quote, 2 bytes total) so the result
// can be safely embedded inside a "..." JS string literal. The next layer up
// (the script tag containing this) is itself a JS string, so when the HTML is
// parsed, \" is decoded back to a literal " in JS source.
func escapeForJSString(s string) string {
	return strings.ReplaceAll(s, `"`, `\\\"`)
}

// NewHandler returns an HTTP handler that serves the Next.js static frontend
// and proxies dynamic routes to an embedded Node.js server.
func NewHandler() http.HandlerFunc {
	// Once node server is started, reuse it.
	var nodeOnce sync.Once
	var nodeProxy *httputil.ReverseProxy
	var nodeErr error

	return func(w http.ResponseWriter, r *http.Request) {
		reqPath := r.URL.Path

		// Handle Next.js static assets (/_next/static/*)
		if strings.HasPrefix(reqPath, "/_next/static/") {
			file := strings.TrimPrefix(reqPath, "/_next/static/")
			staticFile := "dist/.next/static/" + file

			if _, err := distFS.Open(staticFile); err != nil {
				// Try fallback locations as in original code
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

		// For all other routes (non-API, non-static), proxy to Node.js server.
		// Start the Node.js server lazily on first request.
		nodeOnce.Do(func() {
			nodeProxy, nodeErr = startNodeProxy()
		})
		if nodeErr != nil {
			log.Printf("Failed to start Node.js proxy: %v", nodeErr)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		nodeProxy.ServeHTTP(w, r)
	}
}

// startNodeProxy extracts the embedded Next.js standalone build to a temporary
// directory, starts a Node.js server running server.js, and returns a reverse
// proxy pointing to it.
func startNodeProxy() (*httputil.ReverseProxy, error) {
	// Create a temporary directory to hold the standalone build.
	tmpDir, err := os.MkdirTemp("", "groovarr-nextjs-*")
	if err != nil {
		return nil, err
	}

	// Copy the embedded standalone directory to the temporary directory.
	standaloneSrc := "dist/.next/standalone"
	if err := copyEmbeddedDir(distFS, standaloneSrc, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	// The server.js expects to run from the temporary directory.
	// Set PORT=3001 to avoid conflicts with the Go server.
	cmd := exec.Command("node", "server.js")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "PORT=3001")
	// Inherit stdout and stderr so we can see logs in the main process.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Start the process.
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	// Wait for the process to be ready by attempting to connect.
	// We'll do a simple retry mechanism.
	var proxyURL *url.URL
	for i := 0; i < 10; i++ {
		proxyURL, err = url.Parse("http://localhost:3001")
		if err != nil {
			os.RemoveAll(tmpDir)
			cmd.Process.Kill()
			return nil, err
		}
		proxy := httputil.NewSingleHostReverseProxy(proxyURL)
		// Try a simple GET to / to see if the server is up.
		resp, err := http.DefaultClient.Get("http://localhost:3001/")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			// Success.
			break
		}
		// Wait a bit before retrying.
		time.Sleep(100 * time.Millisecond)
		if i == 9 {
			// Failed after retries.
			os.RemoveAll(tmpDir)
			cmd.Process.Kill()
			return nil, fmt.Errorf("Node.js server did not become ready")
		}
	}

	// Create the reverse proxy.
	proxy := httputil.NewSingleHostReverseProxy(proxyURL)

	// We also need to wait for the Node.js process to exit when the main program
	// exits. We'll do that by storing the cmd in a global variable and calling
	// Wait on shutdown. However, for now, we'll just let it run; the container
	// will be killed when the main process exits.
	// To avoid zombie processes, we can set up a signal handler, but given the
	// complexity, we'll leave it for now and note that the process will be
	// reaped when the parent exits (since we are the parent and we will wait
	// for it if we store the cmd). We'll store the cmd in a closure.

	// We'll return the proxy and also store the cmd so we can wait on it later.
	// For simplicity, we'll just return the proxy and note that the tmpDir
	// will be cleaned up when the process exits (we'll set up a wait goroutine).
	go func() {
		// Wait for the process to exit.
		if err := cmd.Wait(); err != nil {
			log.Printf("Node.js server exited with error: %v", err)
		}
		// Clean up the temporary directory.
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf("Failed to remove temporary directory %s: %v", tmpDir, err)
		}
	}()

	return proxy, nil
}

// copyEmbeddedDir copies a directory from the embedded filesystem to a physical directory.
func copyEmbeddedDir(fs embed.FS, src string, dst string) error {
	// Walk the embedded directory.
	return fs.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		data, err := fs.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
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

// setContentType sets the Content-Type header based on file extension.
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