package frontend

import (
	"embed"
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
)

//go:embed all:dist
var distFS embed.FS

// nodeProxy holds the Node.js reverse proxy, lazily initialized.
// nodeMu protects nodeProxy and nodeReady.
var (
	nodeMu     sync.RWMutex
	nodeProxy  *httputil.ReverseProxy
	nodeReady  = make(chan struct{}) // closed when Node.js is ready
	nodeErr    error
)

// NewHandler returns an HTTP handler that serves the Next.js frontend.
// It serves pre-rendered HTML for static routes and proxies dynamic routes
// to the embedded Node.js standalone server.
func NewHandler() http.HandlerFunc {
	// Start Node.js startup in the background on the first call.
	// Subsequent calls are no-ops.
	startNodeBackground()

	return func(w http.ResponseWriter, r *http.Request) {
		reqPath := r.URL.Path

		// Static assets: /_next/static/*
		if strings.HasPrefix(reqPath, "/_next/static/") {
			serveStaticAsset(w, r, reqPath)
			return
		}

		// Public assets: /public/*
		if strings.HasPrefix(reqPath, "/public/") {
			servePublicAsset(w, r, reqPath)
			return
		}

		// API routes should have been handled by the main router before this handler.
		// Try pre-rendered HTML for static routes (artists.html, settings.html, etc.)
		// This gives instant response even while Node.js is warming up.
		pagePath := mapToServerPath(reqPath)
		if content, err := fs.ReadFile(distFS, pagePath); err == nil {
			setNoCache(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(content)
			return
		}

		// No pre-rendered HTML. Proxy to Node.js (handles SSR for dynamic routes).
		// Wait up to 30 seconds for Node.js to be ready.
		select {
		case <-nodeReady:
			// Node.js is ready.
		case <-time.After(30 * time.Second):
			// Timeout — log and return error.
			log.Printf("[frontend] Node.js server did not become ready within 30s")
			http.Error(w, "Service temporarily unavailable (server warming up)", http.StatusServiceUnavailable)
			return
		}

		nodeMu.RLock()
		proxy := nodeProxy
		nodeMu.RUnlock()

		if proxy == nil {
			log.Printf("[frontend] Node.js proxy is nil")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		proxy.ServeHTTP(w, r)
	}
}

// startNodeBackground starts the Node.js server in a background goroutine.
// It runs exactly once (protected by sync.Once).
func startNodeBackground() {
	var once sync.Once
	once.Do(func() {
		go startNodeProxy()
	})
}

// startNodeProxy extracts the embedded Next.js standalone build, starts a Node.js
// server, and stores the reverse proxy. Signals nodeReady when ready.
func startNodeProxy() {
	log.Printf("[frontend] Starting embedded Node.js server...")

	tmpDir, err := os.MkdirTemp("", "groovarr-nextjs-*")
	if err != nil {
		log.Printf("[frontend] Failed to create temp dir: %v", err)
		nodeErr = err
		close(nodeReady)
		return
	}

	// Copy the standalone directory to the temp dir.
	if err := copyDir(distFS, "dist/.next/standalone", tmpDir); err != nil {
		log.Printf("[frontend] Failed to copy standalone dir: %v", err)
		os.RemoveAll(tmpDir)
		nodeErr = err
		close(nodeReady)
		return
	}

	cmd := exec.Command("node", "server.js")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "PORT=3001")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[frontend] Failed to start Node.js: %v", err)
		os.RemoveAll(tmpDir)
		nodeErr = err
		close(nodeReady)
		return
	}

	// Wait for Node.js to be ready (up to 60 seconds).
	nodeURL, _ := url.Parse("http://localhost:3001")
	for i := 0; i < 300; i++ { // 300 * 200ms = 60s
		resp, err := http.Get("http://localhost:3001/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				log.Printf("[frontend] Node.js server ready on port 3001")
				proxy := httputil.NewSingleHostReverseProxy(nodeURL)
				nodeMu.Lock()
				nodeProxy = proxy
				nodeMu.Unlock()
				close(nodeReady)
				// Reap Node.js process when it exits.
				go func() {
					cmd.Wait()
					log.Printf("[frontend] Node.js server exited")
					os.RemoveAll(tmpDir)
				}()
				return
			}
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Timed out.
	log.Printf("[frontend] Node.js server did not become ready after 60s")
	cmd.Process.Kill()
	os.RemoveAll(tmpDir)
	nodeErr = err
	close(nodeReady)
}

// copyDir copies an entire directory tree from the embedded filesystem to dst.
func copyDir(emb embed.FS, src, dst string) error {
	return fs.WalkDir(emb, src, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, p)
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
		data, err := emb.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
}

// mapToServerPath converts a URL path to the embedded server file path.
func mapToServerPath(urlPath string) string {
	if urlPath == "" || urlPath == "/" {
		return "dist/.next/server/app/index.html"
	}
	cleanPath := strings.TrimPrefix(urlPath, "/")
	if strings.HasPrefix(cleanPath, "_next/data/") {
		parts := strings.Split(cleanPath, "/")
		if len(parts) >= 4 {
			return "dist/.next/server/app/" + strings.Join(parts[3:], "/")
		}
	}
	flatPath := "dist/.next/server/app/" + cleanPath + ".html"
	subPath := "dist/.next/server/app/" + cleanPath + "/index.html"
	if _, err := distFS.Open(flatPath); err == nil {
		return flatPath
	}
	return subPath
}

// serveStaticAsset serves Next.js static assets from the embedded filesystem.
func serveStaticAsset(w http.ResponseWriter, r *http.Request, reqPath string) {
	file := strings.TrimPrefix(reqPath, "/_next/static/")
	staticFile := "dist/.next/static/" + file

	// Try in order: static dir, .next/ root, .next/server/
	paths := []string{staticFile, "dist/.next/" + file, "dist/.next/server/" + file}
	for _, p := range paths {
		if content, err := fs.ReadFile(distFS, p); err == nil {
			setContentType(w, p)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write(content)
			return
		}
	}
	http.NotFound(w, r)
}

// servePublicAsset serves public assets from the embedded filesystem.
func servePublicAsset(w http.ResponseWriter, r *http.Request, reqPath string) {
	filePath := "dist/" + strings.TrimPrefix(reqPath, "/")
	content, err := fs.ReadFile(distFS, filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	setContentType(w, filePath)
	w.Write(content)
}

// setNoCache sets headers to prevent caching of dynamic pages.
func setNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
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
