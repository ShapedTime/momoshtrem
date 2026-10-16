package api

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
)

// storageCache is a piece cache for the integration test: its cap and fill.
type storageCache struct {
	mu               sync.Mutex
	capacity, filled int64
}

func (c *storageCache) funcs() storagecap.CacheFuncs {
	return storagecap.CacheFuncs{
		InfoFunc: func() storagecap.CacheInfo {
			c.mu.Lock()
			defer c.mu.Unlock()
			return storagecap.CacheInfo{Capacity: c.capacity, Filled: c.filled}
		},
		SetCapacityFunc: func(n int64) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.capacity = n
		},
		TrimToCapacityFunc: func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.filled = min(c.filled, c.capacity)
		},
	}
}

type committedKept int64

func (c committedKept) DownloadUsage(context.Context) (storagecap.DownloadUsage, error) {
	return storagecap.DownloadUsage{CommittedKeptBytes: int64(c)}, nil
}

// /settings/storage end to end (#200): the routes over the Governor and
// Postgres, on a 200 GiB disk with 160 free and config.yaml setting
// 52.5 GiB. The setting wins over config.yaml once sent, a null is auto,
// partial PUTs merge, and a bad value (422) or a kept cap below what's
// committed (409) changes nothing.
func TestStorage_Integration(t *testing.T) {
	db := openRerunDB(t)
	const gib = int64(1) << 30
	config := int64(53760) << 20
	g := storagecap.New(storagecap.Deps{
		Settings:    library.NewSettingsRepository(db),
		DiskPath:    "/data",
		ConfigBytes: &config,
		Statfs:      func(string) (int64, int64, error) { return 200 * gib, 160 * gib, nil },
	})
	cache := &storageCache{filled: 37 * gib}
	cache.capacity = g.StartupLimit(context.Background()).LimitBytes
	g.Start(context.Background(), cache.funcs())
	t.Cleanup(g.Close)
	s := newTestServer(Options{Storage: g})

	get := func(t *testing.T, method, body string) map[string]json.RawMessage {
		t.Helper()
		rec := doBody(t, s, method, "/api/settings/storage", body)
		var got map[string]json.RawMessage
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("%s %s = %d %s", method, body, rec.Code, rec.Body)
		}
		return got
	}
	check := func(t *testing.T, got map[string]json.RawMessage, want map[string]string) {
		t.Helper()
		for field, value := range want {
			if string(got[field]) != value {
				t.Errorf("%s = %s, want %s", field, got[field], value)
			}
		}
	}

	// A fresh install: config.yaml's limit, the defaults.
	got := get(t, "GET", "")
	check(t, got, map[string]string{
		"limit_bytes": `null`, "source": `"config"`, "config_bytes": `56371445760`, "auto_bytes": `161061273600`,
		"limit_effective_bytes": `56371445760`, "effective_bytes": `56371445760`, "floored": `false`,
		"disk":       `{"total_bytes":214748364800,"free_bytes":171798691840}`,
		"usage":      `{"kept_bytes":0,"smart_bytes":0,"cache_bytes":39728447488}`,
		"kept":       `{"max_bytes":null,"effective_max_bytes":28185722880,"default_until_watched":true,"default_days":30}`,
		"smart":      `{"enabled":false,"episodes_ahead":2,"max_bytes":null,"effective_max_bytes":8455716864}`,
		"background": `{"enabled":true,"pause_while_streaming":true,"windows":[]}`,
	})

	// Nested fields merge; the limit is untouched by them, so config.yaml
	// still applies.
	get(t, "PUT", `{"smart":{"enabled":true},"background":{"windows":[{"start":"23:00","end":"06:00"}]}}`)
	got = get(t, "PUT", `{"smart":{"episodes_ahead":4},"kept":{"default_days":null}}`)
	check(t, got, map[string]string{
		"source":     `"config"`,
		"kept":       `{"max_bytes":null,"effective_max_bytes":28185722880,"default_until_watched":true,"default_days":null}`,
		"smart":      `{"enabled":true,"episodes_ahead":4,"max_bytes":null,"effective_max_bytes":8455716864}`,
		"background": `{"enabled":true,"pause_while_streaming":true,"windows":[{"start":"23:00","end":"06:00"}]}`,
	})

	// The setting wins over config.yaml; a null is auto, 75% of the disk.
	got = get(t, "PUT", `{"limit_bytes":107374182400}`)
	check(t, got, map[string]string{"limit_bytes": `107374182400`, "source": `"setting"`, "effective_bytes": `107374182400`})
	got = get(t, "PUT", `{"limit_bytes":null}`)
	check(t, got, map[string]string{"limit_bytes": `null`, "source": `"auto"`, "limit_effective_bytes": `161061273600`,
		"reserve_bytes": `21474836480`, "kept": `{"max_bytes":null,"effective_max_bytes":80530636800,"default_until_watched":true,"default_days":null}`})
	if got := get(t, "GET", ""); string(got["source"]) != `"auto"` {
		t.Errorf("GET after the null = %s", got["source"])
	}

	// 422 names the field and changes nothing; the rest of a rejected PUT
	// isn't applied either.
	before := doBody(t, s, "GET", "/api/settings/storage", "").Body.String()
	for body, field := range map[string]string{
		`{"limit_bytes":1073741824}`:                                    "limit_bytes",
		`{"limit_bytes":999999999999999}`:                               "limit_bytes",
		`{"smart":{"enabled":false,"episodes_ahead":0}}`:                "smart.episodes_ahead",
		`{"kept":{"max_bytes":214748364800}}`:                           "kept.max_bytes",
		`{"kept":{"default_days":14}}`:                                  "kept.default_days",
		`{"background":{"enabled":false,"windows":[{"start":"7:00"}]}}`: "background.windows",
	} {
		rec := doBody(t, s, "PUT", "/api/settings/storage", body)
		if e := errorOf(t, rec.Body.Bytes()); rec.Code != 422 || e.Code != CodeValidationError || e.Field != field {
			t.Errorf("PUT %s = %d %s, want 422 on %s", body, rec.Code, rec.Body, field)
		}
	}
	if rec := doBody(t, s, "PUT", "/api/settings/storage", `"auto"`); rec.Code != 400 {
		t.Errorf("not an object = %d", rec.Code)
	}
	if after := doBody(t, s, "GET", "/api/settings/storage", "").Body.String(); after != before {
		t.Errorf("rejected PUTs changed the settings:\n%s\n%s", before, after)
	}

	// 409: kept downloads want 70 GiB; the kept cap is 75 of 150.
	g.SetDownloads(committedKept(70 * gib))
	rec := doBody(t, s, "PUT", "/api/settings/storage", `{"limit_bytes":107374182400}`) // kept 50 GiB
	var conflict struct {
		Error struct {
			Code               string `json:"code"`
			CommittedKeptBytes int64  `json:"committed_kept_bytes"`
		} `json:"error"`
	}
	if rec.Code != 409 || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil ||
		conflict.Error.Code != CodeLimitBelowCommitted || conflict.Error.CommittedKeptBytes != 70*gib {
		t.Errorf("409 = %d %s", rec.Code, rec.Body)
	}
	got = get(t, "PUT", `{"limit_bytes":161061273600,"kept":{"max_bytes":85899345920}}`) // 150 GiB, kept 80
	check(t, got, map[string]string{"source": `"setting"`, "committed_kept_bytes": `75161927680`})

	// The Repository reads the same document.
	stored, err := library.NewSettingsRepository(db).StorageSettings(context.Background())
	if err != nil || !stored.LimitSet || *stored.LimitBytes != 150*gib || *stored.Kept.MaxBytes != 80*gib || stored.Smart.EpisodesAhead != 4 {
		t.Errorf("stored = %+v, %v", stored, err)
	}
}
