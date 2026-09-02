package frontend

import (
	"embed"
	"fmt"
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
)

//go:embed all:dist
var distFS embed.FS

// nodeProxy holds the Node.js reverse proxy, lazily initialized.
// nodeMu protects nodeProxy and nodeReady.
var (
	nodeMu        sync.RWMutex
	nodeProxy     *httputil.ReverseProxy
	nodeReady     = make(chan struct{}) // closed when Node.js is ready
	nodeErr       error
	nodeLogBuf    strings.Builder
	nodeLogMu     sync.Mutex
	nodeStartOnce sync.Once // ensures Node.js is started exactly once
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
	nodeStartOnce.Do(func() {
		go startNodeProxy()
	})
}

// logNode logs a line to both the log and the in-memory buffer.
func logNode(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[node] %s", msg)
	nodeLogMu.Lock()
	nodeLogBuf.WriteString(msg)
	nodeLogBuf.WriteString("\n")
	nodeLogMu.Unlock()
}

// DebugNodeStatus returns the Node.js startup status for the /debug/node endpoint.
func DebugNodeStatus() map[string]interface{} {
	nodeLogMu.Lock()
	logSnapshot := nodeLogBuf.String()
	nodeLogMu.Unlock()
	status := map[string]interface{}{"log": logSnapshot}
	if nodeErr != nil {
		status["error"] = nodeErr.Error()
	}
	select {
	case <-nodeReady:
		status["ready"] = true
		nodeMu.RLock()
		if nodeProxy != nil {
			status["proxy"] = "initialized"
		} else {
			status["proxy"] = "nil (error)"
		}
		nodeMu.RUnlock()
	default:
		status["ready"] = false
	}
	return status
}

// startNodeProxy extracts the embedded Next.js standalone build, starts a Node.js
// server, and stores the reverse proxy. Signals nodeReady when ready.
func startNodeProxy() {
	logNode("Starting embedded Node.js server...")

	tmpDir, err := os.MkdirTemp("", "groovarr-nextjs-")
	if err != nil {
		logNode("Failed to create temp dir: %v", err)
		nodeErr = err
		close(nodeReady)
		return
	}

	// Copy the standalone directory to the temp dir.
	if err := copyDir(distFS, "dist", tmpDir); err != nil {
		logNode("Failed to copy standalone dir: %v", err)
		os.RemoveAll(tmpDir)
		nodeErr = err
		close(nodeReady)
		return
	}

	logNode("Copied standalone to %s", tmpDir)

	// Verify critical node_modules are present (diagnostic).
	if entries, err := os.ReadDir(filepath.Join(tmpDir, "node_modules")); err != nil {
		logNode("WARNING: could not read node_modules: %v", err)
	} else {
		mods := make([]string, 0, len(entries))
		for _, e := range entries {
			mods = append(mods, e.Name())
		}
		logNode("node_modules contents (%d): %v", len(mods), mods)
		if _, err := os.Stat(filepath.Join(tmpDir, "node_modules", "next")); err != nil {
			logNode("WARNING: 'next' module MISSING from node_modules!")
		} else {
			logNode("'next' module present in node_modules ✓")
		}
	}

	// Capture Node.js output in real-time via a pipe goroutine.
	cmd := exec.Command("node", "server.js")
	cmd.Dir = tmpDir
	// Bind to 0.0.0.0 so we listen on all interfaces (avoids "container IP" bind issues).
	// Go will connect via 127.0.0.1 below.
	cmd.Env = append(os.Environ(), "PORT=3001", "HOSTNAME=0.0.0.0")
	cmd.Stdin = nil // prevent Node from blocking on stdin

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logNode("Failed to create stdout pipe: %v", err)
		os.RemoveAll(tmpDir)
		nodeErr = fmt.Errorf("stdout pipe: %w", err)
		close(nodeReady)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logNode("Failed to create stderr pipe: %v", err)
		os.RemoveAll(tmpDir)
		nodeErr = fmt.Errorf("stderr pipe: %w", err)
		close(nodeReady)
		return
	}

	// Drain stdout/stderr in background and append to log buffer.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				logNode("node stdout: %s", strings.TrimSpace(string(buf[:n])))
			}
			if err != nil {
				break
			}
		}
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				logNode("node stderr: %s", strings.TrimSpace(string(buf[:n])))
			}
			if err != nil {
				break
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		logNode("Failed to start Node.js: %v", err)
		os.RemoveAll(tmpDir)
		nodeErr = fmt.Errorf("start node: %w", err)
		close(nodeReady)
		return
	}

	logNode("Node.js process started (PID %d)", cmd.Process.Pid)

	// HTTP client with a per-request timeout.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	// Use 127.0.0.1 explicitly to avoid IPv6 ([::1]) resolution issues
	// in containers where Node.js binds to 0.0.0.0 but localhost resolves to ::1.
	nodeURL, _ := url.Parse("http://127.0.0.1:3001")

	// Wait for Node.js to be ready (up to 120 seconds).
	for i := 0; i < 600; i++ { // 600 * 200ms = 120s
		logNode("Polling Node.js (attempt %d/600)...", i+1)
		resp, err := httpClient.Get("http://127.0.0.1:3001/")
		if err != nil {
			// Log the type of error to see what's happening.
			logNode("httpClient.Get error: %T %v", err, err)
			// Check if the process is still alive.
			if cmd.ProcessState != nil {
				logNode("Node.js process exited already: %v", cmd.ProcessState)
				os.RemoveAll(tmpDir)
				nodeErr = fmt.Errorf("Node.js process exited: %v", cmd.ProcessState)
				close(nodeReady)
				return
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		logNode("Node.js returned HTTP %d on /: %s", resp.StatusCode, strings.TrimSpace(string(body)[:200]))
		if resp.StatusCode < 500 {
			logNode("Node.js server ready on port 3001")
			proxy := httputil.NewSingleHostReverseProxy(nodeURL)
			nodeMu.Lock()
			nodeProxy = proxy
			nodeMu.Unlock()
			close(nodeReady)
			// Reap Node.js process when it exits.
			go func() {
				cmd.Wait()
				logNode("Node.js server exited")
				os.RemoveAll(tmpDir)
			}()
			return
		}
		// 5xx = still initializing, keep waiting
		time.Sleep(200 * time.Millisecond)
	}

	// Timed out.
	logNode("Node.js server did not become ready after 120s")
	cmd.Process.Kill()
	os.RemoveAll(tmpDir)
	nodeErr = fmt.Errorf("Node.js did not become ready")
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
