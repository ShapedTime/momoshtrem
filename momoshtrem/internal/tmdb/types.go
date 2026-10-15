package tmdb

import (
	"encoding/json"
	"strconv"
)

// Genre is a TMDB genre. ID 16 is Animation.
type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// GenreAnimation is TMDB's Animation genre ID, for movies and TV alike.
const GenreAnimation = 16

// Keyword is a TMDB keyword.
type Keyword struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Keywords holds a title's keywords. TMDB nests them under "keywords" for
// movies but "results" for TV; both decode into Keywords.
type Keywords struct {
	Keywords []Keyword `json:"keywords"`
}

// UnmarshalJSON accepts both the movie and the TV shape.
func (k *Keywords) UnmarshalJSON(data []byte) error {
	var raw struct {
		Keywords []Keyword `json:"keywords"`
		Results  []Keyword `json:"results"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	k.Keywords = append(raw.Keywords, raw.Results...)
	return nil
}

// ExternalIDs are a title's IDs on other services.
type ExternalIDs struct {
	IMDbID string `json:"imdb_id"`
	TVDBID int    `json:"tvdb_id"` // TV only; 0 when unknown
}

// CastMember is one credited actor.
type CastMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
}

// CrewMember is one credited crew member.
type CrewMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}

// Credits is a movie's (or a single season's) cast and crew.
type Credits struct {
	Cast []CastMember `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

// Directors returns the names of the crew credited as Director.
func (c Credits) Directors() []string {
	var names []string
	for _, m := range c.Crew {
		if m.Job == "Director" {
			names = append(names, m.Name)
		}
	}
	return names
}

// AggregateRole is one character an actor played across a show.
type AggregateRole struct {
	Character    string `json:"character"`
	EpisodeCount int    `json:"episode_count"`
}

// AggregateCastMember is an actor credited across every season of a show.
type AggregateCastMember struct {
	ID                int             `json:"id"`
	Name              string          `json:"name"`
	ProfilePath       string          `json:"profile_path"`
	Order             int             `json:"order"`
	TotalEpisodeCount int             `json:"total_episode_count"`
	Roles             []AggregateRole `json:"roles"`
}

// Character returns the actor's main role (the first TMDB lists).
func (m AggregateCastMember) Character() string {
	if len(m.Roles) == 0 {
		return ""
	}
	return m.Roles[0].Character
}

// AggregateJob is one job a crew member did across a show.
type AggregateJob struct {
	Job          string `json:"job"`
	EpisodeCount int    `json:"episode_count"`
}

// AggregateCrewMember is a crew member credited across a show.
type AggregateCrewMember struct {
	ID                int            `json:"id"`
	Name              string         `json:"name"`
	Department        string         `json:"department"`
	ProfilePath       string         `json:"profile_path"`
	TotalEpisodeCount int            `json:"total_episode_count"`
	Jobs              []AggregateJob `json:"jobs"`
}

// AggregateCredits is a show's cast and crew across all seasons.
type AggregateCredits struct {
	Cast []AggregateCastMember `json:"cast"`
	Crew []AggregateCrewMember `json:"crew"`
}

// Video is a trailer, teaser, clip... hosted on YouTube or Vimeo.
type Video struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Site        string `json:"site"`
	Type        string `json:"type"`
	Official    bool   `json:"official"`
	PublishedAt string `json:"published_at"`
	Language    string `json:"iso_639_1"`
}

// Videos lists a title's videos.
type Videos struct {
	Results []Video `json:"results"`
}

// TrailerKey returns the YouTube key of the best trailer: an official
// trailer, then any trailer, then a teaser. "" when there is none.
func (v Videos) TrailerKey() string {
	best, bestRank := "", 0
	for _, video := range v.Results {
		if video.Site != "YouTube" || video.Key == "" {
			continue
		}
		rank := 0
		switch {
		case video.Type == "Trailer" && video.Official:
			rank = 3
		case video.Type == "Trailer":
			rank = 2
		case video.Type == "Teaser":
			rank = 1
		}
		if rank > bestRank {
			best, bestRank = video.Key, rank
		}
	}
	return best
}

// ListItem is a movie or show in a list (recommendations, similar).
type ListItem struct {
	ID               int     `json:"id"`
	MediaType        string  `json:"media_type,omitempty"`
	Title            string  `json:"title,omitempty"` // Movies
	Name             string  `json:"name,omitempty"`  // TV
	OriginalLanguage string  `json:"original_language"`
	Overview         string  `json:"overview"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	ReleaseDate      string  `json:"release_date,omitempty"`   // Movies
	FirstAirDate     string  `json:"first_air_date,omitempty"` // TV
	GenreIDs         []int   `json:"genre_ids"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
	Popularity       float64 `json:"popularity"`
}

// DisplayTitle returns the movie title or show name.
func (i ListItem) DisplayTitle() string {
	if i.Title != "" {
		return i.Title
	}
	return i.Name
}

// Year returns the release or first air year, 0 when unknown.
func (i ListItem) Year() int {
	if i.ReleaseDate != "" {
		return yearOf(i.ReleaseDate)
	}
	return yearOf(i.FirstAirDate)
}

// ResultsPage is one page of a TMDB list.
type ResultsPage struct {
	Page         int        `json:"page"`
	Results      []ListItem `json:"results"`
	TotalPages   int        `json:"total_pages"`
	TotalResults int        `json:"total_results"`
}

// Company is a network or production company.
type Company struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path"`
	OriginCountry string `json:"origin_country"`
}

// Creator is someone credited with creating a show.
type Creator struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
}

// MovieDetails is GET /movie/{id} with external_ids, keywords, credits,
// videos, recommendations and similar appended.
type MovieDetails struct {
	ID               int      `json:"id"`
	IMDbID           string   `json:"imdb_id"`
	Title            string   `json:"title"`
	OriginalTitle    string   `json:"original_title"`
	OriginalLanguage string   `json:"original_language"`
	OriginCountry    []string `json:"origin_country"`
	Overview         string   `json:"overview"`
	Tagline          string   `json:"tagline"`
	Status           string   `json:"status"`
	ReleaseDate      string   `json:"release_date"`
	Runtime          int      `json:"runtime"`
	PosterPath       string   `json:"poster_path"`
	BackdropPath     string   `json:"backdrop_path"`
	Genres           []Genre  `json:"genres"`
	VoteAverage      float64  `json:"vote_average"`
	VoteCount        int      `json:"vote_count"`
	Popularity       float64  `json:"popularity"`

	ExternalIDs     ExternalIDs `json:"external_ids"`
	Keywords        Keywords    `json:"keywords"`
	Credits         Credits     `json:"credits"`
	Videos          Videos      `json:"videos"`
	Recommendations ResultsPage `json:"recommendations"`
	Similar         ResultsPage `json:"similar"`
}

// Year extracts the year from the release date
func (m *MovieDetails) Year() int { return yearOf(m.ReleaseDate) }

// SeasonSummary is a season as listed in the show details.
type SeasonSummary struct {
	ID           int     `json:"id"`
	SeasonNumber int     `json:"season_number"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	AirDate      string  `json:"air_date"`
	EpisodeCount int     `json:"episode_count"`
	VoteAverage  float64 `json:"vote_average"`
}

// EpisodeRef is the last or next episode to air, from the show details.
type EpisodeRef struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	AirDate       string `json:"air_date"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	EpisodeType   string `json:"episode_type"`
}

// ShowDetails is GET /tv/{id} with external_ids, keywords, credits,
// aggregate_credits, videos, recommendations and similar appended.
type ShowDetails struct {
	ID               int             `json:"id"`
	Name             string          `json:"name"`
	OriginalName     string          `json:"original_name"`
	OriginalLanguage string          `json:"original_language"`
	OriginCountry    []string        `json:"origin_country"`
	Overview         string          `json:"overview"`
	Tagline          string          `json:"tagline"`
	Status           string          `json:"status"` // "Returning Series", "Ended", "Canceled", ...
	Type             string          `json:"type"`   // "Scripted", "Documentary", ...
	InProduction     bool            `json:"in_production"`
	FirstAirDate     string          `json:"first_air_date"`
	LastAirDate      string          `json:"last_air_date"`
	PosterPath       string          `json:"poster_path"`
	BackdropPath     string          `json:"backdrop_path"`
	Genres           []Genre         `json:"genres"`
	Networks         []Company       `json:"networks"`
	CreatedBy        []Creator       `json:"created_by"`
	EpisodeRunTime   []int           `json:"episode_run_time"`
	NumberOfSeasons  int             `json:"number_of_seasons"`
	NumberOfEpisodes int             `json:"number_of_episodes"`
	VoteAverage      float64         `json:"vote_average"`
	VoteCount        int             `json:"vote_count"`
	Popularity       float64         `json:"popularity"`
	Seasons          []SeasonSummary `json:"seasons"`
	LastEpisodeToAir *EpisodeRef     `json:"last_episode_to_air"`
	NextEpisodeToAir *EpisodeRef     `json:"next_episode_to_air"`

	ExternalIDs      ExternalIDs      `json:"external_ids"`
	Keywords         Keywords         `json:"keywords"`
	Credits          Credits          `json:"credits"`
	AggregateCredits AggregateCredits `json:"aggregate_credits"`
	Videos           Videos           `json:"videos"`
	Recommendations  ResultsPage      `json:"recommendations"`
	Similar          ResultsPage      `json:"similar"`
}

// Year extracts the year from the first air date
func (s *ShowDetails) Year() int { return yearOf(s.FirstAirDate) }

// SeasonNumbers returns the show's season numbers in TMDB's order, season 0
// (specials) included when TMDB lists it.
func (s *ShowDetails) SeasonNumbers() []int {
	numbers := make([]int, 0, len(s.Seasons))
	for _, season := range s.Seasons {
		numbers = append(numbers, season.SeasonNumber)
	}
	return numbers
}

// Season is GET /tv/{id}/season/{n}. Season 0 holds specials.
type Season struct {
	ID           int       `json:"id"`
	SeasonNumber int       `json:"season_number"`
	Name         string    `json:"name"`
	Overview     string    `json:"overview"`
	PosterPath   string    `json:"poster_path"`
	AirDate      string    `json:"air_date"`
	Episodes     []Episode `json:"episodes"`
}

// Episode is one episode of a season. Per-episode crew and guest stars are
// not decoded.
type Episode struct {
	ID            int     `json:"id"`
	SeasonNumber  int     `json:"season_number"`
	EpisodeNumber int     `json:"episode_number"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	AirDate       string  `json:"air_date"`
	EpisodeType   string  `json:"episode_type"` // "standard", "finale", "mid_season"
	Runtime       int     `json:"runtime"`
	StillPath     string  `json:"still_path"`
	VoteAverage   float64 `json:"vote_average"`
}

// Episode group types, as TMDB numbers them.
const (
	GroupOriginalAirDate = 1
	GroupAbsolute        = 2
	GroupDVD             = 3
	GroupDigital         = 4
	GroupStoryArc        = 5
	GroupProduction      = 6
	GroupTV              = 7
)

// AlternativeTitle is one entry of GET /tv/{id}/alternative_titles: the
// show's name in another country or script ("Sousou no Frieren", type
// "Romaji").
type AlternativeTitle struct {
	Country string `json:"iso_3166_1"`
	Title   string `json:"title"`
	Type    string `json:"type"`
}

// EpisodeGroupSummary is one entry of GET /tv/{id}/episode_groups.
type EpisodeGroupSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Type         int      `json:"type"`
	GroupCount   int      `json:"group_count"`
	EpisodeCount int      `json:"episode_count"`
	Network      *Company `json:"network"`
}

// EpisodeGroup is GET /tv/episode_group/{id}: an alternative ordering of a
// show's episodes, split into groups (seasons, arcs, ...).
type EpisodeGroup struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Type         int                 `json:"type"`
	GroupCount   int                 `json:"group_count"`
	EpisodeCount int                 `json:"episode_count"`
	Network      *Company            `json:"network"`
	Groups       []EpisodeGroupGroup `json:"groups"`
}

// EpisodeGroupGroup is one group (season, arc...) of an episode group.
type EpisodeGroupGroup struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Order    int            `json:"order"`
	Locked   bool           `json:"locked"`
	Episodes []GroupEpisode `json:"episodes"`
}

// GroupEpisode is an episode inside an episode group. SeasonNumber and
// EpisodeNumber are its aired numbering; Order its 0-based position in the
// group.
type GroupEpisode struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Order         int    `json:"order"`
	AirDate       string `json:"air_date"`
}

// yearOf parses the year of a YYYY-MM-DD date; 0 when it can't.
func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return year
}
