package status

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// JackettSource is the part of the Torznab client the Jackett probe uses.
type JackettSource interface {
	FetchIndexers(ctx context.Context) ([]torznab.Indexer, error)
	Snapshot() []torznab.IndexerHealth
}

// JackettProber asks Jackett for its configured indexers
// (t=indexers&configured=true), which checks the API key and lists them in
// one request, then reports each with its latest search outcome.
func JackettProber(src JackettSource) JackettProbe {
	return func(ctx context.Context) ([]IndexerStatus, error) {
		if _, err := src.FetchIndexers(ctx); err != nil {
			return nil, err
		}
		health := src.Snapshot()
		out := make([]IndexerStatus, 0, len(health))
		for _, h := range health {
			st := IndexerStatus{Name: h.Name, Status: string(h.Status), MS: h.MS, Error: h.Error}
			if h.Since != nil {
				since := h.Since.UTC().Format(time.RFC3339)
				st.Since = &since
			}
			out = append(out, st)
		}
		return out, nil
	}
}
