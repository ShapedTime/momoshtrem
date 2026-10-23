package library

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// seedTaste fills a private schema with a small history:
//   - Dune: Part Two (693134), watched twice, rated 9; Past Lives (666277)
//     on the watchlist, rated 3, never watched; The Room (17473) rated 2
//     with nothing else naming it;
//   - Frieren (209867): progress 26 of 38, in the library (anime, two
//     episodes with runtimes), rated 10;
//   - Severance (95396): plays but no progress row, its episodes rated 8
//     and 9 (no show rating);
//   - Game of Thrones (1399): progress, dropped and hidden on Trakt;
//   - The Bear (136315): in the library only.
func seedTaste(t *testing.T, db *DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, year, season, episode, watched_at) VALUES
			(1, 1, 'movie', 693134, 367444, 'Dune: Part Two', 2024, NULL, NULL, '2026-09-01T20:00:00Z'),
			(1, 2, 'movie', 693134, 367444, 'Dune: Part Two', 2024, NULL, NULL, '2026-10-01T20:00:00Z'),
			(1, 3, 'episode', 209867, 184317, 'Frieren', 2023, 1, 26, '2026-10-10T20:00:00Z'),
			(1, 4, 'episode', 95396, 154997, 'Severance', 2022, 1, 1, '2026-08-01T20:00:00Z'),
			(1, 5, 'episode', 95396, 154997, 'Severance', 2022, 1, 2, '2026-08-02T20:00:00Z'),
			(1, 6, 'episode', 95396, 154997, 'Severance', 2022, 0, 1, '2026-08-03T20:00:00Z')`,
		`INSERT INTO trakt_watched_movies (member_id, tmdb_id, plays, last_watched_at) VALUES (1, 693134, 2, '2026-10-01T20:00:00Z')`,
		`INSERT INTO trakt_watched_episodes (member_id, show_tmdb_id, season, episode, plays, last_watched_at) VALUES
			(1, 209867, 1, 26, 1, '2026-10-10T20:00:00Z'),
			(1, 95396, 1, 1, 2, '2026-08-01T20:00:00Z'),
			(1, 95396, 1, 2, 1, '2026-08-02T20:00:00Z'),
			(1, 95396, 0, 1, 1, '2026-08-03T20:00:00Z')`,
		`INSERT INTO trakt_show_progress (member_id, show_trakt_id, show_tmdb_id, title, year, aired, completed, last_watched_at) VALUES
			(1, 184317, 209867, 'Frieren: Beyond Journey''s End', 2023, 38, 26, '2026-10-10T20:00:00Z'),
			(1, 1390, 1399, 'Game of Thrones', 2011, 73, 5, '2025-01-05T20:00:00Z')`,
		`INSERT INTO trakt_hidden (member_id, section, trakt_id, tmdb_id) VALUES (1, 'dropped', 1390, 1399), (1, 'progress_watched', 1390, 1399)`,
		`INSERT INTO trakt_ratings (member_id, media_type, trakt_id, tmdb_id, show_tmdb_id, season, episode, rating, rated_at) VALUES
			(1, 'movie', 367444, 693134, NULL, NULL, NULL, 9, '2026-10-01T22:00:00Z'),
			(1, 'movie', 1, 666277, NULL, NULL, NULL, 3, '2026-06-01T22:00:00Z'),
			(1, 'movie', 2, 17473, NULL, NULL, NULL, 2, '2026-05-01T22:00:00Z'),
			(1, 'show', 184317, 209867, NULL, NULL, NULL, 10, '2026-10-11T22:00:00Z'),
			(1, 'episode', 11, 4001, 95396, 1, 1, 8, '2026-08-01T22:00:00Z'),
			(1, 'episode', 12, 4002, 95396, 1, 2, 9, '2026-08-02T22:00:00Z'),
			(1, 'episode', 13, 4003, 209867, 1, 26, 2, '2026-10-10T22:00:00Z')`,
		`INSERT INTO trakt_watchlist (member_id, media_type, trakt_id, tmdb_id, title, year, listed_at) VALUES
			(1, 'movie', 761234, 666277, 'Past Lives', 2023, '2026-03-01T00:00:00Z'),
			(1, 'tv', 9, 95396, 'Severance', 2022, '2026-02-01T00:00:00Z')`,
		`INSERT INTO shows (id, tmdb_id, title, year, original_language, origin_country, is_anime, genres) VALUES
			(1, 209867, 'Frieren', 2023, 'ja', '{JP}', TRUE, '["Animation", "Fantasy"]'),
			(2, 136315, 'The Bear', 2022, 'en', '{US}', FALSE, '["Comedy", "Drama"]')`,
		`INSERT INTO seasons (id, show_id, season_number) VALUES (1, 1, 1), (2, 1, 0)`,
		`INSERT INTO episodes (season_id, episode_number, runtime) VALUES (1, 1, 24), (1, 2, 25), (2, 1, 90)`,
		`INSERT INTO movies (tmdb_id, title, year, runtime, genres) VALUES (693134, 'Dune Part Two', 2024, 166, '["Science Fiction"]')`,
		`INSERT INTO rec_feedback (member_id, media_type, tmdb_id, signal, source, updated_at) VALUES
			(1, 'tv', 136315, 'like', 'shelf', '2026-10-12T00:00:00Z'),
			(1, 'movie', 550, 'dislike', 'hero', '2026-10-13T00:00:00Z'),
			(1, 'tv', 1399, 'seen', 'title', '2026-10-11T00:00:00Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%.60s…: %v", q, err)
		}
	}
}

func atPtr(s string) *time.Time {
	t := at(s)
	return &t
}

func TestTasteRepository(t *testing.T) {
	db := openSchemaDB(t)
	seedTaste(t, db)
	repo := NewTasteRepository(db).ForMember(1)
	ctx := context.Background()

	t.Run("watched movies", func(t *testing.T) {
		got, err := repo.WatchedMovies(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []WatchedMovie{{TMDBID: 693134, Title: "Dune: Part Two", Year: 2024, Plays: 2, LastWatchedAt: at("2026-10-01T20:00:00Z")}}
		if !equalJSON(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("watched shows", func(t *testing.T) {
		got, err := repo.WatchedShows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []WatchedShow{
			{TMDBID: 209867, Title: "Frieren: Beyond Journey's End", Year: 2023, Plays: 1, EpisodesWatched: 1, Aired: 38, Completed: 26,
				HasProgress: true, LastWatchedAt: atPtr("2026-10-10T20:00:00Z")},
			// Plays only: named by its newest play; the special isn't a
			// regular episode watched
			{TMDBID: 95396, Title: "Severance", Year: 2022, Plays: 4, EpisodesWatched: 2, LastWatchedAt: atPtr("2026-08-03T20:00:00Z")},
			// Progress only
			{TMDBID: 1399, Title: "Game of Thrones", Year: 2011, Aired: 73, Completed: 5, HasProgress: true,
				LastWatchedAt: atPtr("2025-01-05T20:00:00Z")},
		}
		if !equalJSON(got, want) {
			t.Errorf("got %s\nwant %s", toJSON(got), toJSON(want))
		}
	})

	t.Run("ratings", func(t *testing.T) {
		got, err := repo.TasteRatings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []TasteRating{
			{TitleRef: TitleRef{TitleTV, 209867}, Title: "Frieren", Year: 2023, Rating: 10, RatedAt: at("2026-10-11T22:00:00Z")},
			{TitleRef: TitleRef{TitleMovie, 693134}, Title: "Dune Part Two", Year: 2024, Rating: 9, RatedAt: at("2026-10-01T22:00:00Z")},
			// 8 and 9 → 8.5 rounds to 9
			{TitleRef: TitleRef{TitleTV, 95396}, Title: "Severance", Year: 2022, Rating: 9, FromEpisodes: true, RatedAt: at("2026-08-02T22:00:00Z")},
			{TitleRef: TitleRef{TitleMovie, 666277}, Title: "Past Lives", Year: 2023, Rating: 3, RatedAt: at("2026-06-01T22:00:00Z")},
			{TitleRef: TitleRef{TitleMovie, 17473}, Rating: 2, RatedAt: at("2026-05-01T22:00:00Z")},
		}
		if !equalJSON(got, want) {
			t.Errorf("got %s\nwant %s", toJSON(got), toJSON(want))
		}
	})

	t.Run("hidden shows", func(t *testing.T) {
		got, err := repo.HiddenShows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []HiddenShow{{TMDBID: 1399, Dropped: true, FromProgress: true}}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("watchlist", func(t *testing.T) {
		got, err := repo.WatchlistRefs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []TitleRef{{TitleMovie, 666277}, {TitleTV, 95396}}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("library titles", func(t *testing.T) {
		got, err := repo.LibraryTitles(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []LibraryTitle{
			{TitleRef: TitleRef{TitleMovie, 693134}, Title: "Dune Part Two", Year: 2024, Genres: []string{"Science Fiction"}, Runtime: 166},
			{TitleRef: TitleRef{TitleTV, 136315}, Title: "The Bear", Year: 2022, Genres: []string{"Comedy", "Drama"},
				OriginalLanguage: "en", OriginCountry: []string{"US"}},
			// Specials don't count toward the episode runtime
			{TitleRef: TitleRef{TitleTV, 209867}, Title: "Frieren", Year: 2023, Genres: []string{"Animation", "Fantasy"},
				OriginalLanguage: "ja", OriginCountry: []string{"JP"}, IsAnime: true, Runtime: 25},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %s\nwant %s", toJSON(got), toJSON(want))
		}
	})

	t.Run("feedback with titles", func(t *testing.T) {
		got, err := repo.TasteFeedback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []TasteFeedback{
			{TitleRef: TitleRef{TitleMovie, 550}, Signal: "dislike", UpdatedAt: at("2026-10-13T00:00:00Z")},
			{TitleRef: TitleRef{TitleTV, 136315}, Signal: "like", Title: "The Bear", Year: 2022, UpdatedAt: at("2026-10-12T00:00:00Z")},
			{TitleRef: TitleRef{TitleTV, 1399}, Signal: "seen", Title: "Game of Thrones", Year: 2011, UpdatedAt: at("2026-10-11T00:00:00Z")},
		}
		if !equalJSON(got, want) {
			t.Errorf("got %s\nwant %s", toJSON(got), toJSON(want))
		}
	})

	t.Run("title states", func(t *testing.T) {
		refs := []TitleRef{{TitleTV, 1399}, {TitleMovie, 693134}, {TitleTV, 95396}, {TitleMovie, 550}, {TitleTV, 693134}, {TitleTV, 136315}}
		got, err := repo.TitleStates(ctx, refs)
		if err != nil {
			t.Fatal(err)
		}
		nine := 9
		want := []TitleState{
			{TitleRef: refs[0], Started: true, Aired: 73, Completed: 5, LastWatchedAt: atPtr("2025-01-05T20:00:00Z"),
				Dropped: true, Hidden: true, Feedback: "seen"},
			{TitleRef: refs[1], Plays: 2, Started: true, LastWatchedAt: atPtr("2026-10-01T20:00:00Z"), Rating: &nine, InLibrary: true},
			{TitleRef: refs[2], Plays: 4, Started: true, LastWatchedAt: atPtr("2026-08-03T20:00:00Z"), OnWatchlist: true},
			{TitleRef: refs[3], Feedback: "dislike"},
			// The same id as a show is another title
			{TitleRef: refs[4]},
			{TitleRef: refs[5], Feedback: "like", InLibrary: true},
		}
		if !equalJSON(got, want) {
			t.Errorf("got %s\nwant %s", toJSON(got), toJSON(want))
		}
		if none, err := repo.TitleStates(ctx, nil); err != nil || none != nil {
			t.Errorf("no refs = %v, %v", none, err)
		}
		// int4 holds TMDB ids: a larger one is refused, never wrapped
		if _, err := repo.TitleStates(ctx, []TitleRef{{TitleMovie, 1 << 31}}); err == nil {
			t.Error("an id past int4 was looked up")
		}
	})
}

// Without Trakt or a library every reader finds nothing.
func TestTasteRepository_Empty(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewTasteRepository(db).ForMember(1)
	ctx := context.Background()
	movies, err1 := repo.WatchedMovies(ctx)
	shows, err2 := repo.WatchedShows(ctx)
	ratings, err3 := repo.TasteRatings(ctx)
	hidden, err4 := repo.HiddenShows(ctx)
	watchlist, err5 := repo.WatchlistRefs(ctx)
	titles, err6 := repo.LibraryTitles(ctx)
	feedback, err7 := repo.TasteFeedback(ctx)
	for i, err := range []error{err1, err2, err3, err4, err5, err6, err7} {
		if err != nil {
			t.Errorf("reader %d: %v", i, err)
		}
	}
	if len(movies)+len(shows)+len(ratings)+len(hidden)+len(watchlist)+len(titles)+len(feedback) != 0 {
		t.Errorf("found rows in an empty database")
	}
}

func TestRecFeedbackRepository(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewRecFeedbackRepository(db)
	ctx := context.Background()
	severance := TitleRef{TitleTV, 95396}

	if err := repo.Set(ctx, 1, severance, "dislike", "shelf"); err != nil {
		t.Fatal(err)
	}
	first, err := repo.List(ctx, 1)
	if err != nil || len(first) != 1 || first[0].Signal != "dislike" || first[0].Source != "shelf" {
		t.Fatalf("after set: %+v, %v", first, err)
	}
	// The newest signal wins; created_at stays
	if _, err := db.Exec(`UPDATE rec_feedback SET created_at = created_at - INTERVAL '1 day', updated_at = updated_at - INTERVAL '1 day'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, 1, severance, "like", "title"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, 1, TitleRef{TitleMovie, 95396}, "seen", "ask"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.List(ctx, 1)
	if err != nil || len(got) != 2 {
		t.Fatalf("after upsert: %+v, %v", got, err)
	}
	var tv RecFeedback
	for _, f := range got {
		if f.TitleRef == severance {
			tv = f
		}
	}
	if tv.Signal != "like" || tv.Source != "title" || !tv.UpdatedAt.After(tv.CreatedAt) {
		t.Errorf("upserted = %+v, want like from title with created_at kept", tv)
	}

	// An unknown signal is the database's to refuse
	if err := repo.Set(ctx, 1, severance, "love", "shelf"); err == nil {
		t.Error("unknown signal saved")
	}

	// Another member's feedback is theirs (#245): their own signal on the
	// same title, in their list and title states, and theirs to clear.
	if _, err := NewMemberRepository(db).Create(ctx, "Sam", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, 2, severance, "not_interested", "hero"); err != nil {
		t.Fatal(err)
	}
	if theirs, err := repo.List(ctx, 2); err != nil || len(theirs) != 1 || theirs[0].Signal != "not_interested" {
		t.Errorf("member 2's feedback = %+v, %v", theirs, err)
	}
	for memberID, want := range map[int64]string{1: "like", 2: "not_interested"} {
		taste := NewTasteRepository(db).ForMember(memberID)
		states, err := taste.TitleStates(ctx, []TitleRef{severance})
		if err != nil || len(states) != 1 || states[0].Feedback != want {
			t.Errorf("member %d's title state = %+v, %v; want %s", memberID, states, err, want)
		}
		listed, err := taste.TasteFeedback(ctx)
		if err != nil || len(listed) == 0 || listed[0].Signal == "" {
			t.Fatalf("member %d's taste feedback = %+v, %v", memberID, listed, err)
		}
		for _, f := range listed {
			if f.TitleRef == severance && f.Signal != want {
				t.Errorf("member %d's taste feedback on Severance = %s, want %s", memberID, f.Signal, want)
			}
		}
	}
	if err := repo.Clear(ctx, 2, severance); err != nil {
		t.Fatal(err)
	}
	if mine, err := repo.List(ctx, 1); err != nil || len(mine) != 2 {
		t.Errorf("member 1's feedback after member 2 cleared theirs = %+v, %v", mine, err)
	}

	// Clear is idempotent
	for i := 0; i < 2; i++ {
		if err := repo.Clear(ctx, 1, severance); err != nil {
			t.Fatalf("clear %d: %v", i, err)
		}
	}
	if left, err := repo.List(ctx, 1); err != nil || len(left) != 1 || left[0].MediaType != TitleMovie {
		t.Errorf("after clear: %+v, %v", left, err)
	}
	if _, err := NewTasteRepository(db).TasteFeedback(ctx); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound feedback read = %v, want errUnbound", err)
	}
}

func TestTasteProfileRepository(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewTasteProfileRepository(db).ForMember(1)
	ctx := context.Background()

	if p, err := repo.Latest(ctx); err != nil || p != nil {
		t.Fatalf("latest of none = %v, %v", p, err)
	}
	old, err := repo.Insert(ctx, json.RawMessage(`{"loved": 1}`), json.RawMessage(`{"v": 1}`), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	referenced, err := repo.Insert(ctx, json.RawMessage(`{"loved": 2}`), json.RawMessage(`{"v": 2}`), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rec_runs (member_id, kind, status, model, profile_id) VALUES (1, 'manual', 'succeeded', 'x', $1)`,
		referenced.ID); err != nil {
		t.Fatal(err)
	}
	// Another member's old profile, which member 1's builds never prune
	// (#245).
	if _, err := NewMemberRepository(db).Create(ctx, "Sam", ""); err != nil {
		t.Fatal(err)
	}
	theirs, err := NewTasteProfileRepository(db).ForMember(2).Insert(ctx, json.RawMessage(`{"loved": 9}`), json.RawMessage(`{"v": 9}`),
		time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// Both are young: the next insert keeps them
	recent, err := repo.Insert(ctx, json.RawMessage(`{"loved": 3}`), json.RawMessage(`{"v": 3}`), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := repo.Get(ctx, old.ID); p == nil {
		t.Fatal("a young unreferenced profile was pruned")
	}
	// Once they're old, only the referenced one stays
	latest, err := repo.Insert(ctx, json.RawMessage(`{"loved": 4}`), json.RawMessage(`{"v": 4}`), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]bool{old.ID: false, referenced.ID: true, recent.ID: false, latest.ID: true, theirs.ID: true} {
		p, err := repo.Get(ctx, id)
		if err != nil || (p != nil) != want {
			t.Errorf("profile %d exists = %v (%v), want %v", id, p != nil, err, want)
		}
	}
	got, err := repo.Latest(ctx)
	if err != nil || got == nil || got.ID != latest.ID || string(got.Counts) != `{"loved": 4}` || string(got.Profile) != `{"v": 4}` {
		t.Errorf("latest = %+v, %v", got, err)
	}
	if got, err := NewTasteProfileRepository(db).ForMember(2).Latest(ctx); err != nil || got == nil || got.ID != theirs.ID {
		t.Errorf("member 2's latest = %+v, %v; want %d", got, err, theirs.ID)
	}
	if _, err := NewTasteProfileRepository(db).Latest(ctx); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound Latest = %v, want errUnbound", err)
	}
	if _, err := NewTasteProfileRepository(db).ForMember(99).Insert(ctx, json.RawMessage(`{}`), json.RawMessage(`{}`), time.Now()); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("a profile for no member = %v, want member.ErrUnknown", err)
	}
}

// toJSON is v as JSON with every timestamp in UTC, so times compare by
// instant whatever their location.
func toJSON(v any) string {
	b, _ := json.Marshal(v)
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		return string(b)
	}
	b, _ = json.Marshal(utcTimes(tree))
	return string(b)
}

func utcTimes(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = utcTimes(e)
		}
	case []any:
		for i, e := range x {
			x[i] = utcTimes(e)
		}
	case string:
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return v
}

func equalJSON(a, b any) bool {
	return toJSON(a) == toJSON(b)
}
