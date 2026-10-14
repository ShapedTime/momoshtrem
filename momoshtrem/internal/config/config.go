package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
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
	Jackett       JackettConfig       `yaml:"jackett"`
	AI            AIConfig            `yaml:"ai"`
	Trakt         TraktConfig         `yaml:"trakt"`
	Security      SecurityConfig      `yaml:"security"`
}

type ServerConfig struct {
	HTTPPort   int              `yaml:"http_port"`
	WebDAVPort int              `yaml:"webdav_port"`
	WebDAVAuth WebDAVAuthConfig `yaml:"webdav_auth"`

	// APIToken is the shared secret required in the X-Momoshtrem-Token header
	// on every /api route. Empty disables the check (startup warning).
	APIToken string `yaml:"api_token"`
	// CORSAllowedOrigins lists browser origins allowed to call the REST API.
	// Empty means no CORS headers at all (server-to-server only).
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	// WebDAVPublicURL is the address Infuse uses to reach WebDAV, e.g.
	// https://momoshtrem.duckdns.org:36911/. Reported by /api/status.
	WebDAVPublicURL string `yaml:"webdav_public_url"`
}

// WebDAVAuthConfig configures authentication for the WebDAV server
type WebDAVAuthConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable Basic Auth (default: false)
	Username string `yaml:"username"` // Username for Basic Auth
	Password string `yaml:"password"` // Password for Basic Auth
}

// DatabaseConfig configures the Postgres connection.
type DatabaseConfig struct {
	URL string `yaml:"url"` // Postgres connection string (required)
}

type TorrentConfig struct {
	MetadataFolder       string `yaml:"metadata_folder"`
	GlobalCacheSize      int64  `yaml:"global_cache_size"` // MB
	AddTimeout           int    `yaml:"add_timeout"`       // seconds
	ReadTimeout          int    `yaml:"read_timeout"`      // seconds
	IdleEnabled          bool   `yaml:"idle_enabled"`
	IdleTimeout          int    `yaml:"idle_timeout"` // seconds
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

// JackettConfig configures the Jackett Torznab indexer aggregator.
type JackettConfig struct {
	URL    string `yaml:"url"`     // e.g. http://jackett:9117
	APIKey string `yaml:"api_key"` // From the Jackett dashboard
}

// AIConfig configures Anthropic-backed features. They are disabled when the
// key is empty.
type AIConfig struct {
	AnthropicAPIKey string `yaml:"anthropic_api_key"`
}

// TraktConfig configures the read-only Trakt integration.
type TraktConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

// SecurityConfig holds keys used to protect data at rest.
type SecurityConfig struct {
	SecretKey string `yaml:"secret_key"` // AES-GCM key for encrypting tokens at rest
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
			URL: "", // Required: DATABASE_URL env var or database.url in config.yaml
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
			MaxUnverifiedMB:      8,
		},
		VFS: VFSConfig{
			TreeTTL:  0,              // DEPRECATED: ignored
			CacheDir: "./data/cache", // Persistent VFS tree cache
		},
		Streaming: StreamingConfig{
			HeaderPriorityBytes: 10 * 1024 * 1024, // 10MB
			FooterPriorityBytes: 5 * 1024 * 1024,  // 5MB
			ReadaheadBytes:      64 * 1024 * 1024, // 64MB
			UrgentBufferBytes:   8 * 1024 * 1024,  // 8MB
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

// Load builds the configuration in three layers: defaults, then the optional
// YAML file at path, then environment variables. Environment overrides are
// applied whether or not the file exists, so a container can be configured
// with env vars alone. The result is validated before it is returned.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
			}
		case errors.Is(err, fs.ErrNotExist):
			slog.Info("Config file not found, using defaults and environment", "path", path)
		default:
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	cfg.Server.CORSAllowedOrigins = normalizeOrigins(cfg.Server.CORSAllowedOrigins)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnv overrides cfg with any non-empty environment variables.
// An empty or whitespace-only variable means "not set" and leaves the file or
// default value alone.
func applyEnv(cfg *Config) error {
	// Server and REST API security
	envString("BACKEND_API_TOKEN", &cfg.Server.APIToken)
	envList("CORS_ALLOWED_ORIGINS", &cfg.Server.CORSAllowedOrigins)
	envString("WEBDAV_PUBLIC_URL", &cfg.Server.WebDAVPublicURL)

	// WebDAV auth
	if err := envBool("WEBDAV_AUTH_ENABLED", &cfg.Server.WebDAVAuth.Enabled); err != nil {
		return err
	}
	envString("WEBDAV_USERNAME", &cfg.Server.WebDAVAuth.Username)
	envString("WEBDAV_PASSWORD", &cfg.Server.WebDAVAuth.Password)

	// Database
	envString("DATABASE_URL", &cfg.Database.URL)

	// Metrics
	if err := envBool("METRICS_ENABLED", &cfg.Metrics.Enabled); err != nil {
		return err
	}
	if err := envPort("METRICS_PORT", &cfg.Metrics.Port); err != nil {
		return err
	}

	// External services
	envString("TMDB_API_KEY", &cfg.TMDB.APIKey)
	envString("JACKETT_URL", &cfg.Jackett.URL)
	envString("JACKETT_API_KEY", &cfg.Jackett.APIKey)
	envString("ANTHROPIC_API_KEY", &cfg.AI.AnthropicAPIKey)
	envString("TRAKT_CLIENT_ID", &cfg.Trakt.ClientID)
	envString("TRAKT_CLIENT_SECRET", &cfg.Trakt.ClientSecret)
	envString("SECRET_KEY", &cfg.Security.SecretKey)

	// OpenSubtitles
	envString("OPENSUBTITLES_API_KEY", &cfg.OpenSubtitles.APIKey)
	envString("OPENSUBTITLES_USERNAME", &cfg.OpenSubtitles.Username)
	envString("OPENSUBTITLES_PASSWORD", &cfg.OpenSubtitles.Password)

	return nil
}

func envString(name string, dst *string) {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		*dst = v
	}
}

func envBool(name string, dst *bool) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return fmt.Errorf("invalid %s %q: want true or false", name, v)
	}
	*dst = b
	return nil
}

func envPort(name string, dst *int) error {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid %s %q: want a port number between 1 and 65535", name, v)
	}
	*dst = port
	return nil
}

// envList parses a comma-separated variable, trimming spaces and dropping
// empty entries.
func envList(name string, dst *[]string) {
	v := os.Getenv(name)
	if v == "" {
		return
	}
	*dst = splitList(v)
}

func splitList(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// normalizeOrigins trims and lowercases entries, drops empty ones and strips a
// trailing slash, matching the Origin header browsers send.
func normalizeOrigins(origins []string) []string {
	var out []string
	for _, o := range origins {
		o = strings.ToLower(strings.TrimRight(strings.TrimSpace(o), "/"))
		if o != "" {
			out = append(out, o)
		}
	}
	return out
}

// Validate reports configuration that would prevent the service from starting.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Database.URL) == "" {
		return errors.New("DATABASE_URL is required (set the DATABASE_URL environment variable or database.url in the config file)")
	}

	if err := checkPort("server.http_port", c.Server.HTTPPort); err != nil {
		return err
	}
	if err := checkPort("server.webdav_port", c.Server.WebDAVPort); err != nil {
		return err
	}
	if c.Metrics.Enabled {
		if err := checkPort("metrics.port", c.Metrics.Port); err != nil {
			return err
		}
	}

	for _, o := range c.Server.CORSAllowedOrigins {
		if err := validateOrigin(o); err != nil {
			return fmt.Errorf("invalid CORS_ALLOWED_ORIGINS entry %q: %w", o, err)
		}
	}
	return nil
}

func checkPort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid %s %d: want a port number between 1 and 65535", name, port)
	}
	return nil
}

// validateOrigin accepts only what a browser sends in the Origin header:
// http(s)://host[:port], with nothing after it. "*" is rejected: the
// middleware matches origins exactly, so it would silently allow nothing.
func validateOrigin(origin string) error {
	if origin == "*" {
		return errors.New("wildcard is not allowed; list each origin")
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Scheme+"://"+u.Host != origin {
		return errors.New("want http(s)://host[:port] with no path, query or fragment")
	}
	return nil
}

// EnsureDirectories creates required directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		c.Torrent.MetadataFolder,
		c.Subtitles.DownloadPath,
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
