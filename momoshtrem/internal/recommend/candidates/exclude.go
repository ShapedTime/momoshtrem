package candidates

import (
	"github.com/shapedtime/momoshtrem/internal/library"
)

// What's never picked (PLAN §6 "Recommendations"): a title you watched (a
// movie with a play; a show you started, which a progress row or a play
// says), a title you rated on Trakt (its own rating, whatever use_ratings
// says: you've seen it), a show dropped or hidden on Trakt, and a title
// with negative feedback. Library titles can be picked. The pool, check_seen and R2's
// validation of a run's items all go through Exclusion.

// Why a title can't be picked.
const (
	ExcludedWatched       = "watched"
	ExcludedStarted       = "started"
	ExcludedRated         = "rated"
	ExcludedDropped       = "dropped on Trakt"
	ExcludedHidden        = "hidden on Trakt"
	ExcludedDisliked      = "disliked"
	ExcludedNotInterested = "not interested"
	ExcludedSeen          = "marked seen"
)

// Exclusion is why s can't be picked, "" when it can. Your feedback comes
// first, then Trakt's watch state, your Trakt rating, then Trakt's hidden
// shows.
func Exclusion(s library.TitleState) string {
	switch s.Feedback {
	case "dislike":
		return ExcludedDisliked
	case "not_interested":
		return ExcludedNotInterested
	case "seen":
		return ExcludedSeen
	}
	if s.MediaType == library.TitleMovie {
		switch {
		case s.Started || s.Plays > 0:
			return ExcludedWatched
		case s.Rating != nil:
			return ExcludedRated
		}
		return ""
	}
	switch {
	case s.Aired > 0 && s.Completed >= s.Aired:
		return ExcludedWatched
	case s.Started || s.Plays > 0:
		return ExcludedStarted
	case s.Rating != nil:
		return ExcludedRated
	case s.Dropped:
		return ExcludedDropped
	case s.Hidden:
		return ExcludedHidden
	}
	return ""
}

// Known is what the database knows about every title it has a state for,
// in bulk: the pool's exclusions and its library and watchlist marks.
type Known struct {
	states    map[library.TitleRef]*library.TitleState
	library   map[library.TitleRef]bool
	watchlist map[library.TitleRef]bool
}

// NewKnown gathers the bulk reads into title states. Only a title's own
// rating counts (TitleState.Rating), not a show's from its episodes.
func NewKnown(movies []library.WatchedMovie, shows []library.WatchedShow, ratings []library.TasteRating,
	hidden []library.HiddenShow, feedback []library.TasteFeedback, titles []library.LibraryTitle, watchlist []library.TitleRef) *Known {
	k := &Known{states: map[library.TitleRef]*library.TitleState{}, library: map[library.TitleRef]bool{}, watchlist: map[library.TitleRef]bool{}}
	get := func(ref library.TitleRef) *library.TitleState {
		s := k.states[ref]
		if s == nil {
			s = &library.TitleState{TitleRef: ref}
			k.states[ref] = s
		}
		return s
	}
	for _, m := range movies {
		s := get(library.TitleRef{MediaType: library.TitleMovie, TMDBID: m.TMDBID})
		s.Plays, s.Started = m.Plays, true
	}
	for _, sh := range shows {
		s := get(library.TitleRef{MediaType: library.TitleTV, TMDBID: sh.TMDBID})
		s.Plays, s.Started, s.Aired, s.Completed = sh.Plays, true, sh.Aired, sh.Completed
	}
	for _, r := range ratings {
		if !r.FromEpisodes {
			rating := r.Rating
			get(r.TitleRef).Rating = &rating
		}
	}
	for _, h := range hidden {
		s := get(library.TitleRef{MediaType: library.TitleTV, TMDBID: h.TMDBID})
		s.Dropped, s.Hidden = h.Dropped, h.FromProgress
	}
	for _, f := range feedback {
		get(f.TitleRef).Feedback = f.Signal
	}
	for _, t := range titles {
		k.library[t.TitleRef] = true
	}
	for _, ref := range watchlist {
		k.watchlist[ref] = true
	}
	return k
}

// Exclusion is why ref can't be picked, "" when it can.
func (k *Known) Exclusion(ref library.TitleRef) string {
	if s := k.states[ref]; s != nil {
		return Exclusion(*s)
	}
	return ""
}

// InLibrary reports whether ref is in the library.
func (k *Known) InLibrary(ref library.TitleRef) bool { return k.library[ref] }

// OnWatchlist reports whether ref is on the Trakt watchlist.
func (k *Known) OnWatchlist(ref library.TitleRef) bool { return k.watchlist[ref] }
