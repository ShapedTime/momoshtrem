package identify

import (
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify/corpus"
)

// Show fixtures live in testdata/shows; package corpus loads them and
// generates their episodes (see corpus/shows.go).

func fixtureID(season, episode int) int64 { return corpus.FixtureID(season, episode) }

var (
	showsOnce sync.Once
	shows     map[string]*ShowCatalog
	showsErr  error
)

// loadShows reads every fixture in testdata/shows once.
func loadShows(t testing.TB) map[string]*ShowCatalog {
	t.Helper()
	showsOnce.Do(func() {
		fixtures, err := corpus.LoadShows("testdata")
		if err != nil {
			showsErr = err
			return
		}
		shows = make(map[string]*ShowCatalog, len(fixtures))
		for name, s := range fixtures {
			shows[name] = fromCorpus(s)
		}
	})
	if showsErr != nil {
		t.Fatal(showsErr)
	}
	return shows
}

// fromCorpus converts a corpus show. cmd/classifier-eval has the same few
// lines: corpus can't import identify, or these tests couldn't import it.
func fromCorpus(s *corpus.Show) *ShowCatalog {
	cat := &ShowCatalog{
		Title: s.Title, OriginalTitle: s.OriginalTitle, Aliases: s.Aliases, Year: s.Year,
		IsAnime: s.IsAnime, Ordering: Ordering(s.Ordering), Countries: s.Countries,
	}
	for _, e := range s.Episodes {
		cat.Episodes = append(cat.Episodes, Episode(e))
	}
	for _, a := range s.AltOrderings {
		ao := AltOrdering{Name: a.Name}
		for _, g := range a.Groups {
			ao.Groups = append(ao.Groups, AltGroup(g))
		}
		cat.AltOrderings = append(cat.AltOrderings, ao)
	}
	return cat
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
