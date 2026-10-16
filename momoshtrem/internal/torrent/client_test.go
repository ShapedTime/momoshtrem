package torrent

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"

	"github.com/shapedtime/momoshtrem/internal/config"
)

// fakeStorage and fakeItemStore stand in for the client's components: the
// config only carries them.
type fakeStorage struct{ storage.ClientImpl }

type fakeItemStore struct{ bep44.Store }

func testClientConfig() (*config.TorrentConfig, *ClientConfig) {
	cfg := &config.TorrentConfig{
		ListenPort:           51413,
		DropDuplicatePeerIds: true,
		MaxUnverifiedMB:      8,
	}
	cc := &ClientConfig{
		Storage:   &fakeStorage{},
		ItemStore: &fakeItemStore{},
		PeerID:    [20]byte{'-', 'M', 'O', '0', '0', '0', '1', '-', 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
	}
	return cfg, cc
}

// TestClientConfigFor pins what ClientConfigFor sets on anacrolix's defaults:
// each setting's value, and that no other field differs. A new client
// setting fails it until it's listed here, so the streamlab lab (PLAN D3)
// knows what production runs with.
func TestClientConfigFor(t *testing.T) {
	cfg, cc := testClientConfig()
	got := ClientConfigFor(cfg, cc)

	for _, c := range []struct {
		field     string
		got, want any
	}{
		{"Seed", got.Seed, true},
		{"PeerID", got.PeerID, string(cc.PeerID[:])},
		{"DefaultStorage", got.DefaultStorage, cc.Storage},
		{"ListenPort", got.ListenPort, cfg.ListenPort},
		{"DisableIPv6", got.DisableIPv6, true},
		{"DropDuplicatePeerIds", got.DropDuplicatePeerIds, cfg.DropDuplicatePeerIds},
		{"MaxUnverifiedBytes", got.MaxUnverifiedBytes, int64(8 << 20)},
		// Production (CGO_ENABLED=0) runs anacrolix's pure-Go uTP; only the
		// lab turns it off.
		{"DisableUTP", got.DisableUTP, false},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}

	// Logger: anacrolix's records go to slog.
	if hs := got.Logger.Handlers; len(hs) != 1 {
		t.Errorf("Logger has %d handlers, want the slog bridge alone", len(hs))
	} else if _, ok := hs[0].(*torrentLogHandler); !ok {
		t.Errorf("Logger's handler is %T, want *torrentLogHandler", hs[0])
	}

	// ConfigureAnacrolixDhtServer: the item store, a 2 h expiry, security on.
	dhtCfg := &dht.ServerConfig{NoSecurity: true}
	got.ConfigureAnacrolixDhtServer(dhtCfg)
	if dhtCfg.Store != cc.ItemStore || dhtCfg.Exp != 2*time.Hour || dhtCfg.NoSecurity {
		t.Errorf("DHT server config: Store %v Exp %v NoSecurity %v; want the item store, 2h, false",
			dhtCfg.Store, dhtCfg.Exp, dhtCfg.NoSecurity)
	}

	want := []string{
		"ConfigureAnacrolixDhtServer", "DefaultStorage", "DisableIPv6", "DropDuplicatePeerIds",
		"ListenPort", "Logger", "MaxUnverifiedBytes", "PeerID", "Seed",
	}
	if diff := changedFields(atorrent.NewDefaultClientConfig(), got); !slices.Equal(diff, want) {
		t.Errorf("fields that differ from NewDefaultClientConfig = %v, want %v", diff, want)
	}

	// No cap configured: anacrolix's default stays.
	cfg.MaxUnverifiedMB = 0
	if got, def := ClientConfigFor(cfg, cc).MaxUnverifiedBytes, atorrent.NewDefaultClientConfig().MaxUnverifiedBytes; got != def {
		t.Errorf("MaxUnverifiedBytes with max_unverified_mb 0 = %d, want the default %d", got, def)
	}
}

// changedFields names the fields of b that differ from a's, sorted, looking
// inside the embedded configs. Two funcs are the same when they run the same
// code: the defaults make fresh closures on every call.
func changedFields(a, b *atorrent.ClientConfig) []string {
	var names []string
	var walk func(a, b reflect.Value)
	walk = func(a, b reflect.Value) {
		for i := range a.NumField() {
			f := a.Type().Field(i)
			switch {
			case !f.IsExported():
			case f.Anonymous && f.Type.Kind() == reflect.Struct:
				walk(a.Field(i), b.Field(i))
			case !sameValue(a.Field(i), b.Field(i)):
				names = append(names, f.Name)
			}
		}
	}
	walk(reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem())
	slices.Sort(names)
	return names
}

func sameValue(a, b reflect.Value) bool {
	if a.Kind() == reflect.Func {
		return a.IsNil() == b.IsNil() && (a.IsNil() || a.Pointer() == b.Pointer())
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}
