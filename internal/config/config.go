package config

import (
	"os"

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

	return &cfg, nil
}
