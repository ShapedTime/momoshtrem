package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/identify"
)

// Cache keeps answers between runs (ai_cache). A failing cache is logged
// and treated as a miss: it never fails a run.
type Cache interface {
	Get(ctx context.Context, key string) (payload []byte, ok bool, err error)
	Put(ctx context.Context, key string, payload []byte, ttl time.Duration) error
}

// UsageRecorder stores what each API call cost (ai_usage). ref is the
// assignment job the call was for (ai_usage.ref_id), 0 for none. A failing
// recorder is logged; it never fails a run.
type UsageRecorder interface {
	Record(ctx context.Context, u identify.Usage, ref int64) error
}

// Budget guards the monthly AI budget every AI feature shares
// (monthly_budget_usd): Check returns an error wrapping ErrBudgetExceeded
// once this month's spend has reached it (aicore.Ledger).
type Budget = aicore.Budget

// The defaults: no cache, nothing recorded, no limit.
type (
	noCache  struct{}
	noUsage  struct{}
	noBudget struct{}
)

func (noCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noCache) Put(context.Context, string, []byte, time.Duration) error { return nil }
func (noUsage) Record(context.Context, identify.Usage, int64) error      { return nil }
func (noBudget) Check(context.Context) error                             { return nil }

// cacheTTL bounds how long an answer is kept. The key changes with the
// show's metadata, so this only keeps the table small.
const cacheTTL = 30 * 24 * time.Hour

// cacheKey keys an answer by torrent, show, metadata version, hint and
// scope, plus the model and the prompt version. It reports false when the
// input can't be keyed (no info hash or show).
func cacheKey(model string, in identify.ResolveInput, scope []string) (string, bool) {
	if in.InfoHash == "" || in.ShowID == 0 {
		return "", false
	}
	sorted := slices.Clone(scope)
	slices.Sort(sorted)
	fields := struct {
		Version           string   `json:"v"`
		Model             string   `json:"model"`
		InfoHash          string   `json:"info_hash"`
		ShowID            int64    `json:"show_id"`
		MetadataUpdatedAt string   `json:"metadata_updated_at"`
		Hint              string   `json:"hint"`
		Scope             []string `json:"scope"`
	}{promptVersion, model, strings.ToLower(in.InfoHash), in.ShowID,
		in.MetadataUpdatedAt.UTC().Format(time.RFC3339Nano), strings.TrimSpace(in.Hint), sorted}
	data, err := json.Marshal(fields)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	return "classifier:" + hex.EncodeToString(sum[:]), true
}
