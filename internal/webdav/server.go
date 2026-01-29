package webdav

import (
	"log"
	"net/http"
	"strings"

	"golang.org/x/net/webdav"
)

type LocalServer struct {
	handler    *webdav.Handler
	authUser   string
	authPass   string
	authEnable bool
}

type SizeSetter interface {
	SetPendingSize(name string, size int64)
}

func NewLocalServer(prefix string, fs webdav.FileSystem, ls webdav.LockSystem, authUser, authPass string) *LocalServer {
	return &LocalServer{
		handler: &webdav.Handler{
			Prefix:     prefix,
			FileSystem: fs,
			LockSystem: ls,
			Logger: func(r *http.Request, err error) {
				if err != nil {
					log.Printf("WebDAV Error: %s %s: %v", r.Method, r.URL.Path, err)
				}
			},
		},
		authUser:   authUser,
		authPass:   authPass,
		authEnable: authUser != "" && authPass != "",
	}
}

func (s *LocalServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("ServeHTTP: %s %s", r.Method, r.URL.Path)

	// Wrap response writer to capture status code
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

	// Authenticate only when enabled
	if s.authEnable {
		user, pass, ok := r.BasicAuth()
		if !ok || !s.authenticate(user, pass) {
			log.Printf("Auth failed for %s", user)
			s.handleAuthFailure(w)
			return
		}
	}

	// Handle OPTIONS for WebDAV discovery and Windows compatibility
	if r.Method == "OPTIONS" {
		s.handleOptions(w, r)
		return
	}

	if r.Method == "PUT" && r.ContentLength > 0 {
		path := r.URL.Path
		if s.handler.Prefix != "" {
			path = strings.TrimPrefix(path, s.handler.Prefix)
		}
		// Ensure path starts with / for consistency with WebDAV internal handling
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		if setter, ok := s.handler.FileSystem.(SizeSetter); ok {
			setter.SetPendingSize(path, r.ContentLength)
		}
	}

	s.handler.ServeHTTP(sw, r)
	log.Printf("ServeHTTP Done: %s %s -> %d", r.Method, r.URL.Path, sw.status)
}

// handleOptions handles OPTIONS requests for WebDAV discovery
// Based on sweb project - Windows WebDAV client compatibility
func (s *LocalServer) handleOptions(w http.ResponseWriter, r *http.Request) {
	// Set WebDAV compliance level
	w.Header().Set("DAV", "1, 2")
	// Microsoft-specific header for Windows WebDAV clients
	w.Header().Set("MS-Author-Via", "DAV")
	// Server identification (mimic IIS for better Windows compatibility)
	w.Header().Set("Server", "Microsoft-IIS/10.0")
	// Supported HTTP methods
	w.Header().Set("Allow", "OPTIONS, GET, HEAD, POST, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK")
	// Accept-Ranges for resumable downloads
	w.Header().Set("Accept-Ranges", "bytes")

	w.WriteHeader(http.StatusOK)
	log.Printf("OPTIONS response sent for %s", r.URL.Path)
}

// handleAuthFailure returns a detailed authentication failure response
// Similar to sweb project - provides better error information
func (s *LocalServer) handleAuthFailure(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="ClearVault"`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>401 Unauthorized</title></head>
<body>
<h1>401 Unauthorized</h1>
<p>Authentication is required to access this resource.</p>
</body>
</html>`))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(status int) {
	sw.status = status
	sw.ResponseWriter.WriteHeader(status)
}

func (s *LocalServer) authenticate(username, password string) bool {
	if !s.authEnable {
		return true
	}
	return username == s.authUser && password == s.authPass
}
