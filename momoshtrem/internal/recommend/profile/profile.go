// Package profile is R1's taste profile (PLAN §7 R1, #153): what you watch,
// weighed by recency, your ratings (only with Trakt's use_ratings on) and
// how much of it you finished, summed up as compact JSON for the picks
// agent. It's deterministic: no LLM.
//
// Entry points: Builder.CheckHistory (is there enough to run?),
// Builder.Build (the profile), Builder.Save (a taste_profiles row for a
// run) and Builder.Current (the newest stored one, rebuilt when stale).
package profile

import (
	"strings"
	"time"
)

// MinLoved is how many loved titles a run needs (PLAN §6: fewer is
// no_history).
const MinLoved = 5

// MaxBytes is the profile's budget as JSON: about 4k tokens.
const MaxBytes = 16 << 10

// Profile is the taste profile, as get_taste_profile answers it and
// taste_profiles stores it. Shares (w) are the part of your weighted
// history a value covers, 0–1.
type Profile struct {
	BuiltAt time.Time `json:"built_at"`
	Counts  Counts    `json:"counts"`
	// RatingsUsed: Trakt's use_ratings was on, so ratings weighed titles
	// and made loved titles and aversions.
	RatingsUsed bool       `json:"ratings_used"`
	Mix         Mix        `json:"mix"`
	Runtime     Runtime    `json:"runtime"`
	Genres      []Weighted `json:"genres"`
	Keywords    []Weighted `json:"keywords"`
	Creators    []Weighted `json:"creators"` // directors and show creators
	Cast        []Weighted `json:"cast"`
	Decades     []Weighted `json:"decades"`   // "1990s"
	Languages   []Weighted `json:"languages"` // original language, ISO 639-1
	Countries   []Weighted `json:"countries"` // origin country, ISO 3166-1
	// Loved: rated 8 or more, liked, or completed (a movie with a play, a
	// show with every aired episode watched, a title marked seen), but
	// never a title rated 4 or less or disliked. Heaviest first.
	Loved []Title `json:"loved"`
	// Dropped: shows started and left under 30% watched for 60 days, or
	// dropped on Trakt. Newest first.
	Dropped []Title `json:"dropped"`
	// Recent: watched in the last 30 days, newest first.
	Recent []Title `json:"recent"`
	// Aversions: rated 4 or less, disliked or not interested. Newest first.
	Aversions []Title `json:"aversions"`
}

// Counts are the profile's totals (taste_profiles.counts): no_history
// reads Loved.
type Counts struct {
	Loved     int `json:"loved"`
	Completed int `json:"completed"`
	Dropped   int `json:"dropped"`
	Rated     int `json:"rated"` // ratings weighed: 0 with use_ratings off
	Plays     int `json:"plays"` // movie and episode plays
}

// Enough reports whether there's enough history for a run, given Trakt is
// connected: at least MinLoved loved titles.
func (c Counts) Enough() bool { return c.Loved >= MinLoved }

// Mix is what kind of things you watch.
type Mix struct {
	Movies     int     `json:"movies"` // titles that count toward the profile
	Shows      int     `json:"shows"`
	MovieShare float64 `json:"movie_share"` // weighted
	AnimeShare float64 `json:"anime_share"` // weighted, of the titles TMDB described
}

// Runtime is how long the things you watch run, in minutes (0: unknown).
type Runtime struct {
	MovieMedian   int `json:"movie_median,omitempty"`
	MovieP90      int `json:"movie_p90,omitempty"` // rarely longer than this
	EpisodeMedian int `json:"episode_median,omitempty"`
}

// Weighted is a genre, keyword, person, decade, language or country with
// its share. ID is TMDB's (genres, keywords, people); 0 otherwise.
type Weighted struct {
	ID   int     `json:"id,omitempty"`
	Name string  `json:"name"`
	W    float64 `json:"w"`
}

// Title is a title on one of the profile's lists. Note says why it's there:
// "rated 9", "completed", "5 of 73 · idle 640 days".
type Title struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Note      string `json:"note"`
}

// NoteSeen leads a loved title's note when only seen feedback says you
// watched it: no play, so nothing says when.
const NoteSeen = "marked seen"

// SeenOnly reports whether only seen feedback makes t loved. It weighs at
// the recency floor and never seeds the candidate pool.
func (t Title) SeenOnly() bool { return strings.HasPrefix(t.Note, NoteSeen) }

// Readiness is whether a run can go ahead (refresh's no_history check):
// Trakt connected and at least MinLoved loved titles.
type Readiness struct {
	Connected bool
	Counts    Counts
}

// Enough reports whether a run can go ahead.
func (r Readiness) Enough() bool { return r.Connected && r.Counts.Enough() }
