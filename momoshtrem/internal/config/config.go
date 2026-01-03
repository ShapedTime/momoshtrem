package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Torrent  TorrentConfig  `yaml:"torrent"`
	TMDB     TMDBConfig     `yaml:"tmdb"`
}

type ServerConfig struct {
	HTTPPort   int `yaml:"http_port"`
	WebDAVPort int `yaml:"webdav_port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type TorrentConfig struct {
	MetadataFolder  string `yaml:"metadata_folder"`
	GlobalCacheSize int64  `yaml:"global_cache_size"` // MB
	AddTimeout      int    `yaml:"add_timeout"`       // seconds
	ReadTimeout     int    `yaml:"read_timeout"`      // seconds
	IdleEnabled     bool   `yaml:"idle_enabled"`
	IdleTimeout     int    `yaml:"idle_timeout"` // seconds
	StartPaused     bool   `yaml:"start_paused"`
}

type TMDBConfig struct {
	APIKey string `yaml:"api_key"`
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:   4444,
			WebDAVPort: 36911,
		},
		Database: DatabaseConfig{
			Path: "./data/momoshtrem.db",
		},
		Torrent: TorrentConfig{
			MetadataFolder:  "./data/torrents",
			GlobalCacheSize: 4096, // 4GB
			AddTimeout:      60,
			ReadTimeout:     120,
			IdleEnabled:     true,
			IdleTimeout:     300,
			StartPaused:     true,
		},
	}
}

// Load reads configuration from a YAML file
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if no config file
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// EnsureDirectories creates required directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		filepath.Dir(c.Database.Path),
		c.Torrent.MetadataFolder,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
