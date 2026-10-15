package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

func newTest(t *testing.T, client messagesAPI, cfg Config) *Resolver {
	t.Helper()
	r, err := newResolver(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewWithoutKey(t *testing.T) {
	r, err := New(Config{})
	if r != nil || err != nil {
		t.Errorf("New without a key = %v, %v; want nil, nil", r, err)
	}
	if _, err := newResolver(&fakeClient{}, Config{Model: "gpt-4o"}); err == nil {
		t.Error("a model without a price was accepted")
	}
	r, err = New(Config{APIKey: "sk-ant-test"})
	if err != nil || r == nil || r.Model() != DefaultModel {
		t.Errorf("New with a key = %v, %v", r, err)
	}
}

// The input's model (the classifier_model setting) wins over the
// resolver's; one without a price is refused before any call.
func TestResolveModelPerCall(t *testing.T) {
	rec := &recorder{}
	client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) { return answer(t), nil }}
	r := newTest(t, client, Config{Usage: rec})
	in := testInput()
	in.Model = "claude-sonnet-5-5"
	out, err := r.Resolve(context.Background(), in)
	if err != nil || client.calls[0].Model != "claude-sonnet-5-5" || out.Usage.Model != "claude-sonnet-5-5" || rec.usages[0].Model != "claude-sonnet-5-5" {
		t.Fatalf("Resolve = %+v, %v; request model %q", out.Usage, err, client.calls[0].Model)
	}
	// Priced on Sonnet's card: $2 in, $10 out, $0.20 cache reads.
	if want := (1000*2.0 + 200*10.0 + 2000*0.20) / 1e6; math.Abs(out.Usage.CostUSD-want) > 1e-12 {
		t.Errorf("cost = %v, want %v", out.Usage.CostUSD, want)
	}

	in.Model = "gpt-4o"
	if _, err := r.Resolve(context.Background(), in); !errors.Is(err, ErrUnknownModel) || client.count() != 1 {
		t.Errorf("an unknown model: %v after %d calls", err, client.count())
	}
}

func TestResolveNothingUnsure(t *testing.T) {
	client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) {
		return nil, errors.New("must not be called")
	}}
	r := newTest(t, client, Config{})
	in := testInput()
	in.Rules = in.Rules[:2] // two high answers
	in.Files = in.Files[:2]
	out, err := r.Resolve(context.Background(), in)
	if err != nil || len(out.Files) != 0 || client.count() != 0 {
		t.Errorf("Resolve = %+v, %v after %d calls; want no call", out, err, client.count())
	}
}

// TestResolveThroughMergeAI checks the answers' path mapping and that
// MergeAI validates them: nonexistent targets and overrides of high rules
// answers are rejected; good answers fill the gaps.
func TestResolveThroughMergeAI(t *testing.T) {
	client := &fakeClient{respond: func(_ int, p anthropic.MessageNewParams) (*anthropic.Message, error) {
		return answer(t,
			// A relative path, as listed: maps to S01E31.
			identify.AIFile{Path: "Frieren S2 - 03.mkv", Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{31}, AbsoluteNumber: intp(31), Confidence: identify.ConfidenceHigh, Reason: "S2 - 03 is the second part's third"},
			// The full path, an absolute number only.
			identify.AIFile{Path: folder + "/Frieren S2 - 04.mkv", Kind: identify.KindEpisode, AbsoluteNumber: intp(32), Confidence: identify.ConfidenceMedium, Reason: "AI: #32"},
			// Not asked about (a high rules answer): dropped.
			identify.AIFile{Path: "Frieren S2 - 01.mkv", Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{1}, Confidence: identify.ConfidenceHigh, Reason: "x"},
			// A duplicate: the first answer wins.
			identify.AIFile{Path: "Frieren S2 - 03.mkv", Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{3}, Confidence: identify.ConfidenceHigh, Reason: "x"},
			// A path that isn't in the torrent.
			identify.AIFile{Path: "Other - 05.mkv", Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{5}, Confidence: identify.ConfidenceHigh, Reason: "x"},
		), nil
	}}
	r := newTest(t, client, Config{})
	in := testInput()
	out, err := r.Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 2 || out.Files[0].Path != folder+"/Frieren S2 - 03.mkv" || out.Files[1].Path != folder+"/Frieren S2 - 04.mkv" {
		t.Fatalf("files = %+v", out.Files)
	}

	merged := identify.MergeAI(in.Rules, out.Files, in.Show)
	want := map[string]string{
		"Frieren S2 - 01.mkv":    "S01E29 high rules",
		"Frieren S2 - 03.mkv":    "S01E31 high ai",
		"Frieren S2 - 04.mkv":    "S01E32 medium ai",
		"Frieren S2 - 04.en.srt": "S01E32 medium ai",
	}
	for _, p := range merged {
		w, ok := want[strings.TrimPrefix(p.Path, folder+"/")]
		if !ok {
			continue
		}
		var labels []string
		for _, t := range p.Targets {
			labels = append(labels, label(t.Season, t.Episode))
		}
		if got := fmt.Sprintf("%s %s %s", strings.Join(labels, "+"), p.Confidence, p.Source); got != w {
			t.Errorf("%s = %s, want %s", p.Path, got, w)
		}
	}
	if r := merged[3].Reason; strings.HasPrefix(r, "AI") {
		t.Errorf("reason %q keeps its AI: prefix", r)
	}

	// MergeAI rejects what the model gets wrong: an episode that doesn't
	// exist, and overriding a high rules answer.
	bad := []identify.AIFile{
		{Path: in.Rules[3].Path, Kind: identify.KindEpisode, Season: intp(2), Episodes: []int{4}, Confidence: identify.ConfidenceHigh, Reason: "S02E04"},
		{Path: in.Rules[0].Path, Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{1}, Confidence: identify.ConfidenceHigh, Reason: "01"},
	}
	merged = identify.MergeAI(in.Rules, bad, in.Show)
	if merged[3].HasTarget() || merged[3].Source != identify.SourceRules {
		t.Errorf("a nonexistent target was accepted: %+v", merged[3])
	}
	if merged[0].Targets[0].Episode != 29 || merged[0].Source != identify.SourceRules {
		t.Errorf("a high rules answer was overridden: %+v", merged[0])
	}
}

func TestResolveUsageAndCost(t *testing.T) {
	rec := &recorder{}
	client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) {
		return answer(t), nil
	}}
	r := newTest(t, client, Config{Usage: rec})
	in := testInput()
	in.JobID = 42
	out, err := r.Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// 1,000 input at $0.10, 200 output at $0.50 and 2,000 cache reads at
	// $0.01 per million tokens.
	wantCost := (1000*0.10 + 200*0.50 + 2000*0.01) / 1e6
	want := identify.Usage{Model: DefaultModel, InputTokens: 1000, OutputTokens: 200, CacheReadTokens: 2000, CostUSD: wantCost}
	if math.Abs(out.Usage.CostUSD-wantCost) > 1e-12 {
		t.Errorf("cost = %v, want %v", out.Usage.CostUSD, wantCost)
	}
	out.Usage.CostUSD = wantCost
	if out.Usage != want {
		t.Errorf("usage = %+v, want %+v", out.Usage, want)
	}
	if len(rec.usages) != 1 || rec.refs[0] != 42 || rec.usages[0].InputTokens != 1000 {
		t.Errorf("recorded %+v for %v", rec.usages, rec.refs)
	}
}

func TestResolveSplitsBigScopes(t *testing.T) {
	rec := &recorder{}
	client := &fakeClient{respond: func(_ int, p anthropic.MessageNewParams) (*anthropic.Message, error) {
		// Answer the first file of each request.
		ask := p.Messages[0].Content[1].OfText.Text
		line := ask[strings.Index(ask, "1. ")+3:]
		path := line[:strings.Index(line, "\n")]
		return answer(t, identify.AIFile{Path: path, Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{1}, Confidence: identify.ConfidenceLow, Reason: "x"}), nil
	}}
	r := newTest(t, client, Config{Usage: rec})
	in := bigTorrent(130)
	in.Scope = Videos(in.Rules)
	out, err := r.Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if client.count() != 3 || len(out.Files) != 3 || len(rec.usages) != 3 {
		t.Fatalf("%d calls, %d answers, %d usages; want 3 each", client.count(), len(out.Files), len(rec.usages))
	}
	if out.Usage.InputTokens != 3000 || out.Usage.OutputTokens != 600 {
		t.Errorf("usage isn't summed: %+v", out.Usage)
	}
	// The answers come back in the files' order, whichever request ends
	// first.
	if out.Files[0].Path != "Big/Show - 001.mkv" || out.Files[1].Path != "Big/Show - 061.mkv" || out.Files[2].Path != "Big/Show - 121.mkv" {
		t.Errorf("answers out of order: %s, %s, %s", out.Files[0].Path, out.Files[1].Path, out.Files[2].Path)
	}

	// One failing request fails the run, without answers, but the usage
	// of those that answered is still returned.
	failing := &fakeClient{respond: func(_ int, p anthropic.MessageNewParams) (*anthropic.Message, error) {
		if strings.Contains(p.Messages[0].Content[1].OfText.Text, "Part 2 of 3") {
			return nil, sdkError(529, "overloaded_error")
		}
		return answer(t), nil
	}}
	out, err = newTest(t, failing, Config{}).Resolve(context.Background(), in)
	if !errors.Is(err, ErrUnavailable) || len(out.Files) != 0 {
		t.Errorf("a failing request: %v, %d answers", err, len(out.Files))
	}
	if out.Usage.InputTokens == 0 || out.Usage.InputTokens > 2000 {
		t.Errorf("usage of the requests that answered = %+v", out.Usage)
	}
}

func TestResolveCache(t *testing.T) {
	cache := &memCache{}
	client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) {
		return answer(t, identify.AIFile{Path: "Frieren S2 - 04.mkv", Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{32}, Confidence: identify.ConfidenceMedium, Reason: "x"}), nil
	}}
	r := newTest(t, client, Config{Cache: cache})
	in := testInput()
	in.InfoHash, in.ShowID, in.MetadataUpdatedAt = "ABCDEF", 7, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	first, err := r.Resolve(ctx, in)
	if err != nil || client.count() != 1 || len(cache.data) != 1 {
		t.Fatalf("miss: %v, %d calls, %d entries", err, client.count(), len(cache.data))
	}
	second, err := r.Resolve(ctx, in)
	if err != nil || client.count() != 1 {
		t.Fatalf("hit: %v, %d calls; want no new call", err, client.count())
	}
	if len(second.Files) != 1 || second.Files[0].Path != first.Files[0].Path || second.Usage.CostUSD != 0 {
		t.Errorf("cached answer = %+v", second)
	}

	tests := []struct {
		name   string
		change func(*identify.ResolveInput)
	}{
		{"another hint", func(in *identify.ResolveInput) { in.Hint = "absolute numbering" }},
		{"another scope", func(in *identify.ResolveInput) { in.Scope = Videos(in.Rules) }},
		{"newer metadata", func(in *identify.ResolveInput) { in.MetadataUpdatedAt = in.MetadataUpdatedAt.Add(time.Hour) }},
		{"another torrent", func(in *identify.ResolveInput) { in.InfoHash = "123456" }},
		{"another show", func(in *identify.ResolveInput) { in.ShowID = 8 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := client.count()
			changed := in
			tt.change(&changed)
			if _, err := r.Resolve(ctx, changed); err != nil || client.count() != before+1 {
				t.Errorf("%v, %d new calls; want a miss", err, client.count()-before)
			}
		})
	}

	// The info hash's case doesn't matter.
	lower := in
	lower.InfoHash = "abcdef"
	before := client.count()
	if _, err := r.Resolve(ctx, lower); err != nil || client.count() != before {
		t.Errorf("a lower-case info hash missed the cache")
	}

	// Without an info hash nothing is cached.
	gets := cache.gets
	none := in
	none.InfoHash = ""
	if _, err := r.Resolve(ctx, none); err != nil || cache.gets != gets {
		t.Errorf("an input without an info hash used the cache")
	}
}

func TestResolveBudgetExceeded(t *testing.T) {
	client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) {
		return answer(t), nil
	}}
	r := newTest(t, client, Config{Budget: budgetFunc(func(context.Context) error {
		return fmt.Errorf("%w: $5.02 of $5.00", ErrBudgetExceeded)
	})})
	_, err := r.Resolve(context.Background(), testInput())
	if !errors.Is(err, ErrBudgetExceeded) || client.count() != 0 {
		t.Errorf("Resolve = %v after %d calls; want ErrBudgetExceeded and no call", err, client.count())
	}
}

func TestResolveStopReasons(t *testing.T) {
	tests := []struct {
		name string
		resp func(t *testing.T) *anthropic.Message
		want error
	}{
		{"refusal", func(t *testing.T) *anthropic.Message { return message(t, "refusal", "", "general_harms") }, ErrRefused},
		{"max_tokens", func(t *testing.T) *anthropic.Message { return message(t, "max_tokens", `{"files":[{"path":`, "") }, ErrTruncated},
		{"context window", func(t *testing.T) *anthropic.Message {
			return message(t, "model_context_window_exceeded", "", "")
		}, ErrTruncated},
		{"not JSON", func(t *testing.T) *anthropic.Message { return message(t, "end_turn", "Sure! Here you go.", "") }, ErrBadAnswer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}
			cache := &memCache{}
			client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) { return tt.resp(t), nil }}
			r := newTest(t, client, Config{Usage: rec, Cache: cache})
			in := testInput()
			in.InfoHash, in.ShowID = "abc", 1
			out, err := r.Resolve(context.Background(), in)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(out.Files) != 0 {
				t.Errorf("files = %+v, want none", out.Files)
			}
			// The tokens were spent: the usage is still recorded and
			// returned, and nothing is cached.
			if len(rec.usages) != 1 || out.Usage.InputTokens != 1000 || len(cache.data) != 0 {
				t.Errorf("usages %+v, output usage %+v, %d cached", rec.usages, out.Usage, len(cache.data))
			}
		})
	}
	if err := fmt.Errorf("%w (%s)", ErrRefused, "cyber"); !strings.Contains(err.Error(), "cyber") {
		t.Error("the refusal category is lost")
	}
}

func TestResolveAPIErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		kind      error
		retryable bool
		status    int
	}{
		{"rate limited", sdkError(429, "rate_limit_error"), ErrRateLimited, true, 429},
		{"overloaded", sdkError(529, "overloaded_error"), ErrUnavailable, true, 529},
		{"server error", sdkError(500, "api_error"), ErrUnavailable, true, 500},
		{"bad key", sdkError(401, "authentication_error"), ErrRejected, false, 401},
		{"bad request", sdkError(400, "invalid_request_error"), ErrRejected, false, 400},
		{"timeout", fmt.Errorf("Post: %w", context.DeadlineExceeded), ErrUnavailable, true, 0},
		{"network", errors.New("dial tcp: connection refused"), ErrUnavailable, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) { return nil, tt.err }}
			_, err := newTest(t, client, Config{}).Resolve(context.Background(), testInput())
			var apiErr *APIError
			if !errors.As(err, &apiErr) || !errors.Is(err, tt.kind) {
				t.Fatalf("err = %v, want an APIError matching %v", err, tt.kind)
			}
			if apiErr.Retryable() != tt.retryable || apiErr.StatusCode != tt.status {
				t.Errorf("retryable %v, status %d; want %v, %d", apiErr.Retryable(), apiErr.StatusCode, tt.retryable, tt.status)
			}
			// The message is shown to users: no URL, no response body.
			if msg := err.Error(); strings.Contains(msg, "anthropic.com") || strings.Contains(msg, "boom") {
				t.Errorf("message leaks request details: %q", msg)
			}
			if tt.status != 0 && apiErr.RequestID != "req_test" {
				t.Errorf("request id = %q", apiErr.RequestID)
			}
		})
	}

	// A cancelled caller isn't the API's fault.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &fakeClient{respond: func(int, anthropic.MessageNewParams) (*anthropic.Message, error) { return nil, context.Canceled }}
	_, err := newTest(t, client, Config{}).Resolve(ctx, testInput())
	var apiErr *APIError
	if !errors.Is(err, context.Canceled) || errors.As(err, &apiErr) {
		t.Errorf("cancelled: err = %v", err)
	}
}

func TestHintText(t *testing.T) {
	tests := []struct {
		hint *identify.Hint
		want string
	}{
		{nil, ""},
		{&identify.Hint{}, ""},
		{&identify.Hint{Season: intp(2)}, "The files are from season 2."},
		{&identify.Hint{Episode: intp(5), Text: "  the OVA  "}, "The file is episode 5. the OVA"},
	}
	for _, tt := range tests {
		if got := HintText(tt.hint); got != tt.want {
			t.Errorf("HintText(%+v) = %q, want %q", tt.hint, got, tt.want)
		}
	}
}

func TestDescribeRequests(t *testing.T) {
	in := testInput()
	reqs := DescribeRequests(in)
	if len(reqs) != 1 || reqs[0].Files != 2 || reqs[0].Episodes != 34 || reqs[0].SystemChars != len(systemPrompt) ||
		reqs[0].MaxTokens != 4300 || !strings.Contains(reqs[0].User, "<resolve>") {
		t.Errorf("DescribeRequests = %+v", reqs)
	}
}
