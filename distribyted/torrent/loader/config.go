package loader

import "github.com/distribyted/distribyted/config"

var _ Loader = &Config{}

type Config struct {
	c []*config.Route
}

func NewConfig(r []*config.Route) *Config {
	return &Config{
		c: r,
	}
}

func (l *Config) ListMagnets() (map[string][]TorrentWithMetadata, error) {
	out := make(map[string][]TorrentWithMetadata)
	for _, r := range l.c {
		for _, t := range r.Torrents {
			if t.MagnetURI == "" {
				continue
			}

			twm := TorrentWithMetadata{
				MagnetURI: t.MagnetURI,
				Metadata:  nil,
			}
			out[r.Name] = append(out[r.Name], twm)
		}
	}

	return out, nil
}

func (l *Config) ListTorrentPaths() (map[string][]string, error) {
	out := make(map[string][]string)
	for _, r := range l.c {
		for _, t := range r.Torrents {
			if t.TorrentPath == "" {
				continue
			}

			out[r.Name] = append(out[r.Name], t.TorrentPath)
		}
	}

	return out, nil
}
