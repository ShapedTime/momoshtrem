package identify

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Show fixtures (testdata/shows/*.yaml) describe a show compactly; the
// loader generates its episodes. Episode IDs are season*100000 + episode,
// absolute numbers are cumulative over the regular seasons, and air dates
// run on from the previous episode when a run gives no first_air.
type showFixture struct {
	Title         string          `yaml:"title"`
	OriginalTitle string          `yaml:"original_title"`
	Aliases       []string        `yaml:"aliases"`
	Countries     []string        `yaml:"countries"`
	Year          int             `yaml:"year"`
	IsAnime       bool            `yaml:"is_anime"`
	Ordering      string          `yaml:"ordering"`
	Seasons       []seasonFixture `yaml:"seasons"`
	AltOrderings  []altFixture    `yaml:"alt_orderings"`
}

type seasonFixture struct {
	N        int              `yaml:"n"`
	Runs     []runFixture     `yaml:"runs"`
	Episodes []episodeFixture `yaml:"episodes"`
	Names    map[int]string   `yaml:"names"`
	Types    map[int]string   `yaml:"types"`
}

type runFixture struct {
	Count    int      `yaml:"count"`
	FirstAir string   `yaml:"first_air"`
	Every    int      `yaml:"every"`
	Days     []string `yaml:"days"`
}

type episodeFixture struct {
	E    int    `yaml:"e"`
	Name string `yaml:"name"`
	Air  string `yaml:"air"`
	Type string `yaml:"type"`
}

type altFixture struct {
	Name   string `yaml:"name"`
	Groups []struct {
		N     int    `yaml:"n"`
		From  string `yaml:"from"`
		Count int    `yaml:"count"`
	} `yaml:"groups"`
}

func fixtureID(season, episode int) int64 { return int64(season)*100000 + int64(episode) }

var (
	showsOnce sync.Once
	shows     map[string]*ShowCatalog
	showsErr  error
)

// loadShows reads every fixture in testdata/shows once.
func loadShows(t testing.TB) map[string]*ShowCatalog {
	t.Helper()
	showsOnce.Do(func() {
		shows = map[string]*ShowCatalog{}
		paths, err := filepath.Glob("testdata/shows/*.yaml")
		if err != nil {
			showsErr = err
			return
		}
		for _, p := range paths {
			cat, err := loadShow(p)
			if err != nil {
				showsErr = fmt.Errorf("%s: %w", p, err)
				return
			}
			shows[strings.TrimSuffix(filepath.Base(p), ".yaml")] = cat
		}
	})
	if showsErr != nil {
		t.Fatal(showsErr)
	}
	return shows
}

// show returns a fixture by name.
func show(t testing.TB, name string) *ShowCatalog {
	t.Helper()
	cat, ok := loadShows(t)[name]
	if !ok {
		t.Fatalf("no show fixture %q", name)
	}
	return cat
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func loadShow(path string) (*ShowCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f showFixture
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	cat := &ShowCatalog{
		Title: f.Title, OriginalTitle: f.OriginalTitle, Aliases: f.Aliases, Year: f.Year,
		IsAnime: f.IsAnime, Ordering: Ordering(f.Ordering), Countries: f.Countries,
	}
	if cat.Ordering == "" {
		cat.Ordering = OrderingAired
	}

	var last time.Time
	for _, s := range f.Seasons {
		eps := map[int]*Episode{}
		next := 1
		for _, r := range s.Runs {
			every := r.Every
			if every == 0 {
				every = 7
			}
			days := map[time.Weekday]bool{}
			for _, d := range r.Days {
				wd, ok := weekdays[d]
				if !ok {
					return nil, fmt.Errorf("season %d: unknown day %q", s.N, d)
				}
				days[wd] = true
			}
			air := last
			if r.FirstAir != "" {
				if air, err = time.Parse("2006-01-02", r.FirstAir); err != nil {
					return nil, fmt.Errorf("season %d: %w", s.N, err)
				}
			} else if !last.IsZero() {
				air = advance(last, every, days)
			}
			for i := 0; i < r.Count; i++ {
				if i > 0 {
					air = advance(air, every, days)
				}
				ep := &Episode{ID: fixtureID(s.N, next), Season: s.N, Number: next, Name: "Episode " + strconv.Itoa(next)}
				if !air.IsZero() {
					ep.AirDate = air.Format("2006-01-02")
					last = air
				}
				eps[next] = ep
				next++
			}
		}
		for _, e := range s.Episodes {
			ep := &Episode{ID: fixtureID(s.N, e.E), Season: s.N, Number: e.E, Name: e.Name, AirDate: e.Air, Type: e.Type}
			eps[e.E] = ep
		}
		for n, name := range s.Names {
			if ep, ok := eps[n]; ok {
				ep.Name = name
			} else {
				return nil, fmt.Errorf("season %d: name for missing episode %d", s.N, n)
			}
		}
		for n, typ := range s.Types {
			if ep, ok := eps[n]; ok {
				ep.Type = typ
			}
		}
		numbers := make([]int, 0, len(eps))
		for n := range eps {
			numbers = append(numbers, n)
		}
		sort.Ints(numbers)
		for _, n := range numbers {
			cat.Episodes = append(cat.Episodes, *eps[n])
		}
	}

	sort.SliceStable(cat.Episodes, func(i, j int) bool {
		a, b := cat.Episodes[i], cat.Episodes[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		return a.Number < b.Number
	})
	abs := 1
	for i := range cat.Episodes {
		if cat.Episodes[i].Season > 0 {
			cat.Episodes[i].Absolute = abs
			abs++
		}
	}

	for _, alt := range f.AltOrderings {
		ao := AltOrdering{Name: alt.Name}
		for _, g := range alt.Groups {
			var season, from int
			if _, err := fmt.Sscanf(g.From, "S%02dE%02d", &season, &from); err != nil {
				return nil, fmt.Errorf("alt ordering %q: %w", alt.Name, err)
			}
			group := AltGroup{Number: g.N}
			for e := from; e < from+g.Count; e++ {
				group.EpisodeIDs = append(group.EpisodeIDs, fixtureID(season, e))
			}
			ao.Groups = append(ao.Groups, group)
		}
		cat.AltOrderings = append(cat.AltOrderings, ao)
	}
	return cat, nil
}

// advance moves to the next air date: every days later, or the next of
// days when the run airs on set weekdays.
func advance(t time.Time, every int, days map[time.Weekday]bool) time.Time {
	if len(days) == 0 {
		return t.AddDate(0, 0, every)
	}
	for {
		t = t.AddDate(0, 0, 1)
		if days[t.Weekday()] {
			return t
		}
	}
}

// withoutAliases copies a catalog without its aliases, as the library has
// it until TMDB alternative titles are loaded.
func withoutAliases(cat *ShowCatalog) *ShowCatalog {
	c := *cat
	c.Aliases = nil
	return &c
}

// withoutSpecials copies a catalog without season 0, as when a show's
// specials are off.
func withoutSpecials(cat *ShowCatalog) *ShowCatalog {
	c := *cat
	c.Episodes = nil
	for _, ep := range cat.Episodes {
		if ep.Season != 0 {
			c.Episodes = append(c.Episodes, ep)
		}
	}
	return &c
}

func TestFixtures(t *testing.T) {
	op := show(t, "one-piece")
	var abs1071 *Episode
	for i := range op.Episodes {
		if op.Episodes[i].Absolute == 1071 {
			abs1071 = &op.Episodes[i]
		}
	}
	if abs1071 == nil || abs1071.Season != 21 || abs1071.Number != 79 {
		t.Errorf("One Piece #1071 = %+v, want S21E79", abs1071)
	}

	frieren := show(t, "frieren")
	for _, ep := range frieren.Episodes {
		if ep.Season == 1 && ep.Number == 29 && ep.AirDate != "2026-01-16" {
			t.Errorf("Frieren S01E29 airs %s, want 2026-01-16", ep.AirDate)
		}
	}

	daily := show(t, "the-daily-show")
	for _, ep := range daily.Episodes {
		if ep.Season == 29 && ep.Number == 40 && ep.AirDate != "2024-03-14" {
			t.Errorf("The Daily Show S29E40 airs %s, want 2024-03-14", ep.AirDate)
		}
	}
}
