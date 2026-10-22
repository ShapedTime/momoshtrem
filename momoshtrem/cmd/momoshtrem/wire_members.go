package main

import (
	"context"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// newMembers reads the household's members (#241).
func newMembers(db *library.DB) *member.Service {
	members := member.NewService(library.NewMemberRepository(db))
	if err := members.Load(context.Background()); err != nil {
		slog.Error("Failed to read the household's members; trying again on first use", "error", err)
	}
	return members
}

// wireMembers serves the household's members: /members, and the member
// each API request is for. Until each member has their own Trakt
// connection (#244), the one there is shows on the lowest member, and the
// others' are off.
func wireMembers(members *member.Service, traktAuth *trakt.Auth, opts *api.Options) {
	members.SetTraktSummary(lowestMemberTrakt(members, traktAuth))
	opts.Members = members
}

// traktSnapshot is the one Trakt connection's state. *trakt.Auth
// implements it.
type traktSnapshot interface {
	Snapshot() (trakt.State, *string)
}

// lowestMemberTrakt is the members' Trakt summary while there's one
// connection: the lowest member's is the connection's, everyone else's
// off.
func lowestMemberTrakt(members *member.Service, auth traktSnapshot) func(id int64) member.TraktSummary {
	return func(id int64) member.TraktSummary {
		lowest, err := members.Default(context.Background())
		if err != nil || id != lowest {
			return member.TraktSummary{State: string(trakt.StateOff)}
		}
		state, username := auth.Snapshot()
		return member.TraktSummary{State: string(state), Username: username}
	}
}
