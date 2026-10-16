package stack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streaming"
)

// loadConfig is the effective config: config.DefaultConfig(), or a YAML
// file laid over it by config.LoadWithoutDatabase (environment included,
// as production loads its own), then opts.Tune. The lab's folder goes in
// last: Tune can't move it.
func loadConfig(opts Options, dir string) (*config.Config, error) {
	cfg := config.DefaultConfig()
	if opts.ConfigPath != "" {
		var err error
		if cfg, err = config.LoadWithoutDatabase(opts.ConfigPath); err != nil {
			return nil, fmt.Errorf("stack: loading %s: %w", opts.ConfigPath, err)
		}
	}
	if opts.Tune != nil {
		opts.Tune(cfg)
	}
	cfg.Torrent.MetadataFolder = dir
	return cfg, nil
}

// streamingConfig is the prioritizer's config, as cmd/momoshtrem/main.go
// builds it: the defaults when none is set.
func streamingConfig(cfg *config.Config) streaming.Config {
	sc := streaming.Config{
		HeaderPriorityBytes: cfg.Streaming.HeaderPriorityBytes,
		FooterPriorityBytes: cfg.Streaming.FooterPriorityBytes,
		ReadaheadBytes:      cfg.Streaming.ReadaheadBytes,
		UrgentBufferBytes:   cfg.Streaming.UrgentBufferBytes,
	}
	if sc.IsZero() {
		sc = streaming.DefaultConfig()
	}
	return sc
}

// fingerprint names the config the streaming path ran with: a hash of the
// torrent settings, without the lab's folder and port, and the effective
// streaming config. Two runs with one fingerprint ran one configuration.
func fingerprint(tc config.TorrentConfig, sc streaming.Config) string {
	tc.MetadataFolder, tc.ListenPort = "", 0
	b, err := json.Marshal(struct {
		Torrent   config.TorrentConfig
		Streaming streaming.Config
	}{tc, sc})
	if err != nil {
		panic(err) // plain data: can't fail
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}
