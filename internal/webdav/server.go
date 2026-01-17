package webdav

import (
	"log"
	"net/http"

	"golang.org/x/net/webdav"
)

type LocalServer struct {
	handler *webdav.Handler
}

func NewLocalServer(prefix string, fs webdav.FileSystem, ls webdav.LockSystem) *LocalServer {
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

func (s *LocalServer) authenticate(user, pass string) bool {
	// TODO: Load from config
	return true
}
