package catalog

import (
	"context"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
)

type fakeSessions struct {
	sessions []sessions.Session
	calls    int
}

func (f *fakeSessions) Sessions(context.Context) []sessions.Session {
	f.calls++
	return f.sessions
}

// LibraryItem.streaming is an overlay of the sessions on the one query's
// rows: a show streams while any of its episodes does, and the media type
// tells a movie from a show with the same library id.
func TestLibrary_Streaming(t *testing.T) {
	f := newFixture()
	f.store.library = []library.LibraryItem{
		{MediaType: "tv", ID: 3, TMDBID: frierenTMDB, Title: "Frieren", Stage: "ready"},
		{MediaType: "movie", ID: 3, TMDBID: duneTMDB, Title: "Dune", Stage: "ready"},
		{MediaType: "movie", ID: 9, TMDBID: 1054867, Title: "One Battle After Another", Stage: "ready"},
	}
	season, episode := 1, 28
	live := &fakeSessions{sessions: []sessions.Session{
		{ID: "1", Item: &sessions.Item{MediaType: "tv", LibraryID: 3, TMDBID: frierenTMDB, SeasonNumber: &season, EpisodeNumber: &episode}},
		{ID: "2", Item: &sessions.Item{MediaType: "movie", LibraryID: 9}},
		{ID: "3"}, // a file the library doesn't know
	}}
	f.svc.sessions = live
	before := f.store.total()

	page, err := f.svc.Library(context.Background(), LibraryParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, true}
	for i, it := range page.Items {
		if it.Streaming != want[i] {
			t.Errorf("%s %s: streaming = %v, want %v", it.MediaType, it.Title, it.Streaming, want[i])
		}
	}
	if calls := f.store.total() - before; calls != 1 || live.calls != 1 {
		t.Errorf("store calls %d, session reads %d; want one of each", calls, live.calls)
	}

	// No rows: the sessions aren't read.
	f.store.library = nil
	if _, err := f.svc.Library(context.Background(), LibraryParams{}); err != nil || live.calls != 1 {
		t.Errorf("an empty page read the sessions (%d), %v", live.calls, err)
	}
}
