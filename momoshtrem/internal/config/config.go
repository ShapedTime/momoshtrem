package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Torrent       TorrentConfig       `yaml:"torrent"`
	TMDB          TMDBConfig          `yaml:"tmdb"`
	VFS           VFSConfig           `yaml:"vfs"`
	Streaming     StreamingConfig     `yaml:"streaming"`
	OpenSubtitles OpenSubtitlesConfig `yaml:"opensubtitles"`
	Subtitles     SubtitlesConfig     `yaml:"subtitles"`
	AirDateSync   AirDateSyncConfig   `yaml:"airdate_sync"`
	Metrics       MetricsConfig       `yaml:"metrics"`
}

type ServerConfig struct {
	HTTPPort   int              `yaml:"http_port"`
	WebDAVPort int              `yaml:"webdav_port"`
	WebDAVAuth WebDAVAuthConfig `yaml:"webdav_auth"`
}

// WebDAVAuthConfig configures authentication for the WebDAV server
type WebDAVAuthConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable Basic Auth (default: false)
	Username string `yaml:"username"` // Username for Basic Auth
	Password string `yaml:"password"` // Password for Basic Auth
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
	URL  string `yaml:"url"`
}

type TorrentConfig struct {
	MetadataFolder       string `yaml:"metadata_folder"`
	GlobalCacheSize      int64  `yaml:"global_cache_size"`       // MB
	AddTimeout           int    `yaml:"add_timeout"`             // seconds
	ReadTimeout          int    `yaml:"read_timeout"`            // seconds
	IdleEnabled          bool   `yaml:"idle_enabled"`
	IdleTimeout          int    `yaml:"idle_timeout"`            // seconds
	StartPaused          bool   `yaml:"start_paused"`
	DropDuplicatePeerIds bool   `yaml:"drop_duplicate_peer_ids"` // Prevent duplicate peer connections
	MaxUnverifiedMB      int64  `yaml:"max_unverified_mb"`       // Cap in-flight unverified data (MB, 0=unlimited)
}

type TMDBConfig struct {
	APIKey string `yaml:"api_key"`
}

type VFSConfig struct {
	TreeTTL  int    `yaml:"tree_ttl"`  // DEPRECATED: ignored, updates are now event-driven
	CacheDir string `yaml:"cache_dir"` // Directory for persistent VFS tree cache
}

// StreamingConfig configures streaming optimization for video playback
type StreamingConfig struct {
	HeaderPriorityBytes int64 `yaml:"header_priority_bytes"` // Bytes at start to prioritize (default: 10MB)
	FooterPriorityBytes int64 `yaml:"footer_priority_bytes"` // Bytes at end to prioritize (default: 5MB)
	ReadaheadBytes      int64 `yaml:"readahead_bytes"`       // Bytes to read ahead (default: 64MB)
	UrgentBufferBytes   int64 `yaml:"urgent_buffer_bytes"`   // Immediate buffer around seek (default: 8MB)
}

// OpenSubtitlesConfig configures the OpenSubtitles API client
type OpenSubtitlesConfig struct {
	APIKey   string `yaml:"api_key"`  // Required: OpenSubtitles API key
	Username string `yaml:"username"` // Optional: for higher download limits
	Password string `yaml:"password"` // Optional: for authenticated downloads
}

// SubtitlesConfig configures subtitle storage
type SubtitlesConfig struct {
	DownloadPath string `yaml:"download_path"` // Local storage path for downloaded subtitles
}

// MetricsConfig configures Prometheus metrics exposure
type MetricsConfig struct {
	Enabled bool `yaml:"enabled"` // Enable metrics endpoint (default: false)
	Port    int  `yaml:"port"`    // HTTP port for /metrics (default: 9090)
}

// AirDateSyncConfig configures the background air date sync service
type AirDateSyncConfig struct {
	Enabled           bool `yaml:"enabled"`             // Enable air date sync (default: true)
	SyncIntervalHours int  `yaml:"sync_interval_hours"` // Hours between syncs (default: 24)
	LookbackDays      int  `yaml:"lookback_days"`       // Days to look back for recently aired (default: 30)
	BatchSize         int  `yaml:"batch_size"`          // Shows per batch to avoid rate limits (default: 5)
	BatchDelayMs      int  `yaml:"batch_delay_ms"`      // Delay between batches in ms (default: 500)
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:   4444,
			WebDAVPort: 36911,
			WebDAVAuth: WebDAVAuthConfig{
				Enabled: false, // Disabled by default for backward compatibility
			},
		},
		Database: DatabaseConfig{
			Path: "./data/momoshtrem.db",
			URL:  "postgres://momoshtrem:momoshtrem@localhost:5432/momoshtrem?sslmode=disable",
		},
		Torrent: TorrentConfig{
			MetadataFolder:       "./data/torrents",
			GlobalCacheSize:      4096, // 4GB
			AddTimeout:           60,
			ReadTimeout:          120,
			IdleEnabled:          true,
			IdleTimeout:          300,
			StartPaused:          true,
			DropDuplicatePeerIds: true,
			MaxUnverifiedMB:      16,
		},
		VFS: VFSConfig{
			TreeTTL:  0,              // DEPRECATED: ignored
			CacheDir: "./data/cache", // Persistent VFS tree cache
		},
		Streaming: StreamingConfig{
			HeaderPriorityBytes: 10 * 1024 * 1024, // 10MB
			FooterPriorityBytes: 5 * 1024 * 1024,  // 5MB
			ReadaheadBytes:      32 * 1024 * 1024,  // 32MB
			UrgentBufferBytes:   8 * 1024 * 1024,   // 8MB
		},
		OpenSubtitles: OpenSubtitlesConfig{},
		Subtitles: SubtitlesConfig{
			DownloadPath: "./data/subtitles",
		},
		AirDateSync: AirDateSyncConfig{
			Enabled:           true,
			SyncIntervalHours: 24,
			LookbackDays:      30,
			BatchSize:         5,
			BatchDelayMs:      500,
		},
		Metrics: MetricsConfig{
			Enabled: false,
			Port:    9090,
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

	// Environment variable overrides
	if envKey := os.Getenv("TMDB_API_KEY"); envKey != "" {
		cfg.TMDB.APIKey = envKey
	}

	// WebDAV auth environment variable overrides
	if envEnabled := os.Getenv("WEBDAV_AUTH_ENABLED"); envEnabled != "" {
		cfg.Server.WebDAVAuth.Enabled = strings.ToLower(envEnabled) == "true"
	}
	if envUser := os.Getenv("WEBDAV_USERNAME"); envUser != "" {
		cfg.Server.WebDAVAuth.Username = envUser
	}
	if envPass := os.Getenv("WEBDAV_PASSWORD"); envPass != "" {
		cfg.Server.WebDAVAuth.Password = envPass
	}

	// Metrics environment variable overrides
	if envEnabled := os.Getenv("METRICS_ENABLED"); envEnabled != "" {
		cfg.Metrics.Enabled = strings.ToLower(envEnabled) == "true"
	}
	if envPort := os.Getenv("METRICS_PORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			cfg.Metrics.Port = port
		}
	}

	// Database URL environment variable override
	if envURL := os.Getenv("DATABASE_URL"); envURL != "" {
		cfg.Database.URL = envURL
	}

	// OpenSubtitles environment variable overrides
	if envKey := os.Getenv("OPENSUBTITLES_API_KEY"); envKey != "" {
		cfg.OpenSubtitles.APIKey = envKey
	}
	if envUser := os.Getenv("OPENSUBTITLES_USERNAME"); envUser != "" {
		cfg.OpenSubtitles.Username = envUser
	}
	if envPass := os.Getenv("OPENSUBTITLES_PASSWORD"); envPass != "" {
		cfg.OpenSubtitles.Password = envPass
	}

	return cfg, nil
}

// EnsureDirectories creates required directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		c.Torrent.MetadataFolder,
		c.Subtitles.DownloadPath,
	}

	// Only create SQLite directory if Path is configured
	if c.Database.Path != "" {
		dirs = append(dirs, filepath.Dir(c.Database.Path))
	}

	// Add VFS cache directory if configured
	if c.VFS.CacheDir != "" {
		dirs = append(dirs, c.VFS.CacheDir)
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
