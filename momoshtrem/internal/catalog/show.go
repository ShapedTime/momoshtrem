package catalog

import (
	"context"
	"sort"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// ShowTitle builds GET /titles/tv/{tmdb_id}. Queries: the show, its episodes
// (one lean query however many there are), its mapping review, its watch
// state, the TMDB cache, the similar titles' library state and the colour
// cache. Episodes themselves load per season (Season).
//
// TMDB failures for a show in the library fall back to its stored metadata
// (no cast, creators, networks or similar titles); for one that isn't,
// they're returned.
func (s *Service) ShowTitle(ctx context.Context, tmdbID int) (*ShowTitle, error) {
	show, err := s.store.ShowByTMDBID(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	var seasons []library.Season
	var rev *review
	if show != nil {
		if seasons, err = s.store.ShowEpisodes(ctx, show.ID); err != nil {
			return nil, err
		}
		if rev, err = s.reviewOf(ctx, string(library.JobTV), show.ID); err != nil {
			return nil, err
		}
	}
	watch, err := s.showWatchState(ctx, tmdbID)
	if err != nil {
		return nil, err
	}

	details, err := s.tmdb.ShowDetails(ctx, tmdbID)
	if err != nil {
		if show == nil || !fallBackToLibrary(err) {
			return nil, tmdbError(err)
		}
		s.log.Warn("TMDB unavailable; serving the show from the library", "tmdb_id", tmdbID, "error", err)
		details = nil
	}

	var title *ShowTitle
	if details != nil {
		title = showFromTMDB(details)
	} else {
		title = showFromLibrary(show)
	}
	color := s.colorAsync(ctx, deref(title.BackdropPath))

	title.WatchState = watch
	if show != nil {
		s.applyLibraryShow(title, show, seasons, rev)
	} else {
		title.Seasons = tmdbSeasons(details.Seasons)
		title.DefaultSeason = firstSeason(title.Seasons)
	}
	preferNextSeason(title)

	if details != nil {
		if title.Similar, err = s.similar(ctx, "tv", details.Recommendations.Results, details.Similar.Results); err != nil {
			return nil, err
		}
	}
	title.BackdropDominantColor = color()
	return title, nil
}

func showFromTMDB(d *tmdb.ShowDetails) *ShowTitle {
	isAnime := metadata.IsAnime(d.Genres, d.OriginalLanguage, d.OriginCountry, d.Keywords.Keywords)

	cast := make([]CastMember, 0, maxCast)
	for _, c := range d.AggregateCredits.Cast {
		if len(cast) == maxCast {
			break
		}
		cast = append(cast, CastMember{Name: c.Name, Character: c.Character(), ProfilePath: strPtr(c.ProfilePath)})
	}
	if len(cast) == 0 {
		for _, c := range d.Credits.Cast {
			if len(cast) == maxCast {
				break
			}
			cast = append(cast, CastMember{Name: c.Name, Character: c.Character, ProfilePath: strPtr(c.ProfilePath)})
		}
	}
	creators := make([]string, 0, len(d.CreatedBy))
	for _, c := range d.CreatedBy {
		creators = append(creators, c.Name)
	}
	networks := make([]string, 0, len(d.Networks))
	for _, n := range d.Networks {
		networks = append(networks, n.Name)
	}
	var runTime *int
	for _, m := range d.EpisodeRunTime {
		if m > 0 {
			runTime = intPtr(m)
			break
		}
	}
	var endYear *int
	if ended(d.Status) {
		endYear = yearPtr(yearOf(d.LastAirDate))
	}

	return &ShowTitle{
		TitleBase: TitleBase{
			MediaType:         "tv",
			TMDBID:            d.ID,
			Title:             d.Name,
			OriginalTitle:     originalTitle(d.OriginalName, d.Name),
			Year:              yearPtr(d.Year()),
			Overview:          d.Overview,
			PosterPath:        strPtr(d.PosterPath),
			BackdropPath:      strPtr(d.BackdropPath),
			Genres:            genreNames(d.Genres),
			VoteAverage:       voteAverage(d.VoteAverage, d.VoteCount),
			VoteCount:         d.VoteCount,
			OriginalLanguage:  strPtr(d.OriginalLanguage),
			Cast:              cast,
			TrailerYouTubeKey: strPtr(d.Videos.TrailerKey()),
			Similar:           []SimilarTitle{},
			Library:           notInLibrary(),
		},
		Status:          strPtr(d.Status),
		IsAnime:         isAnime,
		EpisodeOrdering: string(metadata.DefaultOrdering(isAnime)),
		IncludeSpecials: true,
		ClassifierMode:  string(library.ClassifierAuto),
		Creators:        creators,
		Networks:        networks,
		EndYear:         endYear,
		EpisodeRunTime:  runTime,
		Seasons:         []SeasonSummary{},
	}
}

// showFromLibrary is the title from stored metadata, when TMDB is down.
func showFromLibrary(sh *library.Show) *ShowTitle {
	return &ShowTitle{
		TitleBase: TitleBase{
			MediaType:        "tv",
			TMDBID:           sh.TMDBID,
			Title:            sh.Title,
			OriginalTitle:    originalTitle(sh.OriginalName, sh.Title),
			Year:             yearPtr(sh.Year),
			PosterPath:       strPtr(sh.PosterPath),
			BackdropPath:     strPtr(sh.BackdropPath),
			Genres:           nonNil(sh.Genres),
			OriginalLanguage: strPtr(sh.OriginalLanguage),
			Cast:             []CastMember{},
			Similar:          []SimilarTitle{},
		},
		Status:   strPtr(sh.Status),
		Creators: []string{},
		Networks: []string{},
		Seasons:  []SeasonSummary{},
	}
}

// applyLibraryShow sets what the library knows: the show's settings, its
// seasons, where it stands and what's under review.
func (s *Service) applyLibraryShow(title *ShowTitle, show *library.Show, seasons []library.Season, rev *review) {
	title.IsAnime = show.IsAnime
	title.EpisodeOrdering = string(show.EpisodeOrdering)
	title.EpisodeGroupID = strPtr(show.EpisodeGroupID)
	title.IncludeSpecials = show.IncludeSpecials
	title.ClassifierMode = string(show.ClassifierMode)
	if title.Status == nil {
		title.Status = strPtr(show.Status)
	}
	if title.EpisodeRunTime == nil {
		title.EpisodeRunTime = medianRuntime(seasons)
	}

	tv := s.torrentView()
	var facts []SeasonFacts
	var download downloadTotals
	anyFile := false // the VFS lists hidden specials too
	for _, season := range seasons {
		visible := season.SeasonNumber > 0 || show.IncludeSpecials
		f := SeasonFacts{Number: season.SeasonNumber, Name: season.Name}
		for i := range season.Episodes {
			ep := &season.Episodes[i]
			anyFile = anyFile || ep.Assignment != nil
			downloading, done, length, seeders := tv.fileProgress(ep.Assignment)
			if downloading && visible {
				download.add(ep.Assignment, done, length, seeders, tv.state(ep.Assignment.InfoHash).speedBps)
			}
			f.Episodes = append(f.Episodes, EpisodeFact{
				Number:      ep.EpisodeNumber,
				Absolute:    ep.AbsoluteNumber,
				AirDate:     ep.AirDate,
				HasFile:     ep.Assignment != nil,
				Downloading: downloading,
				UnderReview: rev.underReview(ep.ID),
				Watched:     ep.Watched,
			})
		}
		if visible {
			facts = append(facts, f)
		}
	}

	sum := SummarizeShow(facts, s.today())
	title.Seasons = sum.Seasons
	title.DefaultSeason = sum.DefaultSeason

	state := inLibrary(show.ID, show.CreatedAt)
	state.AvailabilityStage = ShowStage(true, sum.AnyFile, sum.Frontier, sum.Downloading)
	state.EpisodesTotal = intPtr(sum.EpisodesTotal)
	state.EpisodesReady = intPtr(sum.EpisodesReady)
	state.Missing = sum.Missing
	state.Download = download.progress()
	rev.setOn(&state)
	if anyFile {
		state.WebDAVPath = strPtr(vfs.ShowDir(show.Title, show.Year) + "/")
		state.WebDAVURL = s.webdavURL(state.WebDAVPath)
	}
	title.Library = state
}

// downloadTotals combines the files being fetched for a show.
type downloadTotals struct {
	files   map[library.FileRef]bool
	speeds  map[string]int64 // per torrent: a torrent's speed counts once
	done    int64
	length  int64
	seeders int
}

func (d *downloadTotals) add(a *library.TorrentAssignment, done, length int64, seeders int, speedBps int64) {
	ref := library.FileRef{InfoHash: a.InfoHash, Path: a.FilePath}
	if d.files[ref] {
		return // a multi-episode file counts once
	}
	if d.files == nil {
		d.files = map[library.FileRef]bool{}
		d.speeds = map[string]int64{}
	}
	d.files[ref] = true
	d.speeds[a.InfoHash] = speedBps
	d.done += done
	d.length += length
	d.seeders = max(d.seeders, seeders)
}

func (d *downloadTotals) progress() *DownloadProgress {
	if len(d.files) == 0 {
		return nil
	}
	var speed int64
	for _, s := range d.speeds {
		speed += s
	}
	return &DownloadProgress{Pct: pct(d.done, d.length), SpeedBps: speed, Seeders: d.seeders}
}

// tmdbSeasons lists the seasons TMDB knows, for a show that isn't in the
// library: only the episode counts are known. Specials last.
func tmdbSeasons(list []tmdb.SeasonSummary) []SeasonSummary {
	seasons := make([]SeasonSummary, 0, len(list))
	for _, s := range list {
		seasons = append(seasons, SeasonSummary{
			SeasonNumber:  s.SeasonNumber,
			Name:          SeasonName(s.SeasonNumber, s.Name),
			EpisodesTotal: s.EpisodeCount,
		})
	}
	sort.SliceStable(seasons, func(i, j int) bool { return seasonLess(seasons[i].SeasonNumber, seasons[j].SeasonNumber) })
	return seasons
}

// firstSeason is the first listed season (regular seasons come first), or
// nil without seasons.
func firstSeason(seasons []SeasonSummary) *int {
	if len(seasons) == 0 {
		return nil
	}
	return intPtr(seasons[0].SeasonNumber)
}

// medianRuntime is the median runtime of a show's regular episodes, nil
// when none is known.
func medianRuntime(seasons []library.Season) *int {
	var runtimes []int
	for _, season := range seasons {
		if season.SeasonNumber == 0 {
			continue
		}
		for _, ep := range season.Episodes {
			if ep.Runtime > 0 {
				runtimes = append(runtimes, ep.Runtime)
			}
		}
	}
	if len(runtimes) == 0 {
		return nil
	}
	sort.Ints(runtimes)
	return intPtr(runtimes[len(runtimes)/2])
}

func ended(status string) bool {
	switch status {
	case "Ended", "Canceled", "Cancelled":
		return true
	}
	return false
}

// yearOf parses the year of a YYYY-MM-DD date; 0 when it can't.
func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	year := 0
	for _, c := range date[:4] {
		if c < '0' || c > '9' {
			return 0
		}
		year = year*10 + int(c-'0')
	}
	return year
}
