package stack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/config"
)

// A YAML file lays over the defaults, Tune goes after it, the lab's folder
// last; the fingerprint follows the streaming path's settings, not the
// folder or the port.
func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prod.yaml")
	yaml := "torrent:\n  read_timeout: 90\n  metadata_folder: /data/torrents\nstreaming:\n  readahead_bytes: 33554432\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(Options{
		ConfigPath: path,
		Tune: func(c *config.Config) {
			c.Torrent.StartPaused = false
			c.Torrent.MetadataFolder = "/elsewhere"
		},
	}, "/lab")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Torrent.ReadTimeout != 90 || cfg.Streaming.ReadaheadBytes != 32<<20 || cfg.Torrent.StartPaused ||
		cfg.Torrent.MetadataFolder != "/lab" || cfg.Torrent.AddTimeout != config.DefaultConfig().Torrent.AddTimeout {
		t.Fatalf("config = %+v %+v", cfg.Torrent, cfg.Streaming)
	}

	def := config.DefaultConfig()
	base := fingerprint(def.Torrent, streamingConfig(def))
	moved := def.Torrent
	moved.MetadataFolder, moved.ListenPort = "/lab", 0
	if got := fingerprint(moved, streamingConfig(def)); got != base {
		t.Errorf("the folder and port change the fingerprint: %s, %s", got, base)
	}
	if got := fingerprint(cfg.Torrent, streamingConfig(cfg)); got == base {
		t.Error("another read timeout and readahead keep the fingerprint")
	}
	if _, err := loadConfig(Options{ConfigPath: filepath.Join(t.TempDir(), "bad.yaml")}, "/lab"); err != nil {
		t.Errorf("a missing file is the defaults, as in production: %v", err)
	}
}

// With no streaming config, the defaults apply, as in main.go.
func TestStreamingConfig_Defaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Streaming = config.StreamingConfig{}
	if got := streamingConfig(cfg); got.ReadaheadBytes == 0 || got.UrgentBufferBytes == 0 {
		t.Errorf("streamingConfig = %+v", got)
	}
}
