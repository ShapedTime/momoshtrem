// Package ai resolves, with Claude, the torrent files the episode
// classifier's rules couldn't place. It implements identify.Resolver on
// claude-haiku-5-5 by default.
//
// # When it runs
//
// The caller decides, and passes the files to look at as
// identify.ResolveInput.Scope:
//   - classifier_mode auto: Scope(rules, false), the main videos the rules
//     left without a target, below high confidence or in a conflict.
//     Nothing unsure, no call.
//   - classifier_mode ai_always: Scope(rules, true), every main video.
//   - a user's re-check: the episode files the user hasn't decided (all of
//     them, or the pending and conflicting ones), with the user's hint.
//   - rules_only, classifier_enabled off, or no ANTHROPIC_API_KEY: never.
//     New returns nil without a key; the caller keeps
//     identify.NoopResolver and reports ai_available false.
//
// An empty Scope means Scope(rules, false). Non-video files are never
// asked about.
//
// # What the caller does with the answer
//
// Resolve returns the model's answers for the files in scope, with their
// paths mapped back to the torrent's, but not validated. The caller merges
// them with identify.MergeAI, which rejects targets that aren't episodes of
// the show and never overrides a high-confidence rules answer, then runs
// identify.FlagDisagreements: a high rules answer the AI disagrees with
// keeps its target but goes to review, its reason naming the AI's pick. An
// answer only the AI gives keeps the AI's confidence: the job service sends
// it to needs_review unless it's high and auto_apply_high_confidence allows
// applying it.
//
// # Cost and time
//
// Answers are cached by torrent, show, metadata version, hint and scope (a
// cached answer is free, budget or not). Before calling, Resolve checks the
// monthly budget and returns ErrBudgetExceeded when it's spent. Every API
// call's usage is recorded with its cost from the price table (cost.go),
// whether or not its answer could be used.
//
// More than 60 files are split into requests that run side by side, four
// at a time: Waves says how many rounds a scope takes, so the caller can
// scale its time budget. A request that fails doesn't stop the others:
// Resolve returns the answers it got along with the error.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

// DefaultModel is the classifier's model (PLAN D6).
const DefaultModel = "claude-haiku-5-5"

// DefaultTimeout bounds one API request, within the time a caller should
// allow each wave (the job service gives each 90 seconds). The SDK retries
// 429s, 5xx and timeouts twice; the caller's context bounds the whole run.
const DefaultTimeout = 75 * time.Second

// maxConcurrentCalls bounds the requests one Resolve runs at once.
const maxConcurrentCalls = 4

// Waves is how many rounds of requests Resolve makes for n files in
// scope: requests of up to 60 files, four at a time. A caller scales its
// time budget by it.
func Waves(n int) int {
	chunks := (n + filesPerCall - 1) / filesPerCall
	return max(1, (chunks+maxConcurrentCalls-1)/maxConcurrentCalls)
}

// messagesAPI is the one SDK method the resolver uses; tests use a fake.
type messagesAPI interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

// Config configures a Resolver.
type Config struct {
	// APIKey is ANTHROPIC_API_KEY. Empty disables the resolver.
	APIKey string
	// Model is the classifier_model setting; empty means DefaultModel. It
	// must be in the price table.
	Model string
	// Timeout bounds one API request; zero means DefaultTimeout.
	Timeout time.Duration
	// Cache, Usage and Budget default to no cache, no records and no limit.
	Cache  Cache
	Usage  UsageRecorder
	Budget Budget
}

// Resolver asks Claude about the files the rules are unsure of.
type Resolver struct {
	client  messagesAPI
	model   string
	timeout time.Duration
	cache   Cache
	usage   UsageRecorder
	budget  Budget
	log     *slog.Logger
}

var _ identify.Resolver = (*Resolver)(nil)

// New returns a resolver, or nil (and no error) when there's no API key:
// the classifier then runs on its rules alone.
func New(cfg Config) (*Resolver, error) {
	if cfg.APIKey == "" {
		slog.Info("ANTHROPIC_API_KEY not set: episode mapping runs on rules only", "component", "ai-resolver")
		return nil, nil
	}
	client := anthropic.NewClient(option.WithAPIKey(cfg.APIKey))
	return newResolver(&client.Messages, cfg)
}

func newResolver(client messagesAPI, cfg Config) (*Resolver, error) {
	r := &Resolver{
		client: client, model: cfg.Model, timeout: cfg.Timeout,
		cache: cfg.Cache, usage: cfg.Usage, budget: cfg.Budget,
		log: slog.With("component", "ai-resolver"),
	}
	if r.model == "" {
		r.model = DefaultModel
	}
	if _, ok := prices[r.model]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownModel, r.model)
	}
	if r.timeout <= 0 {
		r.timeout = DefaultTimeout
	}
	if r.cache == nil {
		r.cache = noCache{}
	}
	if r.usage == nil {
		r.usage = noUsage{}
	}
	if r.budget == nil {
		r.budget = noBudget{}
	}
	return r, nil
}

// Model is the model the resolver calls when the input names none.
func (r *Resolver) Model() string { return r.model }

// Resolve asks the model about the files in scope: in.Model, else the
// resolver's. On an error the output still carries the usage of the calls
// that were made and, when the scope was split, the answers of the
// requests that succeeded: the caller may keep those.
func (r *Resolver) Resolve(ctx context.Context, in identify.ResolveInput) (identify.ResolveOutput, error) {
	scope := effectiveScope(in)
	if len(scope) == 0 {
		return identify.ResolveOutput{}, nil
	}
	model := r.model
	if in.Model != "" {
		model = in.Model
	}
	if _, ok := prices[model]; !ok {
		return identify.ResolveOutput{}, fmt.Errorf("%w: %q", ErrUnknownModel, model)
	}

	// A cached answer costs nothing: it's served even with the budget spent.
	key, cacheable := cacheKey(model, in, scope)
	if cacheable {
		if files, ok := r.cached(ctx, key); ok {
			r.log.Debug("Answer from cache", "files", len(files), "job_id", in.JobID)
			return identify.ResolveOutput{Files: files, Usage: identify.Usage{Model: model}}, nil
		}
	}
	if err := r.budget.Check(ctx); err != nil {
		return identify.ResolveOutput{}, err
	}

	// A big scope is split into requests that run side by side, four at a
	// time. One that fails doesn't cancel the others: their answers are
	// kept.
	reqs := buildRequests(in, scope)
	type result struct {
		files []identify.AIFile
		usage identify.Usage
		err   error
	}
	results := make([]result, len(reqs))
	sem := make(chan struct{}, maxConcurrentCalls)
	var wg sync.WaitGroup
	for i, q := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i].err = apiError(ctx, ctx.Err())
				return
			}
			defer func() { <-sem }()
			files, usage, err := r.call(ctx, model, q, in.JobID)
			results[i] = result{files, usage, err}
		}()
	}
	wg.Wait()

	out := identify.ResolveOutput{Usage: identify.Usage{Model: model}}
	var firstErr error
	failed := 0
	for _, res := range results {
		out.Usage = out.Usage.Add(res.usage)
		out.Files = append(out.Files, res.files...)
		if res.err != nil {
			failed++
			if firstErr == nil {
				firstErr = res.err
			}
		}
	}
	switch {
	case failed == 0:
		if cacheable {
			r.store(ctx, key, out.Files) // only a complete answer is cached
		}
		return out, nil
	case len(reqs) == 1:
		return out, firstErr
	default:
		return out, fmt.Errorf("%d of %d requests failed: %w", failed, len(reqs), firstErr)
	}
}

// call makes one API request and reads its answer. Its usage is recorded
// even when the answer can't be used: the tokens were spent.
func (r *Resolver) call(ctx context.Context, model string, q request, ref int64) ([]identify.AIFile, identify.Usage, error) {
	start := time.Now()
	resp, err := r.client.New(ctx, q.params(model), option.WithRequestTimeout(r.timeout))
	if err != nil {
		err = apiError(ctx, err)
		r.log.Warn("Classifier request failed", "error", err, "files", len(q.paths), "job_id", ref)
		return nil, identify.Usage{Model: model}, err
	}
	usage := usageOf(model, resp.Usage)
	// Recorded even when the run was cancelled meanwhile (another request
	// failed, or shutdown): the tokens were spent.
	if err := r.usage.Record(context.WithoutCancel(ctx), usage, ref); err != nil {
		r.log.Warn("Failed to record AI usage", "error", err, "job_id", ref)
	}
	files, err := q.parse(resp)
	r.log.Info("Classifier request", "files", len(q.paths), "answers", len(files),
		"stop_reason", resp.StopReason, "input_tokens", usage.InputTokens, "cache_read_tokens", usage.CacheReadTokens,
		"output_tokens", usage.OutputTokens, "cost_usd", usage.CostUSD, "duration", time.Since(start), "job_id", ref)
	if err != nil {
		return nil, usage, err
	}
	return files, usage, nil
}

func (r *Resolver) cached(ctx context.Context, key string) ([]identify.AIFile, bool) {
	payload, ok, err := r.cache.Get(ctx, key)
	if err != nil {
		r.log.Warn("AI cache read failed", "error", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	var files []identify.AIFile
	if err := json.Unmarshal(payload, &files); err != nil {
		r.log.Warn("Unreadable AI cache entry", "error", err)
		return nil, false
	}
	return files, true
}

func (r *Resolver) store(ctx context.Context, key string, files []identify.AIFile) {
	if files == nil {
		files = []identify.AIFile{}
	}
	payload, err := json.Marshal(files)
	if err == nil {
		err = r.cache.Put(ctx, key, payload, cacheTTL)
	}
	if err != nil {
		r.log.Warn("AI cache write failed", "error", err)
	}
}

// RequestInfo describes one request Resolve would make, for
// classifier-eval's dry run: it prints them without calling the API.
type RequestInfo struct {
	Files       int
	SystemChars int
	MaxTokens   int64
	// Episodes is how many episodes the request lists.
	Episodes int
	// User is the user message: the torrent's context, then the files to
	// answer for.
	User string
}

// DescribeRequests returns the requests Resolve would make for in, without
// making them.
func DescribeRequests(in identify.ResolveInput) []RequestInfo {
	var out []RequestInfo
	for _, q := range buildRequests(in, effectiveScope(in)) {
		out = append(out, RequestInfo{
			Files: len(q.paths), SystemChars: len(systemPrompt), MaxTokens: q.maxTokens(),
			Episodes: q.episodes, User: q.context + q.ask,
		})
	}
	return out
}

// HintText writes an assign hint for the prompt: "Season 2. Episode 5.
// <the user's text>". Empty without a hint.
func HintText(h *identify.Hint) string {
	if h == nil {
		return ""
	}
	var parts []string
	if h.Season != nil {
		parts = append(parts, fmt.Sprintf("The files are from season %d.", *h.Season))
	}
	if h.Episode != nil {
		parts = append(parts, fmt.Sprintf("The file is episode %d.", *h.Episode))
	}
	if t := strings.TrimSpace(h.Text); t != "" {
		parts = append(parts, t)
	}
	return strings.Join(parts, " ")
}
