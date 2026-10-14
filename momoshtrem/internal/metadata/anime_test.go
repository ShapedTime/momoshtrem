package metadata

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

func TestIsAnime(t *testing.T) {
	animation := []tmdb.Genre{{ID: 16, Name: "Animation"}}
	drama := []tmdb.Genre{{ID: 18, Name: "Drama"}}
	animeKeyword := []tmdb.Keyword{{ID: AnimeKeywordID, Name: "anime"}}
	otherKeyword := []tmdb.Keyword{{ID: 6152, Name: "elf"}}

	tests := []struct {
		name     string
		genres   []tmdb.Genre
		language string
		country  []string
		keywords []tmdb.Keyword
		want     bool
	}{
		{"animated, Japanese language", animation, "ja", nil, nil, true},
		{"animated, made in Japan", animation, "en", []string{"US", "JP"}, nil, true},
		{"anime keyword alone", drama, "en", []string{"US"}, animeKeyword, true},
		{"anime keyword without genres", nil, "", nil, animeKeyword, true},
		{"Japanese live action", drama, "ja", []string{"JP"}, otherKeyword, false},
		{"Western animation", animation, "en", []string{"US"}, otherKeyword, false},
		{"Korean animation", animation, "ko", []string{"KR"}, nil, false},
		{"genre matched by ID, not name", []tmdb.Genre{{ID: 16, Name: "Animación"}}, "ja", nil, nil, true},
		{"nothing at all", nil, "", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnime(tt.genres, tt.language, tt.country, tt.keywords); got != tt.want {
				t.Errorf("IsAnime = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultOrdering(t *testing.T) {
	if got := DefaultOrdering(true); got != library.OrderingAbsolute {
		t.Errorf("anime default = %q, want absolute", got)
	}
	if got := DefaultOrdering(false); got != library.OrderingAired {
		t.Errorf("default = %q, want aired", got)
	}
}
