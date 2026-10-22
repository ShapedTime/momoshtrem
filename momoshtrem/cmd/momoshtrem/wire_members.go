package main

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// newMembers reads the household's members (#241) and answers the lowest
// id: until #244, the member the one Trakt connection is bound to.
func newMembers(db *library.DB) (*member.Service, int64, error) {
	members := member.NewService(library.NewMemberRepository(db))
	lowest, err := members.Default(context.Background()) // reads them
	return members, lowest, err
}

// wireMembers serves the household's members: /members, and the member
// each API request is for. Until each member has their own Trakt
// connection (#244), the one there is shows on traktMember, the member
// it's bound to, and the others' are off.
func wireMembers(members *member.Service, traktMember int64, traktAuth *trakt.Auth, opts *api.Options) {
	members.SetTraktSummary(boundMemberTrakt(traktMember, traktAuth))
	opts.Members = members
}

// traktSnapshot is the one Trakt connection's state. *trakt.Auth
// implements it.
type traktSnapshot interface {
	Snapshot() (trakt.State, *string)
}

// boundMemberTrakt is the members' Trakt summary while there's one
// connection: bound's is the connection's, everyone else's off.
func boundMemberTrakt(bound int64, auth traktSnapshot) func(id int64) member.TraktSummary {
	return func(id int64) member.TraktSummary {
		if id != bound {
			return member.TraktSummary{State: string(trakt.StateOff)}
		}
		state, username := auth.Snapshot()
		return member.TraktSummary{State: string(state), Username: username}
	}
}
