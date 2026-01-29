package webdav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/webdav"
)

// mockFileSystem is a simple mock for testing
type mockFileSystem struct {
	files map[string][]byte
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		files: make(map[string][]byte),
	}
}

func (m *mockFileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return nil
}

func (m *mockFileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	return &mockFile{name: name, fs: m}, nil
}

func (m *mockFileSystem) RemoveAll(ctx context.Context, name string) error {
	delete(m.files, name)
	return nil
}

func (m *mockFileSystem) Rename(ctx context.Context, oldName, newName string) error {
	if data, ok := m.files[oldName]; ok {
		m.files[newName] = data
		delete(m.files, oldName)
	}
	return nil
}

func (m *mockFileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return nil, os.ErrNotExist
}

// mockFile implements webdav.File
type mockFile struct {
	name string
	fs   *mockFileSystem
	data []byte
	pos  int
}

func (f *mockFile) Close() error                               { return nil }
func (f *mockFile) Read(p []byte) (n int, err error)           { return 0, io.EOF }
func (f *mockFile) Seek(offset int64, whence int) (int64, error) { return 0, nil }
func (f *mockFile) Readdir(count int) ([]os.FileInfo, error)   { return nil, nil }
func (f *mockFile) Stat() (os.FileInfo, error)                 { return nil, nil }
func (f *mockFile) Write(p []byte) (n int, err error)          { return len(p), nil }



func TestNewLocalServer(t *testing.T) {
	fs := newMockFileSystem()
	ls := webdav.NewMemLS()

	t.Run("create server without auth", func(t *testing.T) {
		server := NewLocalServer("/dav", fs, ls, "", "")
		if server == nil {
			t.Fatal("Expected server, got nil")
		}
		if server.authEnable {
			t.Error("Auth should be disabled when no credentials provided")
		}
	})

	t.Run("create server with auth", func(t *testing.T) {
		server := NewLocalServer("/dav", fs, ls, "admin", "password")
		if server == nil {
			t.Fatal("Expected server, got nil")
		}
		if !server.authEnable {
			t.Error("Auth should be enabled when credentials provided")
		}
		if server.authUser != "admin" {
			t.Errorf("authUser = %q, want %q", server.authUser, "admin")
		}
		if server.authPass != "password" {
			t.Errorf("authPass = %q, want %q", server.authPass, "password")
		}
	})
}

func TestLocalServer_ServeHTTP_Options(t *testing.T) {
	fs := newMockFileSystem()
	ls := webdav.NewMemLS()
	server := NewLocalServer("/dav", fs, ls, "", "")

	t.Run("OPTIONS request", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/dav/test.txt", nil)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
		}

		// Check WebDAV headers
		if dav := rec.Header().Get("DAV"); dav != "1, 2" {
			t.Errorf("DAV header = %q, want %q", dav, "1, 2")
		}
		if msAuth := rec.Header().Get("MS-Author-Via"); msAuth != "DAV" {
			t.Errorf("MS-Author-Via header = %q, want %q", msAuth, "DAV")
		}
		if srv := rec.Header().Get("Server"); srv != "Microsoft-IIS/10.0" {
			t.Errorf("Server header = %q, want %q", srv, "Microsoft-IIS/10.0")
		}
		if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "PROPFIND") {
			t.Errorf("Allow header should contain PROPFIND, got %q", allow)
		}
		if acceptRanges := rec.Header().Get("Accept-Ranges"); acceptRanges != "bytes" {
			t.Errorf("Accept-Ranges header = %q, want %q", acceptRanges, "bytes")
		}
	})
}

func TestLocalServer_ServeHTTP_Auth(t *testing.T) {
	fs := newMockFileSystem()
	ls := webdav.NewMemLS()
	server := NewLocalServer("/dav", fs, ls, "admin", "secret123")

	t.Run("request without auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/dav/test.txt", nil)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		wwwAuth := rec.Header().Get("WWW-Authenticate")
		if !strings.Contains(wwwAuth, "Basic") {
			t.Errorf("WWW-Authenticate should contain Basic, got %q", wwwAuth)
		}

		contentType := rec.Header().Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			t.Errorf("Content-Type should be text/html, got %q", contentType)
		}
	})

	t.Run("request with valid auth", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/dav/test.txt", nil)
		req.SetBasicAuth("admin", "secret123")
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		// OPTIONS should succeed
		if rec.Code != http.StatusOK {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("request with invalid auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/dav/test.txt", nil)
		req.SetBasicAuth("admin", "wrongpassword")
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

func TestLocalServer_authenticate(t *testing.T) {
	fs := newMockFileSystem()
	ls := webdav.NewMemLS()

	t.Run("auth disabled", func(t *testing.T) {
		server := NewLocalServer("/dav", fs, ls, "", "")
		if !server.authenticate("any", "any") {
			t.Error("Should authenticate when auth is disabled")
		}
	})

	t.Run("valid credentials", func(t *testing.T) {
		server := NewLocalServer("/dav", fs, ls, "admin", "password")
		if !server.authenticate("admin", "password") {
			t.Error("Should authenticate with valid credentials")
		}
	})

	t.Run("invalid username", func(t *testing.T) {
		server := NewLocalServer("/dav", fs, ls, "admin", "password")
		if server.authenticate("wronguser", "password") {
			t.Error("Should not authenticate with wrong username")
		}
	})

	t.Run("invalid password", func(t *testing.T) {
		server := NewLocalServer("/dav", fs, ls, "admin", "password")
		if server.authenticate("admin", "wrongpassword") {
			t.Error("Should not authenticate with wrong password")
		}
	})
}

func TestStatusWriter(t *testing.T) {
	t.Run("capture status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

		sw.WriteHeader(http.StatusNotFound)

		if sw.status != http.StatusNotFound {
			t.Errorf("status = %d, want %d", sw.status, http.StatusNotFound)
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("recorder code = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}
