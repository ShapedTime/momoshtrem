package identify

import (
	"fmt"
	"testing"
	"time"
)

// bigShow is a One Piece-sized show with every episode named.
func bigShow() *ShowCatalog {
	cat := &ShowCatalog{Title: "Big Show", IsAnime: true, Ordering: OrderingAbsolute}
	words := []string{"pirate", "king", "island", "storm", "crew", "treasure", "map", "ship", "sea", "battle", "dream", "friend"}
	abs := 1
	day := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for s := 1; s <= 20; s++ {
		for e := 1; e <= 55; e++ {
			name := fmt.Sprintf("The %s of the %s and the %s %d", words[(s+e)%12], words[(s*e)%12], words[(s+2*e)%12], abs)
			cat.Episodes = append(cat.Episodes, Episode{ID: fixtureID(s, e), Season: s, Number: e, Absolute: abs,
				Name: name, AirDate: day.Format("2006-01-02")})
			abs++
			day = day.AddDate(0, 0, 7)
		}
	}
	return cat
}

// A 1,100-file scene pack against a 1,100-episode named show: about 0.2 s
// per Classify on an M-series Mac.
func BenchmarkClassifyBigPack(b *testing.B) {
	cat := bigShow()
	var files []TorrentFile
	for _, ep := range cat.Episodes {
		files = append(files, TorrentFile{
			Path: fmt.Sprintf("Big.Show.Complete/Big.Show.S%02dE%02d.%s.1080p.WEB.x264-GRP.mkv", ep.Season, ep.Number, "Some.Episode.Title"),
			Size: 1 << 30,
		})
	}
	in := Input{TorrentName: "Big.Show.Complete", Files: files, Show: cat, Now: corpusNow}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Classify(in)
	}
}
