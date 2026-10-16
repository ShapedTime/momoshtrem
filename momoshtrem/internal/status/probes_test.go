package status

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/torznab"
)

const testJackettKey = "jackett-secret-key"

const twoIndexers = `<?xml version="1.0" encoding="UTF-8"?>
<indexers>
  <indexer id="thepiratebay" configured="true">
    <title>The Pirate Bay</title>
    <caps><searching><search available="yes" supportedParams="q" /></searching></caps>
  </indexer>
  <indexer id="eztv" configured="true">
    <title>EZTV</title>
    <caps><searching><search available="yes" supportedParams="q" /></searching></caps>
  </indexer>
</indexers>`

func TestJackettProber(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string // "" means success
	}{
		{"indexers listed", http.StatusOK, twoIndexers, ""},
		{"torznab error document", http.StatusOK,
			`<?xml version="1.0" encoding="UTF-8"?><error code="100" description="Invalid API Key" />`, "Invalid API Key"},
		{"unauthorized status", http.StatusUnauthorized, `nope`, "HTTP 401"},
		{"server error", http.StatusInternalServerError, `<html>oops</html>`, "HTTP 500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotT, gotConfigured, gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotT, gotConfigured = r.URL.Query().Get("t"), r.URL.Query().Get("configured")
				gotKey = r.URL.Query().Get("apikey")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			probe := JackettProber(torznab.New(srv.URL+"/", testJackettKey))
			indexers, err := probe(context.Background())

			if gotPath != "/api/v2.0/indexers/all/results/torznab/api" || gotT != "indexers" ||
				gotConfigured != "true" || gotKey != testJackettKey {
				t.Errorf("request = %s t=%s configured=%s apikey=%s", gotPath, gotT, gotConfigured, gotKey)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				if len(indexers) != 2 || indexers[0].Name != "The Pirate Bay" || indexers[1].Name != "EZTV" {
					t.Fatalf("indexers = %+v", indexers)
				}
				for _, ix := range indexers {
					if ix.Status != "unknown" || ix.MS != nil || ix.Since != nil || ix.Error != "" {
						t.Errorf("before any search: %+v, want unknown", ix)
					}
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), testJackettKey) {
				t.Errorf("error leaks the API key: %v", err)
			}
		})
	}
}

func TestJackettProber_TransportErrorHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused from here on

	_, err := JackettProber(torznab.New(url, testJackettKey))(context.Background())
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), testJackettKey) || strings.Contains(err.Error(), "apikey") {
		t.Errorf("error leaks the request URL: %v", err)
	}
}

func TestJackettProber_HonoursContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := JackettProber(torznab.New(srv.URL, testJackettKey))(ctx); err == nil {
		t.Fatal("want an error for a cancelled context")
	}
}

// fakeSource is a JackettSource with fixed health.
type fakeSource struct {
	health []torznab.IndexerHealth
}

func (f fakeSource) FetchIndexers(context.Context) ([]torznab.Indexer, error) { return nil, nil }
func (f fakeSource) Snapshot() []torznab.IndexerHealth                        { return f.health }

func TestJackettProber_MergesHealth(t *testing.T) {
	ms, slow := int64(412), int64(8000)
	since := time.Date(2026, 10, 16, 10, 0, 0, 0, time.FixedZone("GST", 4*3600))
	indexers, err := JackettProber(fakeSource{health: []torznab.IndexerHealth{
		{ID: "thepiratebay", Name: "The Pirate Bay", Status: torznab.StatusOK, MS: &ms},
		{ID: "1337x", Name: "1337x", Status: torznab.StatusTimeout, MS: &slow, Since: &since, Error: "timed out after 8s"},
		{ID: "eztv", Name: "EZTV", Status: torznab.StatusUnknown},
	}})(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok := indexers[0]; ok.Status != "ok" || *ok.MS != 412 || ok.Since != nil || ok.Error != "" {
		t.Errorf("ok = %+v", ok)
	}
	if to := indexers[1]; to.Status != "timeout" || *to.MS != 8000 || to.Since == nil ||
		*to.Since != "2026-10-16T06:00:00Z" || to.Error != "timed out after 8s" {
		t.Errorf("timeout = %+v (since %v)", to, to.Since)
	}
	if u := indexers[2]; u.Status != "unknown" || u.MS != nil || u.Since != nil {
		t.Errorf("unknown = %+v", u)
	}
}
