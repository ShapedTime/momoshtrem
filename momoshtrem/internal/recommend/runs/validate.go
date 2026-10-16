package runs

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Details resolves titles on TMDB (tmdb.Cached).
type Details interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
}

// Titles looks up what's known about titles (library.TasteRepository).
type Titles interface {
	TitleStates(ctx context.Context, refs []library.TitleRef) ([]library.TitleState, error)
}

// detailWorkers is how many titles are resolved on TMDB at once.
const detailWorkers = 4

// proposed is a title the agent submitted: its id, and the name and year
// it gave (0: unsure).
type proposed struct {
	Ref   library.TitleRef
	Title string
	Year  int
}

// verdict is what validation made of a proposed title: kept with its
// snapshot, or dropped with why (for the model to read).
type verdict struct {
	Ref     library.TitleRef
	Meta    recommend.ItemMeta
	Dropped string
}

// validator checks the agent's titles (PLAN §7 R2): each must exist on
// TMDB under the name the agent gave, and be one the exclusion rule lets
// through (not watched, started, rated, hidden or dropped on Trakt, or
// turned down). Library titles stay.
type validator struct {
	titles  Titles
	details Details
	now     func() time.Time
}

// check validates titles in order; comingSoon badges a show's next
// episode. Only the run's context ending or a database error is an error.
// TMDB and the name come first: a remembered id that happens to be
// watched would otherwise drop as "watched", and the model would think the
// title it named is.
func (v *validator) check(ctx context.Context, titles []proposed, comingSoon bool) ([]verdict, error) {
	out := make([]verdict, len(titles))
	for i, t := range titles {
		out[i].Ref = t.Ref
		if !validRef(t.Ref) {
			out[i].Dropped = "media_type must be movie or tv, with a TMDB id (a positive whole number)"
		}
	}

	today := v.now().Format(time.DateOnly)
	var (
		wg   sync.WaitGroup
		work = make(chan int)
	)
	for range detailWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				meta, dropped := v.resolve(ctx, titles[i], comingSoon, today)
				out[i].Meta, out[i].Dropped = meta, dropped
			}
		}()
	}
	for i := range out {
		if out[i].Dropped == "" && ctx.Err() == nil {
			work <- i
		}
	}
	close(work)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lookup []library.TitleRef
	for i := range out {
		if out[i].Dropped == "" {
			lookup = append(lookup, out[i].Ref)
		}
	}
	states, err := v.states(ctx, lookup)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Dropped != "" {
			continue
		}
		if why := candidates.Exclusion(states[out[i].Ref]); why != "" {
			out[i].Meta, out[i].Dropped = recommend.ItemMeta{}, "can't be picked: "+why
		}
	}
	return out, nil
}

func (v *validator) states(ctx context.Context, refs []library.TitleRef) (map[library.TitleRef]library.TitleState, error) {
	out := map[library.TitleRef]library.TitleState{}
	if len(refs) == 0 {
		return out, nil
	}
	list, err := v.titles.TitleStates(ctx, refs)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		out[s.TitleRef] = s
	}
	return out, nil
}

// resolve finds a title on TMDB, checks the agent's name for it, and
// snapshots its facts.
func (v *validator) resolve(ctx context.Context, t proposed, comingSoon bool, today string) (recommend.ItemMeta, string) {
	what := t.Ref.MediaType + " " + strconv.Itoa(t.Ref.TMDBID)
	if t.Ref.MediaType == library.TitleMovie {
		d, err := v.details.MovieDetails(ctx, t.Ref.TMDBID)
		if err != nil {
			return recommend.ItemMeta{}, tmdbDrop(err, what)
		}
		if !recommend.SameTitle(t.Title, d.Title, d.OriginalTitle) || !recommend.SameYear(t.Year, d.Year()) {
			return recommend.ItemMeta{}, recommend.Mismatch(t.Title, t.Year, d.Title, d.Year())
		}
		return movieMeta(d, today), ""
	}
	d, err := v.details.ShowDetails(ctx, t.Ref.TMDBID)
	if err != nil {
		return recommend.ItemMeta{}, tmdbDrop(err, what)
	}
	if !recommend.SameTitle(t.Title, d.Name, d.OriginalName) || !recommend.SameYear(t.Year, d.Year()) {
		return recommend.ItemMeta{}, recommend.Mismatch(t.Title, t.Year, d.Name, d.Year())
	}
	return showMeta(d, comingSoon, today), ""
}

func tmdbDrop(err error, what string) string {
	if errors.Is(err, tmdb.ErrNotFound) {
		return "TMDB has no " + what + ": look the title up with tmdb_search"
	}
	return "TMDB didn't answer for " + what + "; pick another title or submit it again later"
}

func validRef(r library.TitleRef) bool {
	return (r.MediaType == library.TitleMovie || r.MediaType == library.TitleTV) && r.TMDBID > 0 && r.TMDBID <= 1<<31-1
}

func movieMeta(d *tmdb.MovieDetails, today string) recommend.ItemMeta {
	m := recommend.ItemMeta{
		Title:          d.Title,
		Year:           positive(d.Year()),
		PosterPath:     nonEmpty(d.PosterPath),
		RuntimeMinutes: positive(d.Runtime),
		VoteAverage:    voteAverage(d.VoteAverage, d.VoteCount),
		ReleaseDate:    date(d.ReleaseDate),
	}
	m.Badge = badge(d.ReleaseDate, today)
	return m
}

func showMeta(d *tmdb.ShowDetails, comingSoon bool, today string) recommend.ItemMeta {
	m := recommend.ItemMeta{
		Title:          d.Name,
		Year:           positive(d.Year()),
		PosterPath:     nonEmpty(d.PosterPath),
		RuntimeMinutes: positive(d.EpisodeRuntime()),
		VoteAverage:    voteAverage(d.VoteAverage, d.VoteCount),
		ReleaseDate:    date(d.FirstAirDate),
	}
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
	if (d.Status == "Ended" || d.Status == "Canceled") && len(d.LastAirDate) >= 4 {
		if y, err := strconv.Atoi(d.LastAirDate[:4]); err == nil {
			m.YearEnd = positive(y)
		}
	}
	m.Badge = badge(d.FirstAirDate, today)
	if m.Badge == nil && comingSoon && d.NextEpisodeToAir != nil {
		m.Badge = badge(d.NextEpisodeToAir.AirDate, today)
	}
	return m
}

// badge is a poster label for a date still to come: "Jan 16" within a
// year, else its year ("2027"). Nil for a date that's passed or unknown.
func badge(day, today string) *string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil || day < today {
		return nil
	}
	now, _ := time.Parse(time.DateOnly, today)
	label := t.Format("Jan 2")
	if t.After(now.AddDate(1, 0, 0)) {
		label = strconv.Itoa(t.Year())
	}
	return &label
}

func date(s string) *string {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return nil
	}
	return &s
}

func voteAverage(avg float64, count int) *float64 {
	if count == 0 {
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
