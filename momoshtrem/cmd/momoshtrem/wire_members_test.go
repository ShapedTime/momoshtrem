package main

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/trakt"
)

type fixedSnapshot struct {
	state    trakt.State
	username *string
}

func (f fixedSnapshot) Snapshot() (trakt.State, *string) { return f.state, f.username }

// Until #244, the one Trakt connection is the member it's bound to (the
// lowest at startup), even once that's no longer the lowest; everyone else
// is off.
func TestBoundMemberTrakt(t *testing.T) {
	username := "teymur"
	summary := boundMemberTrakt(2, fixedSnapshot{state: trakt.StateConnected, username: &username})
	if got := summary(2); got.State != "connected" || got.Username == nil || *got.Username != "teymur" {
		t.Errorf("the bound member's = %+v", got)
	}
	for _, id := range []int64{1, 5} {
		if got := summary(id); got.State != "off" || got.Username != nil {
			t.Errorf("member %d's = %+v", id, got)
		}
	}
}
