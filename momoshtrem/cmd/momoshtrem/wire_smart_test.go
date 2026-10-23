package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/me"
	"github.com/shapedtime/momoshtrem/internal/member"
)

// memberUpNext answers each member's Up next, recording whom it was asked
// for.
type memberUpNext struct {
	lists map[int64][]me.UpNextEpisode
	err   error
	asked []int64
}

func (f *memberUpNext) UpNextEpisodes(ctx context.Context) ([]me.UpNextEpisode, error) {
	id, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	f.asked = append(f.asked, id)
	return f.lists[id], f.err
}

type fixedIDs []int64

func (f fixedIDs) IDs(context.Context) ([]int64, error) { return f, nil }

// The smart picks read every member's Up next, by member id; a show two
// members watch is there twice (the picks keep the one watched last).
func TestUpNextShows_EveryMember(t *testing.T) {
	at := time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)
	later := at.Add(time.Hour)
	f := &memberUpNext{lists: map[int64][]me.UpNextEpisode{
		1: {{ShowID: 10, EpisodeID: 103, SeasonNumber: 1, EpisodeNumber: 3, LastWatchedAt: &at}},
		4: {{ShowID: 10, EpisodeID: 106, SeasonNumber: 1, EpisodeNumber: 6, LastWatchedAt: &later},
			{ShowID: 20, EpisodeID: 201, SeasonNumber: 1, EpisodeNumber: 1}},
	}}
	got, err := upNextShows{me: f, members: fixedIDs{1, 2, 4}}.UpNextShows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []downloads.UpNextShow{
		{ShowID: 10, SeasonNumber: 1, EpisodeNumber: 3, LastWatchedAt: &at},
		{ShowID: 10, SeasonNumber: 1, EpisodeNumber: 6, LastWatchedAt: &later},
		{ShowID: 20, SeasonNumber: 1, EpisodeNumber: 1},
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(f.asked, []int64{1, 2, 4}) {
		t.Errorf("Up next = %+v for members %v\nwant %+v for [1 2 4]", got, f.asked, want)
	}

	f.err = errors.New("db down")
	if _, err := (upNextShows{me: f, members: fixedIDs{1}}).UpNextShows(context.Background()); !errors.Is(err, f.err) {
		t.Errorf("a failed read = %v", err)
	}
}
