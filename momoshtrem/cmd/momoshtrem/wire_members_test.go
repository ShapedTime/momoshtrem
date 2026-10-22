package main

import (
	"context"
	"errors"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// listStore lists fixed members; nothing else is called.
type listStore struct {
	ids []int64
	err error
}

func (s listStore) List(context.Context) ([]member.Member, error) {
	out := make([]member.Member, len(s.ids))
	for i, id := range s.ids {
		out[i] = member.Member{ID: id, Name: "M", Color: "teal"}
	}
	return out, s.err
}

func (listStore) Create(context.Context, string, string) (member.Member, error) {
	return member.Member{}, errors.New("unused")
}

func (listStore) Update(context.Context, int64, member.Patch) (member.Member, error) {
	return member.Member{}, errors.New("unused")
}

func (listStore) Delete(context.Context, int64) error { return errors.New("unused") }

type fixedSnapshot struct {
	state    trakt.State
	username *string
}

func (f fixedSnapshot) Snapshot() (trakt.State, *string) { return f.state, f.username }

// Until #244, the one Trakt connection is the lowest member's; everyone
// else is off.
func TestLowestMemberTrakt(t *testing.T) {
	username := "teymur"
	auth := fixedSnapshot{state: trakt.StateConnected, username: &username}

	summary := lowestMemberTrakt(member.NewService(listStore{ids: []int64{2, 5}}), auth)
	if got := summary(2); got.State != "connected" || got.Username == nil || *got.Username != "teymur" {
		t.Errorf("the lowest member's = %+v", got)
	}
	if got := summary(5); got.State != "off" || got.Username != nil {
		t.Errorf("another member's = %+v", got)
	}

	failing := lowestMemberTrakt(member.NewService(listStore{err: errors.New("db down")}), auth)
	if got := failing(2); got.State != "off" || got.Username != nil {
		t.Errorf("without the members = %+v", got)
	}
}
