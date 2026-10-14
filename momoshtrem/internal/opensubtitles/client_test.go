package opensubtitles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchQuery(t *testing.T) {
	tests := []struct {
		name   string
		params SearchParams
		want   string
	}{
		{
			"movie by its TMDB ID",
			SearchParams{Type: "movie", TMDBID: 157336, Languages: []string{"en"}},
			"languages=en&tmdb_id=157336&type=movie",
		},
		{
			// tmdb_id is the movie's or the episode's own ID; the show's
			// goes in parent_tmdb_id with the season and episode.
			"episode by the show's TMDB ID",
			SearchParams{Type: "episode", ParentTMDBID: 95396, SeasonNumber: 2, EpisodeNumber: 5, Languages: []string{"en"}},
			"episode_number=5&languages=en&parent_tmdb_id=95396&season_number=2&type=episode",
		},
		{
			"special: season 0 is sent",
			SearchParams{Type: "episode", ParentTMDBID: 57243, SeasonNumber: 0, EpisodeNumber: 3},
			"episode_number=3&parent_tmdb_id=57243&season_number=0&type=episode",
		},
		{
			"languages lowercase, sorted, deduplicated",
			SearchParams{Type: "movie", TMDBID: 1, Languages: []string{"RU", "en", " az ", "en", ""}},
			"languages=az%2Cen%2Cru&tmdb_id=1&type=movie",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchQuery(tt.params).Encode(); got != tt.want {
				t.Errorf("query = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestClient_SearchEpisode(t *testing.T) {
	var gotQuery, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subtitles" {
			http.NotFound(w, r)
			return
		}
		gotQuery, gotKey = r.URL.RawQuery, r.Header.Get("Api-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":1,"page":1,"data":[{"id":"1","type":"subtitle","attributes":{
			"language":"en","download_count":7,"release":"Severance.S02E05.1080p.WEB-DL-NTb",
			"files":[{"file_id":42,"file_name":"x.srt"}]}}]}`))
	}))
	defer srv.Close()

	c := NewClient("key", "", "", WithBaseURL(srv.URL+"/"))
	resp, err := c.Search(context.Background(), SearchParams{
		Type: "episode", ParentTMDBID: 95396, SeasonNumber: 2, EpisodeNumber: 5, Languages: []string{"en"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if want := "episode_number=5&languages=en&parent_tmdb_id=95396&season_number=2&type=episode"; gotQuery != want {
		t.Errorf("query = %s, want %s", gotQuery, want)
	}
	if gotKey != "key" {
		t.Errorf("Api-Key = %q", gotKey)
	}
	if len(resp.Data) != 1 || resp.Data[0].Attributes.Files[0].FileID != 42 {
		t.Errorf("response = %+v", resp)
	}
}
