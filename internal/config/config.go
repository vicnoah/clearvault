package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Remote   RemoteConfig   `yaml:"remote" json:"remote"`
	Security SecurityConfig `yaml:"security" json:"security"`
	Storage  StorageConfig  `yaml:"storage" json:"storage"`
	Access   AccessConfig   `yaml:"access" json:"access"`
}

type AccessConfig struct {
	Token string `yaml:"token" json:"token"`
}

type ServerConfig struct {
	Listen  string `yaml:"listen" json:"listen"`
	BaseURL string `yaml:"base_url" json:"base_url"`
	Auth    Auth   `yaml:"auth" json:"auth"`
}

type Auth struct {
	User string `yaml:"user" json:"user"`
	Pass string `yaml:"pass" json:"pass"`
}

type RemoteConfig struct {
	// 通用字段
	Type string `yaml:"type" json:"type"` // "webdav" 或 "s3"

	// WebDAV 字段
	URL  string `yaml:"url" json:"url"`
	User string `yaml:"user" json:"user"`
	Pass string `yaml:"pass" json:"pass"`

	// S3 字段
	Endpoint  string `yaml:"endpoint" json:"endpoint"`
	Region    string `yaml:"region" json:"region"`
	Bucket    string `yaml:"bucket" json:"bucket"`
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	UseSSL    bool   `yaml:"use_ssl" json:"use_ssl"`

	// Local Filesystem 字段
	LocalPath string `yaml:"local_path" json:"local_path"`
}

type SecurityConfig struct {
	MasterKey string `yaml:"master_key" json:"master_key"`
}

type StorageConfig struct {
	MetadataPath string `yaml:"metadata_path" json:"metadata_path"` // JSON metadata directory
	CacheDir     string `yaml:"cache_dir" json:"cache_dir"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		// Return error if it's not a "file not found" error (e.g., permissions)
		return nil, err
	}

	// Always override with environment variables
	processEnvOverrides(&cfg)

	return &cfg, nil
}

// GenerateMasterKey checks if master key is set, if not generates it and saves to file
func GenerateMasterKey(configPath string, cfg *Config) error {
	// Validate or Generate Master Key
	if cfg.Security.MasterKey == "" || cfg.Security.MasterKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
		// Auto-generate and save
		if err := generateAndSaveMasterKey(configPath, cfg); err != nil {
			return err
		}
	}
	return nil
}

func processEnvOverrides(cfg *Config) {
	if v := os.Getenv("SERVER_LISTEN"); v != "" {
		cfg.Server.Listen = v
	}
	if v := os.Getenv("SERVER_BASE_URL"); v != "" {
		cfg.Server.BaseURL = v
	}
	if v := os.Getenv("SERVER_AUTH_USER"); v != "" {
		cfg.Server.Auth.User = v
	}
	if v := os.Getenv("SERVER_AUTH_PASS"); v != "" {
		cfg.Server.Auth.Pass = v
	}
	if v := os.Getenv("STORAGE_METADATA_PATH"); v != "" {
		cfg.Storage.MetadataPath = v
	}
	if v := os.Getenv("STORAGE_CACHE_DIR"); v != "" {
		cfg.Storage.CacheDir = v
	}
	if v := os.Getenv("REMOTE_TYPE"); v != "" {
		cfg.Remote.Type = v
	}
	if v := os.Getenv("REMOTE_URL"); v != "" {
		cfg.Remote.URL = v
	}
	if v := os.Getenv("REMOTE_USER"); v != "" {
		cfg.Remote.User = v
	}
	if v := os.Getenv("REMOTE_PASS"); v != "" {
		cfg.Remote.Pass = v
	}
	if v := os.Getenv("REMOTE_ENDPOINT"); v != "" {
		cfg.Remote.Endpoint = v
	}
	if v := os.Getenv("REMOTE_REGION"); v != "" {
		cfg.Remote.Region = v
	}
	if v := os.Getenv("REMOTE_BUCKET"); v != "" {
		cfg.Remote.Bucket = v
	}
	if v := os.Getenv("REMOTE_ACCESS_KEY"); v != "" {
		cfg.Remote.AccessKey = v
	}
	if v := os.Getenv("REMOTE_SECRET_KEY"); v != "" {
		cfg.Remote.SecretKey = v
	}
	if v := os.Getenv("REMOTE_USE_SSL"); v != "" {
		cfg.Remote.UseSSL = v == "true" || v == "1"
	}
	if v := os.Getenv("REMOTE_LOCAL_PATH"); v != "" {
		cfg.Remote.LocalPath = v
	}
	if v := os.Getenv("MASTER_KEY"); v != "" {
		cfg.Security.MasterKey = v
	}
	if v := os.Getenv("ACCESS_TOKEN"); v != "" {
		cfg.Access.Token = v
	}
}

func generateAndSaveMasterKey(configPath string, cfg *Config) error {
	// Generate random 32-byte key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("failed to generate random key: %w", err)
	}

	// Encode to base64 for readability
	encodedKey := base64.StdEncoding.EncodeToString(key)
	cfg.Security.MasterKey = encodedKey

	log.Printf("⚠️  Auto-generated master key. Saving to config file...")
	log.Printf("🔑 Master Key: %s", encodedKey)
	log.Printf("⚠️  IMPORTANT: Please backup this key! Data cannot be recovered if lost!")

	// Read original file to preserve formatting and comments
	originalData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Replace master_key line
	lines := strings.Split(string(originalData), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "master_key:") {
			// Preserve indentation
			indent := strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " ")))
			lines[i] = fmt.Sprintf("%smaster_key: \"%s\"", indent, encodedKey)
			found = true
			break
		}
	}

	if !found {
		// Log warning and return if not found (don't want to corrupt file structure)
		log.Printf("Warning: Could not find 'master_key:' line in config file to update.")
	}

	// Write back to file
	newData := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(newData), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Printf("✅ Master key saved to %s", configPath)
	return nil
}

// SaveConfig saves the configuration struct to the specified file path
func SaveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
