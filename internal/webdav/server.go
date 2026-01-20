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

	// Authenticate if needed
	user, pass, ok := r.BasicAuth()
	if !ok || !s.authenticate(user, pass) {
		log.Printf("Auth failed for %s", user)
		w.Header().Set("WWW-Authenticate", `Basic realm="Clearvault"`)
		w.WriteHeader(http.StatusUnauthorized)
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
