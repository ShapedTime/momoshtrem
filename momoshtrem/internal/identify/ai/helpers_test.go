package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

// fakeClient stands in for the Messages API: tests never touch the network.
type fakeClient struct {
	mu      sync.Mutex
	calls   []anthropic.MessageNewParams
	respond func(call int, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

func (f *fakeClient) New(_ context.Context, params anthropic.MessageNewParams, _ ...option.RequestOption) (*anthropic.Message, error) {
	f.mu.Lock()
	f.calls = append(f.calls, params)
	n := len(f.calls)
	f.mu.Unlock()
	return f.respond(n, params)
}

func (f *fakeClient) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// answer is a successful response carrying files as the structured output,
// after an (empty) thinking block as Claude Haiku 5.5 returns by default.
func answer(t testing.TB, files ...identify.AIFile) *anthropic.Message {
	t.Helper()
	text, err := json.Marshal(map[string]any{"files": files})
	if err != nil {
		t.Fatal(err)
	}
	return message(t, "end_turn", string(text), "")
}

// message builds a response the way the SDK decodes one from the API.
func message(t testing.TB, stopReason, text, refusalCategory string) *anthropic.Message {
	t.Helper()
	raw := map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": DefaultModel,
		"content": []any{
			map[string]any{"type": "thinking", "thinking": "", "signature": "sig"},
			map[string]any{"type": "text", "text": text},
		},
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens": 1000, "output_tokens": 200,
			"cache_read_input_tokens": 2000, "cache_creation_input_tokens": 0,
		},
	}
	if refusalCategory != "" {
		raw["stop_details"] = map[string]any{"type": "refusal", "category": refusalCategory, "explanation": "declined"}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var m anthropic.Message
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

// sdkError is the error the SDK returns for an HTTP error response.
func sdkError(status int, errType string) error {
	e := &anthropic.Error{}
	_ = e.UnmarshalJSON([]byte(fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":"boom"}}`, errType)))
	e.StatusCode = status
	e.RequestID = "req_test"
	e.Request = httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	e.Response = &http.Response{StatusCode: status}
	return fmt.Errorf("POST: %w", e)
}

// testShow is a Frieren-like show: two specials, and one season of 32
// episodes that aired in two parts (E29 starts the second, years later).
func testShow() *identify.ShowCatalog {
	cat := &identify.ShowCatalog{
		Title: "Frieren: Beyond Journey's End", OriginalTitle: "Sousou no Frieren", Aliases: []string{"Frieren"},
		Year: 2023, IsAnime: true, Ordering: identify.OrderingAired, Countries: []string{"JP"},
	}
	cat.Episodes = append(cat.Episodes,
		identify.Episode{ID: 1, Season: 0, Number: 1, Name: "Mini Anime", AirDate: "2023-10-06"},
		identify.Episode{ID: 2, Season: 0, Number: 2, Name: "Recap", AirDate: "2024-03-15"},
	)
	day := time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC)
	for e := 1; e <= 32; e++ {
		if e == 29 {
			day = time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
		}
		cat.Episodes = append(cat.Episodes, identify.Episode{
			ID: int64(100 + e), Season: 1, Number: e, Absolute: e,
			Name: fmt.Sprintf("Episode %d", e), AirDate: day.Format("2006-01-02"),
		})
		day = day.AddDate(0, 0, 7)
	}
	return cat
}

const folder = "[SubsPlease] Frieren S2 (01-04)"

// testInput is a four-episode "S2" batch the rules half placed: two high,
// one medium, one unmatched, with a subtitle and a font.
func testInput() identify.ResolveInput {
	tgt := func(e int) []identify.Target {
		return []identify.Target{{EpisodeID: int64(100 + e), Season: 1, Episode: e}}
	}
	rules := []identify.Proposal{
		{Path: folder + "/Frieren S2 - 01.mkv", Size: 1 << 30, Kind: identify.KindEpisode, Targets: tgt(29), Confidence: identify.ConfidenceHigh, Source: identify.SourceRules, Reason: "S2 is the second part"},
		{Path: folder + "/Frieren S2 - 02.mkv", Size: 1 << 30, Kind: identify.KindEpisode, Targets: tgt(30), Confidence: identify.ConfidenceHigh, Source: identify.SourceRules, Reason: "S2 is the second part"},
		{Path: folder + "/Frieren S2 - 03.mkv", Size: 1 << 30, Kind: identify.KindEpisode, Targets: tgt(3), Confidence: identify.ConfidenceMedium, Source: identify.SourceRules, Reason: "03 read in season 1"},
		{Path: folder + "/Frieren S2 - 04.mkv", Size: 1 << 30, Kind: identify.KindUnknown, Source: identify.SourceRules, Reason: "No episode 4 in season 2"},
		{Path: folder + "/Frieren S2 - 04.en.srt", Size: 40 << 10, Kind: identify.KindSubtitle, LinkedVideo: folder + "/Frieren S2 - 04.mkv", Source: identify.SourceRules},
		{Path: folder + "/Fonts/a.ttf", Size: 200 << 10, Kind: identify.KindExtra, ExtraType: identify.ExtraFont, Source: identify.SourceRules},
	}
	in := identify.ResolveInput{TorrentName: folder, Show: testShow(), Rules: rules}
	for _, p := range rules {
		in.Files = append(in.Files, identify.TorrentFile{Path: p.Path, Size: p.Size})
	}
	return in
}

func intp(n int) *int { return &n }

// memCache is an in-memory Cache.
type memCache struct {
	mu   sync.Mutex
	data map[string][]byte
	gets int
}

func (c *memCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	v, ok := c.data[key]
	return v, ok, nil
}

func (c *memCache) Put(_ context.Context, key string, payload []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string][]byte{}
	}
	c.data[key] = payload
	return nil
}

// recorder is a UsageRecorder that keeps what it's given.
type recorder struct {
	mu     sync.Mutex
	usages []identify.Usage
	refs   []int64
}

func (r *recorder) Record(_ context.Context, u identify.Usage, ref int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usages = append(r.usages, u)
	r.refs = append(r.refs, ref)
	return nil
}

type budgetFunc func(context.Context) error

func (f budgetFunc) Check(ctx context.Context) error { return f(ctx) }
