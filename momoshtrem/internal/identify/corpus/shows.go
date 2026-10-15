package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Show is a show fixture with its generated episodes. It mirrors
// identify.ShowCatalog.
type Show struct {
	Title         string
	OriginalTitle string
	Aliases       []string
	Year          int
	IsAnime       bool
	Ordering      string
	Episodes      []Episode
	AltOrderings  []AltOrdering
	Countries     []string
}

// Episode mirrors identify.Episode field for field, so
// identify.Episode(e) converts it.
type Episode struct {
	ID       int64
	Season   int
	Number   int
	Absolute int
	Name     string
	AirDate  string
	Type     string
}

// AltOrdering mirrors identify.AltOrdering.
type AltOrdering struct {
	Name   string
	Groups []AltGroup
}

// AltGroup mirrors identify.AltGroup field for field.
type AltGroup struct {
	Number     int
	EpisodeIDs []int64
}

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

// FixtureID is a fixture episode's ID.
func FixtureID(season, episode int) int64 { return int64(season)*100000 + int64(episode) }

// LoadShows reads every fixture in dir/shows, keyed by file name without
// ".yaml", where dir is the testdata directory.
func LoadShows(dir string) (map[string]*Show, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "shows", "*.yaml"))
	if err != nil {
		return nil, err
	}
	shows := map[string]*Show{}
	for _, p := range paths {
		s, err := loadShow(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		shows[strings.TrimSuffix(filepath.Base(p), ".yaml")] = s
	}
	return shows, nil
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func loadShow(path string) (*Show, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f showFixture
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	s := &Show{
		Title: f.Title, OriginalTitle: f.OriginalTitle, Aliases: f.Aliases, Year: f.Year,
		IsAnime: f.IsAnime, Ordering: f.Ordering, Countries: f.Countries,
	}
	if s.Ordering == "" {
		s.Ordering = "aired"
	}

	var last time.Time
	for _, sf := range f.Seasons {
		eps := map[int]*Episode{}
		next := 1
		for _, r := range sf.Runs {
			every := r.Every
			if every == 0 {
				every = 7
			}
			days := map[time.Weekday]bool{}
			for _, d := range r.Days {
				wd, ok := weekdays[d]
				if !ok {
					return nil, fmt.Errorf("season %d: unknown day %q", sf.N, d)
				}
				days[wd] = true
			}
			air := last
			if r.FirstAir != "" {
				if air, err = time.Parse("2006-01-02", r.FirstAir); err != nil {
					return nil, fmt.Errorf("season %d: %w", sf.N, err)
				}
			} else if !last.IsZero() {
				air = advance(last, every, days)
			}
			for i := 0; i < r.Count; i++ {
				if i > 0 {
					air = advance(air, every, days)
				}
				ep := &Episode{ID: FixtureID(sf.N, next), Season: sf.N, Number: next, Name: "Episode " + strconv.Itoa(next)}
				if !air.IsZero() {
					ep.AirDate = air.Format("2006-01-02")
					last = air
				}
				eps[next] = ep
				next++
			}
		}
		for _, e := range sf.Episodes {
			eps[e.E] = &Episode{ID: FixtureID(sf.N, e.E), Season: sf.N, Number: e.E, Name: e.Name, AirDate: e.Air, Type: e.Type}
		}
		for n, name := range sf.Names {
			ep, ok := eps[n]
			if !ok {
				return nil, fmt.Errorf("season %d: name for missing episode %d", sf.N, n)
			}
			ep.Name = name
		}
		for n, typ := range sf.Types {
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
			s.Episodes = append(s.Episodes, *eps[n])
		}
	}

	sort.SliceStable(s.Episodes, func(i, j int) bool {
		a, b := s.Episodes[i], s.Episodes[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		return a.Number < b.Number
	})
	abs := 1
	for i := range s.Episodes {
		if s.Episodes[i].Season > 0 {
			s.Episodes[i].Absolute = abs
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
				group.EpisodeIDs = append(group.EpisodeIDs, FixtureID(season, e))
			}
			ao.Groups = append(ao.Groups, group)
		}
		s.AltOrderings = append(s.AltOrderings, ao)
	}
	return s, nil
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
