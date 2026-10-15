package identify

import "testing"

func TestRankMovieFiles(t *testing.T) {
	const gb = 1 << 30
	tests := []struct {
		name     string
		movie    MovieInfo
		files    []TorrentFile
		want     []string // candidate paths, best first
		wantConf Confidence
	}{
		{"single file", MovieInfo{Title: "Dune: Part Two", Year: 2024, RuntimeMin: 166},
			[]TorrentFile{{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.H.265-FLUX.mkv", 20 * gb}},
			[]string{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.H.265-FLUX.mkv"}, ConfidenceHigh},
		{"sample and extras are left out", MovieInfo{Title: "Interstellar", Year: 2014, RuntimeMin: 169},
			[]TorrentFile{
				{"Interstellar (2014)/Interstellar.2014.1080p.BluRay.x264.mkv", 12 * gb},
				{"Interstellar (2014)/Sample/interstellar.sample.mkv", 50 << 20},
				{"Interstellar (2014)/Featurettes/The Science of Interstellar.mkv", 2 * gb},
				{"Interstellar (2014)/Interstellar.2014.Trailer.mkv", 100 << 20},
				{"Interstellar (2014)/Interstellar.2014.nfo", 1 << 10},
			},
			[]string{"Interstellar (2014)/Interstellar.2014.1080p.BluRay.x264.mkv"}, ConfidenceHigh},
		{"a collection pack goes to review", MovieInfo{Title: "Back to the Future Part II", Year: 1989, RuntimeMin: 108},
			[]TorrentFile{
				{"Back to the Future Trilogy/Back.to.the.Future.1985.1080p.BluRay.mkv", 9 * gb},
				{"Back to the Future Trilogy/Back.to.the.Future.Part.II.1989.1080p.BluRay.mkv", 8 * gb},
				{"Back to the Future Trilogy/Back.to.the.Future.Part.III.1990.1080p.BluRay.mkv", 8.5 * gb},
			},
			[]string{
				"Back to the Future Trilogy/Back.to.the.Future.Part.II.1989.1080p.BluRay.mkv",
				"Back to the Future Trilogy/Back.to.the.Future.1985.1080p.BluRay.mkv",
				"Back to the Future Trilogy/Back.to.the.Future.Part.III.1990.1080p.BluRay.mkv",
			}, ConfidenceMedium},
		{"a title with a marker word", MovieInfo{Title: "Trailer Park Boys: The Movie", Year: 2006, RuntimeMin: 95},
			[]TorrentFile{{"Trailer.Park.Boys.The.Movie.2006.1080p.WEB.mkv", 4 * gb}},
			[]string{"Trailer.Park.Boys.The.Movie.2006.1080p.WEB.mkv"}, ConfidenceHigh},
		{"too small for the runtime is a sample", MovieInfo{Title: "Heat", Year: 1995, RuntimeMin: 170},
			[]TorrentFile{{"Heat.1995.1080p.mkv", 10 * gb}, {"heat.mkv", 30 << 20}},
			[]string{"Heat.1995.1080p.mkv"}, ConfidenceHigh},
		{"no video", MovieInfo{Title: "Heat"},
			[]TorrentFile{{"Heat.1995.nfo", 1 << 10}}, nil, ""},
		// From the PR #52 review: these gave "No video files found".
		{"a site prefix and a title word that marks extras", MovieInfo{Title: "The Interview", Year: 2014, RuntimeMin: 112},
			[]TorrentFile{{"www.UIndex.org    -    The.Interview.2014.1080p.BluRay.x264-GRP.mkv", 8 * gb}},
			[]string{"www.UIndex.org    -    The.Interview.2014.1080p.BluRay.x264-GRP.mkv"}, ConfidenceHigh},
		{"the title is a featurette phrase", MovieInfo{Title: "Behind the Candelabra", Year: 2013, RuntimeMin: 118},
			[]TorrentFile{{"HBO.Behind.the.Candelabra.2013.720p.mkv", 4 * gb}},
			[]string{"HBO.Behind.the.Candelabra.2013.720p.mkv"}, ConfidenceHigh},
		{"the largest plausible video is never dropped by name", MovieInfo{Title: "Heat", Year: 1995, RuntimeMin: 170},
			[]TorrentFile{{"Heat.1995.Bonus.Edition.1080p.mkv", 10 * gb}, {"Heat.1995.Trailer.mkv", 100 << 20}},
			[]string{"Heat.1995.Bonus.Edition.1080p.mkv"}, ConfidenceHigh},
		{"never every video", MovieInfo{Title: "Heat", Year: 1995, RuntimeMin: 170},
			[]TorrentFile{{"Heat.1995.Teaser.mkv", 50 << 20}},
			[]string{"Heat.1995.Teaser.mkv"}, ConfidenceMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RankMovieFiles(tt.files, tt.movie)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d candidates %+v, want %v", len(got), got, tt.want)
			}
			for i, c := range got {
				if c.Path != tt.want[i] {
					t.Errorf("candidate %d = %s, want %s", i, c.Path, tt.want[i])
				}
				if c.Reason == "" || len(c.Reason) > 200 {
					t.Errorf("candidate %d reason %q", i, c.Reason)
				}
			}
			if len(got) > 0 && got[0].Confidence != tt.wantConf {
				t.Errorf("top confidence = %s (%s), want %s", got[0].Confidence, got[0].Reason, tt.wantConf)
			}
		})
	}
}
