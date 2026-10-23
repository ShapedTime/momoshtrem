package recommend

import (
	"context"
	"encoding/json"

	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
)

// DebugReport is the dev-only GET /recommendations/debug/profile: R1's
// inputs as a run would see them now.
type DebugReport struct {
	TraktConnected bool             `json:"trakt_connected"`
	EnoughHistory  bool             `json:"enough_history"` // a run would go ahead (no no_history)
	ProfileBytes   int              `json:"profile_bytes"`  // as JSON; the budget is profile.MaxBytes
	Profile        *profile.Profile `json:"profile"`
	Pool           *candidates.Pool `json:"pool"`       // null when it couldn't be built
	PoolError      *string          `json:"pool_error"` // why
}

// Debug builds the debug report for the context's member
// (member.Require), storing nothing.
type Debug struct {
	// Builders are a member's profile and pool builders, bound to them.
	Builders func(memberID int64) (*profile.Builder, *candidates.Builder)
}

// DebugProfile builds the member's profile and pool.
func (d *Debug) DebugProfile(ctx context.Context) (*DebugReport, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	profiles, pools := d.Builders(memberID)
	ready, err := profiles.CheckHistory(ctx)
	if err != nil {
		return nil, err
	}
	p, err := profiles.Build(ctx)
	if err != nil {
		return nil, err
	}
	doc, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	report := &DebugReport{TraktConnected: ready.Connected, EnoughHistory: ready.Enough(), ProfileBytes: len(doc), Profile: p}
	pool, err := pools.Build(ctx, p)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := err.Error()
		report.PoolError = &msg
		return report, nil
	}
	report.Pool = pool
	return report, nil
}
