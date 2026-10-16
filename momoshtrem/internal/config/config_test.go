package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// envVars lists every variable applyEnv reads. Tests blank them first so the
// developer's shell can't leak into the results (an empty value means unset).
var envVars = []string{
	"BACKEND_API_TOKEN", "CORS_ALLOWED_ORIGINS", "WEBDAV_PUBLIC_URL",
	"WEBDAV_AUTH_ENABLED", "WEBDAV_USERNAME", "WEBDAV_PASSWORD",
	"DATABASE_URL", "METRICS_ENABLED", "METRICS_PORT", "TORRENT_LISTEN_PORT",
	"TMDB_API_KEY", "TMDB_LANGUAGE", "TMDB_RPS", "JACKETT_URL", "JACKETT_API_KEY", "ANTHROPIC_API_KEY", "RECS_DEBUG",
	"HDREZKA_ENABLED", "HDREZKA_URL",
	"TRAKT_CLIENT_ID", "TRAKT_CLIENT_SECRET", "SECRET_KEY",
	"OPENSUBTITLES_API_KEY", "OPENSUBTITLES_USERNAME", "OPENSUBTITLES_PASSWORD",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range envVars {
		t.Setenv(name, "")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_NoFileHonoursEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/momo")
	t.Setenv("TMDB_API_KEY", "tmdb-key")
	t.Setenv("TMDB_LANGUAGE", "de-DE")
	t.Setenv("TMDB_RPS", " 5 ")
	t.Setenv("BACKEND_API_TOKEN", "token")
	t.Setenv("WEBDAV_PUBLIC_URL", "https://example.org:36911/")
	t.Setenv("WEBDAV_AUTH_ENABLED", "true")
	t.Setenv("WEBDAV_USERNAME", "infuse")
	t.Setenv("WEBDAV_PASSWORD", "secret-pass")
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_PORT", "9191")
	t.Setenv("JACKETT_URL", "http://jackett:9117")
	t.Setenv("JACKETT_API_KEY", "jackett-key")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	t.Setenv("TRAKT_CLIENT_ID", "trakt-id")
	t.Setenv("TRAKT_CLIENT_SECRET", "trakt-secret")
	t.Setenv("SECRET_KEY", "aes-key")
	t.Setenv("OPENSUBTITLES_API_KEY", "os-key")

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"Database.URL", cfg.Database.URL, "postgres://u:p@db:5432/momo"},
		{"TMDB.APIKey", cfg.TMDB.APIKey, "tmdb-key"},
		{"TMDB.Language", cfg.TMDB.Language, "de-DE"},
		{"TMDB.RPS", cfg.TMDB.RPS, 5},
		{"Server.APIToken", cfg.Server.APIToken, "token"},
		{"Server.WebDAVPublicURL", cfg.Server.WebDAVPublicURL, "https://example.org:36911/"},
		{"WebDAVAuth.Enabled", cfg.Server.WebDAVAuth.Enabled, true},
		{"WebDAVAuth.Username", cfg.Server.WebDAVAuth.Username, "infuse"},
		{"WebDAVAuth.Password", cfg.Server.WebDAVAuth.Password, "secret-pass"},
		{"Metrics.Enabled", cfg.Metrics.Enabled, true},
		{"Metrics.Port", cfg.Metrics.Port, 9191},
		{"Jackett.URL", cfg.Jackett.URL, "http://jackett:9117"},
		{"Jackett.APIKey", cfg.Jackett.APIKey, "jackett-key"},
		{"AI.AnthropicAPIKey", cfg.AI.AnthropicAPIKey, "sk-ant"},
		{"Trakt.ClientID", cfg.Trakt.ClientID, "trakt-id"},
		{"Trakt.ClientSecret", cfg.Trakt.ClientSecret, "trakt-secret"},
		{"Security.SecretKey", cfg.Security.SecretKey, "aes-key"},
		{"OpenSubtitles.APIKey", cfg.OpenSubtitles.APIKey, "os-key"},
		// Defaults survive when neither file nor env sets them.
		{"Server.HTTPPort", cfg.Server.HTTPPort, 4444},
		{"Server.WebDAVPort", cfg.Server.WebDAVPort, 36911},
		{"Server.CORSAllowedOrigins", len(cfg.Server.CORSAllowedOrigins), 0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoad_FileThenEnvPrecedence(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
server:
  http_port: 5555
  api_token: file-token
  cors_allowed_origins: ["https://file.example"]
database:
  url: postgres://file/momo
tmdb:
  api_key: file-tmdb
  language: ja-JP
  rps: 8
jackett:
  url: http://file-jackett:9117
  api_key: file-jackett
metrics:
  enabled: true
  port: 9000
`)
	t.Setenv("DATABASE_URL", "postgres://env/momo")
	t.Setenv("BACKEND_API_TOKEN", "env-token")
	t.Setenv("METRICS_ENABLED", "false")
	t.Setenv("TMDB_RPS", "12")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		// Env wins over the file.
		{"Database.URL", cfg.Database.URL, "postgres://env/momo"},
		{"Server.APIToken", cfg.Server.APIToken, "env-token"},
		{"Metrics.Enabled", cfg.Metrics.Enabled, false},
		{"TMDB.RPS", cfg.TMDB.RPS, 12},
		// File wins over defaults when env is unset.
		{"Server.HTTPPort", cfg.Server.HTTPPort, 5555},
		{"TMDB.APIKey", cfg.TMDB.APIKey, "file-tmdb"},
		{"TMDB.Language", cfg.TMDB.Language, "ja-JP"},
		{"Jackett.URL", cfg.Jackett.URL, "http://file-jackett:9117"},
		{"Jackett.APIKey", cfg.Jackett.APIKey, "file-jackett"},
		{"Metrics.Port", cfg.Metrics.Port, 9000},
		// Defaults survive when neither sets them.
		{"Server.WebDAVPort", cfg.Server.WebDAVPort, 36911},
		{"Torrent.AddTimeout", cfg.Torrent.AddTimeout, 60},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if want := []string{"https://file.example"}; !reflect.DeepEqual(cfg.Server.CORSAllowedOrigins, want) {
		t.Errorf("CORSAllowedOrigins = %v, want %v", cfg.Server.CORSAllowedOrigins, want)
	}
}

func TestLoad_CORSAllowedOrigins(t *testing.T) {
	tests := []struct {
		name string
		env  string
		file string
		want []string
	}{
		{"unset", "", "", nil},
		{"single", "https://a.example", "", []string{"https://a.example"}},
		{"trimmed and empty entries dropped", " https://a.example , ,http://localhost:3000,", "", []string{"https://a.example", "http://localhost:3000"}},
		{"trailing slash stripped", "https://a.example/", "", []string{"https://a.example"}},
		{"lowercased", "HTTPS://A.Example:8443", "", []string{"https://a.example:8443"}},
		{"env replaces file list", "https://env.example", `["https://file.example", "https://other.example"]`, []string{"https://env.example"}},
		{"file list normalised", "", `[" https://file.example/ ", ""]`, []string{"https://file.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			t.Setenv("CORS_ALLOWED_ORIGINS", tt.env)
			path := ""
			if tt.file != "" {
				path = writeConfig(t, "server:\n  cors_allowed_origins: "+tt.file+"\n")
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !reflect.DeepEqual(cfg.Server.CORSAllowedOrigins, tt.want) {
				t.Errorf("CORSAllowedOrigins = %#v, want %#v", cfg.Server.CORSAllowedOrigins, tt.want)
			}
		})
	}
}

func TestLoad_InvalidCORSOrigins(t *testing.T) {
	tests := []struct {
		name string
		env  string
		file string
	}{
		{"wildcard", "*", ""},
		{"wildcard among others", "https://a.example,*", ""},
		{"no scheme", "a.example", ""},
		{"host and port without scheme", "localhost:3000", ""},
		{"unsupported scheme", "ftp://a.example", ""},
		{"path", "https://a.example/app", ""},
		{"query", "https://a.example?x=1", ""},
		{"fragment", "https://a.example#top", ""},
		{"userinfo", "https://user@a.example", ""},
		{"no host", "https://", ""},
		{"wildcard in file", "", `["*"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			t.Setenv("CORS_ALLOWED_ORIGINS", tt.env)
			path := ""
			if tt.file != "" {
				path = writeConfig(t, "server:\n  cors_allowed_origins: "+tt.file+"\n")
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
				t.Fatalf("Load: want a CORS_ALLOWED_ORIGINS error, got %v", err)
			}
		})
	}
}

func TestLoad_PortRanges(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr string // "" means valid
	}{
		{"defaults are valid", "", ""},
		{"http port zero", "server:\n  http_port: 0\n", "server.http_port"},
		{"webdav port too high", "server:\n  webdav_port: 70000\n", "server.webdav_port"},
		{"metrics port checked when enabled", "metrics:\n  enabled: true\n  port: -1\n", "metrics.port"},
		{"metrics port ignored when disabled", "metrics:\n  enabled: false\n  port: 0\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			path := ""
			if tt.file != "" {
				path = writeConfig(t, tt.file)
			}
			_, err := Load(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load: want an error naming %s, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoad_TorrentListenPort(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		env     string // TORRENT_LISTEN_PORT
		want    int
		wantErr string // "" means valid
	}{
		{"default", "", "", 42069, ""},
		{"file", "torrent:\n  listen_port: 51413\n", "", 51413, ""},
		{"zero in file picks a free port", "torrent:\n  listen_port: 0\n", "", 0, ""},
		{"env wins over file", "torrent:\n  listen_port: 51413\n", "6881", 6881, ""},
		{"zero in env", "", "0", 0, ""},
		{"blank env keeps the file value", "torrent:\n  listen_port: 51413\n", " ", 51413, ""},
		{"env not a number", "", "any", 0, "TORRENT_LISTEN_PORT"},
		{"env negative", "", "-1", 0, "TORRENT_LISTEN_PORT"},
		{"env too high", "", "65536", 0, "TORRENT_LISTEN_PORT"},
		{"file too high", "torrent:\n  listen_port: 70000\n", "", 0, "torrent.listen_port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			t.Setenv("TORRENT_LISTEN_PORT", tt.env)
			path := ""
			if tt.file != "" {
				path = writeConfig(t, tt.file)
			}
			cfg, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load: want an error naming %s, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Torrent.ListenPort != tt.want {
				t.Errorf("Torrent.ListenPort = %d, want %d", cfg.Torrent.ListenPort, tt.want)
			}
		})
	}
}

func TestLoad_TMDBSettings(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		env      map[string]string
		wantLang string
		wantRPS  int
		wantErr  string // "" means valid
	}{
		{"defaults", "", nil, "en-US", 20, ""},
		{"file", "tmdb:\n  language: fr-FR\n  rps: 4\n", nil, "fr-FR", 4, ""},
		{"env wins over file", "tmdb:\n  language: fr-FR\n  rps: 4\n", map[string]string{"TMDB_LANGUAGE": "pt-BR", "TMDB_RPS": "9"}, "pt-BR", 9, ""},
		{"empty env keeps the file value", "tmdb:\n  language: fr-FR\n", map[string]string{"TMDB_LANGUAGE": " "}, "fr-FR", 20, ""},
		{"rps zero in file", "tmdb:\n  rps: 0\n", nil, "", 0, "tmdb.rps"},
		{"empty language in file", "tmdb:\n  language: \"\"\n", nil, "", 0, "tmdb.language"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			path := ""
			if tt.file != "" {
				path = writeConfig(t, tt.file)
			}
			cfg, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load: want an error naming %s, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.TMDB.Language != tt.wantLang || cfg.TMDB.RPS != tt.wantRPS {
				t.Errorf("TMDB = (%q, %d), want (%q, %d)", cfg.TMDB.Language, cfg.TMDB.RPS, tt.wantLang, tt.wantRPS)
			}
		})
	}
}

func TestLoad_HDRezka(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		env         map[string]string
		wantEnabled bool
		wantURL     string
		wantErr     string // "" means valid
	}{
		{"defaults", "", nil, false, "https://rezka.ag", ""},
		{"file", "hdrezka:\n  enabled: true\n  url: https://hdrezka.me\n", nil, true, "https://hdrezka.me", ""},
		{"env wins over file", "hdrezka:\n  url: https://hdrezka.me\n", map[string]string{"HDREZKA_ENABLED": "true", "HDREZKA_URL": "http://mirror.example:8080"}, true, "http://mirror.example:8080", ""},
		{"trailing slash trimmed", "", map[string]string{"HDREZKA_URL": "https://rezka.ag/"}, false, "https://rezka.ag", ""},
		{"enabled not a bool", "", map[string]string{"HDREZKA_ENABLED": "sometimes"}, false, "", "HDREZKA_ENABLED"},
		{"no scheme", "", map[string]string{"HDREZKA_URL": "rezka.ag"}, false, "", "HDREZKA_URL"},
		{"unsupported scheme", "", map[string]string{"HDREZKA_URL": "ftp://rezka.ag"}, false, "", "HDREZKA_URL"},
		{"path", "", map[string]string{"HDREZKA_URL": "https://rezka.ag/films"}, false, "", "HDREZKA_URL"},
		{"query", "", map[string]string{"HDREZKA_URL": "https://rezka.ag?x=1"}, false, "", "HDREZKA_URL"},
		{"empty in file", "hdrezka:\n  url: \"\"\n", nil, false, "", "HDREZKA_URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			path := ""
			if tt.file != "" {
				path = writeConfig(t, tt.file)
			}
			cfg, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load: want an error naming %s, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.HDRezka.Enabled != tt.wantEnabled || cfg.HDRezka.URL != tt.wantURL {
				t.Errorf("HDRezka = %+v, want enabled %v, url %q", cfg.HDRezka, tt.wantEnabled, tt.wantURL)
			}
		})
	}
}

// The recommendations debug route is off unless the file or env turns it
// on (#153).
func TestLoad_RecsDebug(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		env     map[string]string
		want    bool
		wantErr string // "" means valid
	}{
		{"off by default", "", nil, false, ""},
		{"file", "ai:\n  recs_debug: true\n", nil, true, ""},
		{"env", "", map[string]string{"RECS_DEBUG": "true"}, true, ""},
		{"env wins over file", "ai:\n  recs_debug: true\n", map[string]string{"RECS_DEBUG": "false"}, false, ""},
		{"not a bool", "", map[string]string{"RECS_DEBUG": "yes please"}, false, "RECS_DEBUG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			path := ""
			if tt.file != "" {
				path = writeConfig(t, tt.file)
			}
			cfg, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load: want an error naming %s, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.AI.RecsDebug != tt.want {
				t.Errorf("AI.RecsDebug = %v, want %v", cfg.AI.RecsDebug, tt.want)
			}
		})
	}
}

func TestLoadWithoutDatabase(t *testing.T) {
	clearEnv(t)
	t.Setenv("HDREZKA_URL", "https://hdrezka.me")

	cfg, err := LoadWithoutDatabase("")
	if err != nil {
		t.Fatalf("LoadWithoutDatabase: %v", err)
	}
	if cfg.HDRezka.URL != "https://hdrezka.me" {
		t.Errorf("HDRezka.URL = %q", cfg.HDRezka.URL)
	}

	// Everything else is still checked.
	t.Setenv("HDREZKA_URL", "rezka.ag")
	if _, err := LoadWithoutDatabase(""); err == nil || !strings.Contains(err.Error(), "HDREZKA_URL") {
		t.Fatalf("LoadWithoutDatabase: want an HDREZKA_URL error, got %v", err)
	}
}

func TestLoad_EnvStringsTrimmed(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"surrounding whitespace trimmed", " abc \n", "abc"},
		{"whitespace only counts as unset", " \t\n", "file-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			t.Setenv("BACKEND_API_TOKEN", tt.env)
			path := writeConfig(t, "server:\n  api_token: file-token\n")
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Server.APIToken != tt.want {
				t.Errorf("Server.APIToken = %q, want %q", cfg.Server.APIToken, tt.want)
			}
		})
	}
}

func TestLoad_InvalidEnvValues(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{"metrics port not a number", "METRICS_PORT", "ninety"},
		{"metrics port out of range", "METRICS_PORT", "70000"},
		{"metrics port zero", "METRICS_PORT", "0"},
		{"metrics enabled not a bool", "METRICS_ENABLED", "maybe"},
		{"webdav auth enabled not a bool", "WEBDAV_AUTH_ENABLED", "yes please"},
		{"tmdb rps not a number", "TMDB_RPS", "fast"},
		{"tmdb rps zero", "TMDB_RPS", "0"},
		{"tmdb rps fractional", "TMDB_RPS", "2.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://env/momo")
			t.Setenv(tt.envName, tt.value)
			_, err := Load("")
			if err == nil {
				t.Fatalf("Load with %s=%q: want error, got nil", tt.envName, tt.value)
			}
			if !strings.Contains(err.Error(), tt.envName) {
				t.Errorf("error %q should name %s", err, tt.envName)
			}
		})
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearEnv(t)

	t.Run("no file, no env", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
			t.Fatalf("want DATABASE_URL error, got %v", err)
		}
	})

	t.Run("file without database url", func(t *testing.T) {
		path := writeConfig(t, "server:\n  http_port: 4444\n")
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
			t.Fatalf("want DATABASE_URL error, got %v", err)
		}
	})

	t.Run("file url satisfies it", func(t *testing.T) {
		path := writeConfig(t, "database:\n  url: postgres://file/momo\n")
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Database.URL != "postgres://file/momo" {
			t.Errorf("Database.URL = %q", cfg.Database.URL)
		}
	})
}

func TestLoad_ExampleFileParses(t *testing.T) {
	clearEnv(t)
	cfg, err := Load(filepath.Join("..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("config.yaml.example: %v", err)
	}
	if !strings.HasPrefix(cfg.Database.URL, "postgres://") {
		t.Errorf("example database.url = %q, want a postgres:// URL", cfg.Database.URL)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://env/momo")
	path := writeConfig(t, "server: [this is not a map")
	if _, err := Load(path); err == nil {
		t.Fatal("want parse error, got nil")
	}
}
