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
	Server   ServerConfig   `yaml:"server"`
	Remote   RemoteConfig   `yaml:"remote"`
	Security SecurityConfig `yaml:"security"`
	Storage  StorageConfig  `yaml:"storage"`
}

type ServerConfig struct {
	Listen  string `yaml:"listen"`
	BaseURL string `yaml:"base_url"`
	Auth    Auth   `yaml:"auth"`
}

type Auth struct {
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
}

type RemoteConfig struct {
	URL  string `yaml:"url"`
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
}

type SecurityConfig struct {
	MasterKey string `yaml:"master_key"`
}

type StorageConfig struct {
	MetadataType string `yaml:"metadata_type"` // "sqlite" or "local"
	MetadataPath string `yaml:"metadata_path"` // db file for sqlite, dir for local
	CacheDir     string `yaml:"cache_dir"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Auto-generate master key if empty
	if cfg.Security.MasterKey == "" || cfg.Security.MasterKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
		if err := generateAndSaveMasterKey(path, &cfg); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
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
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "master_key:") {
			// Preserve indentation
			indent := strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " ")))
			lines[i] = fmt.Sprintf("%smaster_key: \"%s\"", indent, encodedKey)
			break
		}
	}

	// Write back to file
	newData := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(newData), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Printf("✅ Master key saved to %s", configPath)
	return nil
}
