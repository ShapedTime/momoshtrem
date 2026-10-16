package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// view is a turn as the thread page shows it (ask_turns.view, PLAN §5),
// written when the turn ends. Items keep the facts TMDB gave when they
// were picked; library, watchlist and feedback are joined at read time.
type view struct {
	Steps       []recommend.Step     `json:"steps"`
	Intro       string               `json:"intro"`
	Items       []viewItem           `json:"items"`
	Refinements []string             `json:"refinements"`
	Error       *recommend.TurnError `json:"error"`
	ElapsedMS   *int64               `json:"elapsed_ms"`
}

type viewItem struct {
	MediaType string             `json:"media_type"`
	TMDBID    int                `json:"tmdb_id"`
	MatchTag  string             `json:"match_tag"`
	Reason    string             `json:"reason"`
	Meta      recommend.ItemMeta `json:"meta"`
}

func (it viewItem) ref() library.TitleRef {
	return library.TitleRef{MediaType: it.MediaType, TMDBID: it.TMDBID}
}

// encode is the view as stored, with every list an array.
func (v view) encode() ([]byte, error) {
	v.Steps = orEmpty(v.Steps)
	v.Items = orEmpty(v.Items)
	v.Refinements = orEmpty(v.Refinements)
	return json.Marshal(v)
}

func orEmpty[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// live is what's read at read time about titles: feedback and the
// watchlist (TitleStates), and the library.
type live struct {
	states  map[library.TitleRef]library.TitleState
	library map[catalog.TitleRef]*catalog.SimilarLibrary
}

// readLive looks refs up: two queries for any number of titles.
func readLive(ctx context.Context, titles Titles, lib Library, refs []library.TitleRef) (live, error) {
	l := live{states: map[library.TitleRef]library.TitleState{}, library: map[catalog.TitleRef]*catalog.SimilarLibrary{}}
	if len(refs) == 0 {
		return l, nil
	}
	states, err := titles.TitleStates(ctx, refs)
	if err != nil {
		return l, err
	}
	for _, s := range states {
		l.states[s.TitleRef] = s
	}
	var ids library.TMDBIDs
	for _, ref := range refs {
		if ref.MediaType == library.TitleMovie {
			ids.Movies = append(ids.Movies, ref.TMDBID)
		} else {
			ids.Shows = append(ids.Shows, ref.TMDBID)
		}
	}
	if lib != nil {
		if l.library, err = lib.LibraryStates(ctx, ids); err != nil {
			return l, err
		}
	}
	return l, nil
}

// hidden reports whether feedback hides the title from every AI surface:
// dislike, not interested or seen.
func (l live) hidden(ref library.TitleRef) bool {
	switch l.states[ref].Feedback {
	case recommend.SignalDislike, recommend.SignalNotInterested, recommend.SignalSeen:
		return true
	}
	return false
}

// item is a picked title with what's read live.
func (l live) item(it viewItem) recommend.AskItem {
	ref := it.ref()
	state := l.states[ref]
	out := recommend.AskItem{
		Item: recommend.Item{
			MediaType:   it.MediaType,
			TMDBID:      it.TMDBID,
			ItemMeta:    it.Meta,
			Reason:      it.Reason,
			Library:     l.library[catalog.TitleRef{MediaType: ref.MediaType, TMDBID: ref.TMDBID}],
			OnWatchlist: state.OnWatchlist,
		},
		MatchTag: it.MatchTag,
	}
	if state.Feedback == recommend.SignalLike {
		like := recommend.SignalLike
		out.Feedback = &like
	}
	return out
}

// excluded is why a title can't be picked now ("" when it can).
func (l live) excluded(ref library.TitleRef) string {
	s, ok := l.states[ref]
	if !ok {
		return ""
	}
	return candidates.Exclusion(s)
}

// turnOf is a stored turn as the thread page reads it. A streaming turn has
// its message only; an ended one its view, with items hidden by feedback
// left out.
func turnOf(t library.AskTurn, v *view, l live) recommend.Turn {
	out := recommend.Turn{ID: t.ID, Message: t.Message, Status: t.Status, CreatedAt: t.CreatedAt,
		Steps: []recommend.Step{}, Items: []recommend.AskItem{}, Refinements: []string{}}
	if t.Status == library.AskTurnStreaming {
		return out
	}
	if v == nil {
		// An end that couldn't be written (AbandonTurn) has no view.
		if t.Status == library.AskTurnError {
			out.Error = errorOf(t.ErrorCode)
		}
		return out
	}
	out.Steps = orEmpty(v.Steps)
	out.Intro = v.Intro
	out.Refinements = orEmpty(v.Refinements)
	out.Error = v.Error
	out.ElapsedMS = v.ElapsedMS
	if t.Status == library.AskTurnError && out.Error == nil {
		out.Error = errorOf(t.ErrorCode)
	}
	for _, it := range v.Items {
		if !l.hidden(it.ref()) {
			out.Items = append(out.Items, l.item(it))
		}
	}
	return out
}

// errorOf is an error turn's error when its view has none.
func errorOf(code string) *recommend.TurnError {
	if code == "" {
		code = recommend.AskErrorInternal
	}
	return &recommend.TurnError{Code: code, Message: "Noir couldn't finish this answer. Try again."}
}

// decodeView reads a stored view; nil without one.
func decodeView(raw []byte) (*view, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var v view
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("failed to decode an ask turn's view: %w", err)
	}
	return &v, nil
}

// metaOf is a title's facts from TMDB, as a pick's card shows them.
// today is the server-local date: a title not out yet gets a badge. names
// are TMDB's names for it (its title and original title), to check the
// model's name against.
func metaOf(ctx context.Context, t TMDB, ref library.TitleRef, today time.Time) (m recommend.ItemMeta, names []string, err error) {
	if ref.MediaType == library.TitleMovie {
		d, err := t.MovieDetails(ctx, ref.TMDBID)
		if err != nil {
			return m, nil, err
		}
		m.Title = d.Title
		names = []string{d.Title, d.OriginalTitle}
		m.Year = positive(yearOf(d.ReleaseDate))
		m.PosterPath = nonEmpty(d.PosterPath)
		m.RuntimeMinutes = positive(d.Runtime)
		m.VoteAverage = voteOf(d.VoteAverage, d.VoteCount)
		m.ReleaseDate = dateOf(d.ReleaseDate)
	} else {
		d, err := t.ShowDetails(ctx, ref.TMDBID)
		if err != nil {
			return m, nil, err
		}
		m.Title = d.Name
		names = []string{d.Name, d.OriginalName}
		m.Year = positive(yearOf(d.FirstAirDate))
		if d.Status == "Ended" || d.Status == "Canceled" {
			m.YearEnd = positive(yearOf(d.LastAirDate))
		}
		m.PosterPath = nonEmpty(d.PosterPath)
		seasons := 0
		for _, s := range d.Seasons {
			if s.SeasonNumber > 0 {
				seasons++
			}
		}
		if seasons == 0 {
			seasons = d.NumberOfSeasons
		}
		m.Seasons = positive(seasons)
		m.RuntimeMinutes = positive(d.EpisodeRuntime())
		m.VoteAverage = voteOf(d.VoteAverage, d.VoteCount)
		m.ReleaseDate = dateOf(d.FirstAirDate)
	}
	m.Badge = badgeOf(m.ReleaseDate, today)
	return m, names, nil
}

// badgeOf labels a title that isn't out yet: "Jan 16" within the next
// year, else its year.
func badgeOf(date *string, today time.Time) *string {
	if date == nil {
		return nil
	}
	day, err := time.ParseInLocation("2006-01-02", *date, today.Location())
	if err != nil || !day.After(today) {
		return nil
	}
	label := day.Format("Jan 2")
	if day.After(today.AddDate(1, 0, 0)) {
		label = strconv.Itoa(day.Year())
	}
	return &label
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

func dateOf(date string) *string {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil
	}
	return &date
}

func voteOf(avg float64, count int) *float64 {
	if count <= 0 || avg <= 0 {
		return nil
	}
	return &avg
}

func positive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var _ TMDB = (*tmdb.Cached)(nil)
