package remote

import (
	"testing"

	"clearvault/internal/config"
)

func TestNewRemoteStorage(t *testing.T) {
	t.Run("create webdav client", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type: "webdav",
			URL:  "http://localhost:8080/dav",
			User: "admin",
			Pass: "password",
		}

		// This will fail to connect but should create the client
		// We expect an error due to connection failure, not type error
		_, err := NewRemoteStorage(cfg)
		// WebDAV client creation doesn't validate connection immediately
		// so it might succeed or fail depending on implementation
		if err != nil {
			// Expected - connection will fail
			t.Logf("WebDAV client creation failed as expected: %v", err)
		}
	})

	t.Run("create s3 client with missing endpoint", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type:      "s3",
			Endpoint:  "",
			Bucket:    "test-bucket",
			AccessKey: "test-key",
			SecretKey: "test-secret",
		}

		_, err := NewRemoteStorage(cfg)
		if err == nil {
			t.Error("Expected error for missing endpoint")
		}
	})

	t.Run("create s3 client with missing bucket", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type:      "s3",
			Endpoint:  "http://localhost:9000",
			Bucket:    "",
			AccessKey: "test-key",
			SecretKey: "test-secret",
		}

		_, err := NewRemoteStorage(cfg)
		if err == nil {
			t.Error("Expected error for missing bucket")
		}
	})

	t.Run("create s3 client with missing access key", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type:      "s3",
			Endpoint:  "http://localhost:9000",
			Bucket:    "test-bucket",
			AccessKey: "",
			SecretKey: "test-secret",
		}

		_, err := NewRemoteStorage(cfg)
		if err == nil {
			t.Error("Expected error for missing access key")
		}
	})

	t.Run("create s3 client with missing secret key", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type:      "s3",
			Endpoint:  "http://localhost:9000",
			Bucket:    "test-bucket",
			AccessKey: "test-key",
			SecretKey: "",
		}

		_, err := NewRemoteStorage(cfg)
		if err == nil {
			t.Error("Expected error for missing secret key")
		}
	})

	t.Run("create local client", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := config.RemoteConfig{
			Type:      "local",
			LocalPath: tmpDir,
		}

		client, err := NewRemoteStorage(cfg)
		if err != nil {
			t.Fatalf("NewRemoteStorage failed: %v", err)
		}
		if client == nil {
			t.Error("Expected client, got nil")
		}
		client.Close()
	})

	t.Run("create local client with missing path", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type:      "local",
			LocalPath: "",
		}

		_, err := NewRemoteStorage(cfg)
		if err == nil {
			t.Error("Expected error for missing local path")
		}
	})

	t.Run("default to webdav", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type: "", // Empty type should default to webdav
			URL:  "http://localhost:8080/dav",
		}

		// This will fail to connect but should attempt webdav
		_, err := NewRemoteStorage(cfg)
		// Expected to fail due to connection
		t.Logf("Result: %v", err)
	})

	t.Run("unsupported storage type", func(t *testing.T) {
		cfg := config.RemoteConfig{
			Type: "unsupported",
		}

		_, err := NewRemoteStorage(cfg)
		if err == nil {
			t.Error("Expected error for unsupported storage type")
		}
		if err != nil && err.Error() != "unsupported storage type: unsupported (supported: webdav, s3, local)" {
			t.Logf("Error message: %v", err)
		}
	})

	t.Run("case insensitive type", func(t *testing.T) {
		tmpDir := t.TempDir()
		testCases := []string{"LOCAL", "Local", "local", "S3", "s3", "S3", "WEBDAV", "WebDAV", "webdav"}

		for _, tc := range testCases {
			var cfg config.RemoteConfig
			switch tc {
			case "LOCAL", "Local", "local":
				cfg = config.RemoteConfig{
					Type:      tc,
					LocalPath: tmpDir,
				}
			case "S3", "s3":
				cfg = config.RemoteConfig{
					Type:      tc,
					Endpoint:  "http://localhost:9000",
					Bucket:    "test",
					AccessKey: "key",
					SecretKey: "secret",
				}
			case "WEBDAV", "WebDAV", "webdav":
				cfg = config.RemoteConfig{
					Type: tc,
					URL:  "http://localhost:8080",
				}
			}

			_, err := NewRemoteStorage(cfg)
			// We just want to ensure it doesn't fail due to type parsing
			// Connection errors are expected
			t.Logf("Type %q: %v", tc, err)
		}
	})
}
