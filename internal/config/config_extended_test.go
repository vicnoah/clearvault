package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	cfg := &Config{
		Server: ServerConfig{
			Listen:  "0.0.0.0:8080",
			BaseURL: "/dav",
			Auth: Auth{
				User: "admin",
				Pass: "secret",
			},
		},
		Security: SecurityConfig{
			MasterKey: "test-master-key-32-bytes-long!!",
		},
		Storage: StorageConfig{
			MetadataPath: "storage/metadata",
			CacheDir:     "storage/cache",
		},
		Remote: RemoteConfig{
			Type: "webdav",
			URL:  "http://example.com/dav",
			User: "user",
			Pass: "pass",
		},
		Access: AccessConfig{
			Token: "test-token",
		},
	}

	err := SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file should exist after SaveConfig")
	}

	// Load and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Server.Listen != cfg.Server.Listen {
		t.Errorf("Server.Listen = %q, want %q", loaded.Server.Listen, cfg.Server.Listen)
	}
	if loaded.Security.MasterKey != cfg.Security.MasterKey {
		t.Errorf("Security.MasterKey mismatch")
	}
	if loaded.Remote.Type != cfg.Remote.Type {
		t.Errorf("Remote.Type = %q, want %q", loaded.Remote.Type, cfg.Remote.Type)
	}
}

func TestProcessEnvOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		check    func(*Config) bool
		expected string
	}{
		{
			name:     "SERVER_LISTEN",
			envVars:  map[string]string{"SERVER_LISTEN": "0.0.0.0:9090"},
			check:    func(c *Config) bool { return c.Server.Listen == "0.0.0.0:9090" },
			expected: "Server.Listen = 0.0.0.0:9090",
		},
		{
			name:     "SERVER_BASE_URL",
			envVars:  map[string]string{"SERVER_BASE_URL": "/api"},
			check:    func(c *Config) bool { return c.Server.BaseURL == "/api" },
			expected: "Server.BaseURL = /api",
		},
		{
			name:     "SERVER_AUTH_USER",
			envVars:  map[string]string{"SERVER_AUTH_USER": "testuser"},
			check:    func(c *Config) bool { return c.Server.Auth.User == "testuser" },
			expected: "Server.Auth.User = testuser",
		},
		{
			name:     "SERVER_AUTH_PASS",
			envVars:  map[string]string{"SERVER_AUTH_PASS": "testpass"},
			check:    func(c *Config) bool { return c.Server.Auth.Pass == "testpass" },
			expected: "Server.Auth.Pass = testpass",
		},
		{
			name:     "STORAGE_METADATA_PATH",
			envVars:  map[string]string{"STORAGE_METADATA_PATH": "/custom/metadata"},
			check:    func(c *Config) bool { return c.Storage.MetadataPath == "/custom/metadata" },
			expected: "Storage.MetadataPath = /custom/metadata",
		},
		{
			name:     "STORAGE_CACHE_DIR",
			envVars:  map[string]string{"STORAGE_CACHE_DIR": "/custom/cache"},
			check:    func(c *Config) bool { return c.Storage.CacheDir == "/custom/cache" },
			expected: "Storage.CacheDir = /custom/cache",
		},
		{
			name:     "REMOTE_TYPE",
			envVars:  map[string]string{"REMOTE_TYPE": "s3"},
			check:    func(c *Config) bool { return c.Remote.Type == "s3" },
			expected: "Remote.Type = s3",
		},
		{
			name:     "REMOTE_URL",
			envVars:  map[string]string{"REMOTE_URL": "http://webdav.example.com"},
			check:    func(c *Config) bool { return c.Remote.URL == "http://webdav.example.com" },
			expected: "Remote.URL = http://webdav.example.com",
		},
		{
			name:     "REMOTE_USER",
			envVars:  map[string]string{"REMOTE_USER": "remoteuser"},
			check:    func(c *Config) bool { return c.Remote.User == "remoteuser" },
			expected: "Remote.User = remoteuser",
		},
		{
			name:     "REMOTE_PASS",
			envVars:  map[string]string{"REMOTE_PASS": "remotepass"},
			check:    func(c *Config) bool { return c.Remote.Pass == "remotepass" },
			expected: "Remote.Pass = remotepass",
		},
		{
			name:     "REMOTE_ENDPOINT",
			envVars:  map[string]string{"REMOTE_ENDPOINT": "s3.example.com"},
			check:    func(c *Config) bool { return c.Remote.Endpoint == "s3.example.com" },
			expected: "Remote.Endpoint = s3.example.com",
		},
		{
			name:     "REMOTE_REGION",
			envVars:  map[string]string{"REMOTE_REGION": "us-west-2"},
			check:    func(c *Config) bool { return c.Remote.Region == "us-west-2" },
			expected: "Remote.Region = us-west-2",
		},
		{
			name:     "REMOTE_BUCKET",
			envVars:  map[string]string{"REMOTE_BUCKET": "my-bucket"},
			check:    func(c *Config) bool { return c.Remote.Bucket == "my-bucket" },
			expected: "Remote.Bucket = my-bucket",
		},
		{
			name:     "REMOTE_ACCESS_KEY",
			envVars:  map[string]string{"REMOTE_ACCESS_KEY": "AKIAIOSFODNN7EXAMPLE"},
			check:    func(c *Config) bool { return c.Remote.AccessKey == "AKIAIOSFODNN7EXAMPLE" },
			expected: "Remote.AccessKey = AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:     "REMOTE_SECRET_KEY",
			envVars:  map[string]string{"REMOTE_SECRET_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
			check:    func(c *Config) bool { return c.Remote.SecretKey == "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" },
			expected: "Remote.SecretKey = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name:     "REMOTE_USE_SSL true",
			envVars:  map[string]string{"REMOTE_USE_SSL": "true"},
			check:    func(c *Config) bool { return c.Remote.UseSSL == true },
			expected: "Remote.UseSSL = true",
		},
		{
			name:     "REMOTE_USE_SSL 1",
			envVars:  map[string]string{"REMOTE_USE_SSL": "1"},
			check:    func(c *Config) bool { return c.Remote.UseSSL == true },
			expected: "Remote.UseSSL = true (with 1)",
		},
		{
			name:     "REMOTE_USE_SSL false",
			envVars:  map[string]string{"REMOTE_USE_SSL": "false"},
			check:    func(c *Config) bool { return c.Remote.UseSSL == false },
			expected: "Remote.UseSSL = false",
		},
		{
			name:     "REMOTE_LOCAL_PATH",
			envVars:  map[string]string{"REMOTE_LOCAL_PATH": "/local/storage"},
			check:    func(c *Config) bool { return c.Remote.LocalPath == "/local/storage" },
			expected: "Remote.LocalPath = /local/storage",
		},
		{
			name:     "MASTER_KEY",
			envVars:  map[string]string{"MASTER_KEY": "env-master-key-32-bytes-long!!"},
			check:    func(c *Config) bool { return c.Security.MasterKey == "env-master-key-32-bytes-long!!" },
			expected: "Security.MasterKey = env-master-key-32-bytes-long!!",
		},
		{
			name:     "ACCESS_TOKEN",
			envVars:  map[string]string{"ACCESS_TOKEN": "env-access-token"},
			check:    func(c *Config) bool { return c.Access.Token == "env-access-token" },
			expected: "Access.Token = env-access-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars first
			clearEnvVars()

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			cfg := &Config{}
			processEnvOverrides(cfg)

			if !tt.check(cfg) {
				t.Errorf("Expected %s", tt.expected)
			}
		})
	}
}

func TestGenerateMasterKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config file with placeholder key
	configContent := `
server:
  listen: ":8080"
security:
  master_key: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Listen: ":8080",
		},
		Security: SecurityConfig{
			MasterKey: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY",
		},
	}

	err := GenerateMasterKey(configPath, cfg)
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	// Verify key was generated
	if cfg.Security.MasterKey == "" {
		t.Error("Master key should be generated")
	}
	if cfg.Security.MasterKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
		t.Error("Master key should be different from placeholder")
	}

	// Verify key is valid base64
	if len(cfg.Security.MasterKey) < 32 {
		t.Error("Generated key should be at least 32 characters")
	}

	// Verify file was updated
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	if !strings.Contains(string(content), cfg.Security.MasterKey) {
		t.Error("Config file should contain the new master key")
	}
}

func TestGenerateMasterKey_EmptyKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config file with empty key
	configContent := `
server:
  listen: ":8080"
security:
  master_key: ""
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Listen: ":8080",
		},
		Security: SecurityConfig{
			MasterKey: "",
		},
	}

	err := GenerateMasterKey(configPath, cfg)
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	// Verify key was generated
	if cfg.Security.MasterKey == "" {
		t.Error("Master key should be generated from empty")
	}
}

func TestGenerateMasterKey_AlreadySet(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	existingKey := "existing-master-key-32-bytes-long!!"
	configContent := `
server:
  listen: ":8080"
security:
  master_key: "` + existingKey + `"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Listen: ":8080",
		},
		Security: SecurityConfig{
			MasterKey: existingKey,
		},
	}

	err := GenerateMasterKey(configPath, cfg)
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	// Verify key was NOT changed
	if cfg.Security.MasterKey != existingKey {
		t.Error("Existing master key should not be changed")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_config.yaml")

	// Write invalid YAML
	invalidContent := `
server:
  listen: [invalid yaml structure
		unclosed bracket
`
	if err := os.WriteFile(configPath, []byte(invalidContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	configPath := "/nonexistent/path/config.yaml"

	// Clear environment
	clearEnvVars()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig should not fail for missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("Config should not be nil")
	}
}

// Helper function to clear all environment variables that affect config
func clearEnvVars() {
	envVars := []string{
		"SERVER_LISTEN", "SERVER_BASE_URL", "SERVER_AUTH_USER", "SERVER_AUTH_PASS",
		"STORAGE_METADATA_PATH", "STORAGE_CACHE_DIR",
		"REMOTE_TYPE", "REMOTE_URL", "REMOTE_USER", "REMOTE_PASS",
		"REMOTE_ENDPOINT", "REMOTE_REGION", "REMOTE_BUCKET",
		"REMOTE_ACCESS_KEY", "REMOTE_SECRET_KEY", "REMOTE_USE_SSL", "REMOTE_LOCAL_PATH",
		"MASTER_KEY", "ACCESS_TOKEN",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}
