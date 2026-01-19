package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigWithEnvOverrides(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  listen: "127.0.0.1:8080"
  base_url: "/original"
  auth:
    user: "admin"
    pass: "orig-pass"
security:
  master_key: "original-master-key-original-master-key"
storage:
  metadata_path: "storage/metadata"
remote:
  url: "http://original.com"
  user: "orig-user"
  pass: "orig-pass"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables
	t.Setenv("SERVER_LISTEN", "0.0.0.0:9090")
	t.Setenv("SERVER_BASE_URL", "/env-override")
	t.Setenv("SERVER_AUTH_USER", "env-admin")
	t.Setenv("REMOTE_URL", "http://env-override.com")

	// Load config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify overrides
	if cfg.Server.Listen != "0.0.0.0:9090" {
		t.Errorf("Expected Server.Listen to be '0.0.0.0:9090', got '%s'", cfg.Server.Listen)
	}
	if cfg.Server.BaseURL != "/env-override" {
		t.Errorf("Expected Server.BaseURL to be '/env-override', got '%s'", cfg.Server.BaseURL)
	}
	if cfg.Server.Auth.User != "env-admin" {
		t.Errorf("Expected Server.Auth.User to be 'env-admin', got '%s'", cfg.Server.Auth.User)
	}
	if cfg.Remote.URL != "http://env-override.com" {
		t.Errorf("Expected Remote.URL to be 'http://env-override.com', got '%s'", cfg.Remote.URL)
	}

	// Verify non-overridden values remain same
	if cfg.Server.Auth.Pass != "orig-pass" {
		t.Errorf("Expected Server.Auth.Pass to be 'orig-pass', got '%s'", cfg.Server.Auth.Pass)
	}
	if cfg.Security.MasterKey != "original-master-key-original-master-key" {
		t.Errorf("Expected Security.MasterKey to remain unchanged, got '%s'", cfg.Security.MasterKey)
	}
}

func TestLoadConfigNoFile(t *testing.T) {
	// Use a non-existent path
	configPath := "/non/existent/path/config.yaml"

	// Set required environment variables
	t.Setenv("MASTER_KEY", "env-provided-master-key-32-bytes-long")
	t.Setenv("SERVER_LISTEN", "0.0.0.0:8080")

	// Load config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if cfg.Security.MasterKey != "env-provided-master-key-32-bytes-long" {
		t.Errorf("Expected Security.MasterKey to be env-provided, got '%s'", cfg.Security.MasterKey)
	}
	if cfg.Server.Listen != "0.0.0.0:8080" {
		t.Errorf("Expected Server.Listen to be '0.0.0.0:8080', got '%s'", cfg.Server.Listen)
	}
}

func TestLoadConfigNoFileNoKey(t *testing.T) {
	// Use a non-existent path
	configPath := "/non/existent/path/config.yaml"

	// Clear MASTER_KEY if set in environment
	os.Unsetenv("MASTER_KEY")

	// Load config should fail
	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected LoadConfig to fail when no config file and no MASTER_KEY env var are present")
	}
}
