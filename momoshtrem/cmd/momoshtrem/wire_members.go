package main

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// newMembers reads the household's members (#241) and answers the lowest
// id: until #245, the member the picks are for.
func newMembers(db *library.DB) (*member.Service, int64, error) {
	members := member.NewService(library.NewMemberRepository(db))
	lowest, err := members.Default(context.Background()) // reads them
	return members, lowest, err
}

// wireMembers serves the household's members: /members, each with their
// own Trakt connection's summary (#244), and the member each API request
// is for.
func wireMembers(members *member.Service, traktRegistry *trakt.Registry, opts *api.Options) {
	members.SetTraktSummary(traktRegistry.Summary)
	opts.Members = members
}
