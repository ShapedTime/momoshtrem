package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// Assigning HDRezka streams (PLAN §6 "HDRezka (#63)"): synchronous, with no
// job. Everything that needs no network is checked first; then each
// stream is resolved and measured, the items that worked are written at
// once, their subtitles recorded and the tree rebuilt once.

// HDRezkaAssign is POST /movies/{id}/assign-hdrezka.
type HDRezkaAssign struct {
	URL          string `json:"url"`
	TranslatorID int    `json:"translator_id"`
	// The translator's cut, for a movie: each flag given must match;
	// omitted, the id's first entry. A series ignores them.
	Camrip   *bool `json:"camrip,omitempty"`
	Ads      *bool `json:"ads,omitempty"`
	Director *bool `json:"director,omitempty"`
	// Quality is a label as GET /hdrezka/streams lists it, e.g. "1080p";
	// when it isn't offered without Premium, the best one below it is used.
	Quality string `json:"quality"`
}

func (r HDRezkaAssign) cut() TranslatorCut {
	return TranslatorCut{Camrip: r.Camrip, Ads: r.Ads, Director: r.Director}
}

// HDRezkaShowAssign is POST /shows/{id}/assign-hdrezka.
type HDRezkaShowAssign struct {
	HDRezkaAssign
	Items []HDRezkaAssignItem `json:"items"` // 1–30
}

// HDRezkaAssignItem maps a library episode to an HDRezka episode.
type HDRezkaAssignItem struct {
	EpisodeID int64 `json:"episode_id"`
	Season    int   `json:"season"`  // HDRezka's
	Episode   int   `json:"episode"` // HDRezka's
}

// HDRezkaShowResult is a show assign's answer: what was assigned and what
// failed, each in request order.
type HDRezkaShowResult struct {
	Assigned []HDRezkaAssigned `json:"assigned"`
	Failed   []HDRezkaFailed   `json:"failed"`
}

// HDRezkaAssigned is an item that got its stream, in the quality used.
type HDRezkaAssigned struct {
	EpisodeID int64  `json:"episode_id"` // 0 for a movie
	Quality   string `json:"quality"`
	SizeBytes int64  `json:"size_bytes"`
}

// HDRezkaFailed is an item that didn't, and why.
type HDRezkaFailed struct {
	EpisodeID int64  `json:"episode_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// validate checks the request without the network and returns the title
// page's path.
func (r HDRezkaAssign) validate() (string, error) {
	if strings.TrimSpace(r.Quality) == "" || r.TranslatorID <= 0 {
		return "", hdrezkaErr(HDRezkaBadRequest, "Send {url, translator_id, quality}.")
	}
	return titlePath(r.URL)
}

// AssignMovie makes the movie play from an HDRezka stream: the quality
// asked for, else the best one below it that plays without Premium. The
// file's size is measured now; then one write, the stream's subtitles and
// one tree rebuild (see finishAssign).
func (s *HDRezkaService) AssignMovie(ctx context.Context, movieID int64, req HDRezkaAssign) (*HDRezkaAssigned, error) {
	path, err := req.validate()
	if err != nil {
		return nil, err
	}
	if err := s.movieExists(movieID); err != nil {
		return nil, err
	}

	bctx, cancel := context.WithTimeout(ctx, s.assignBudget)
	defer cancel()
	t, tr, err := s.titleAndTranslator(bctx, path, req.TranslatorID, req.cut(), hdrezka.KindMovie)
	if err != nil {
		return nil, err
	}
	key := streamKeyOf(t, tr, 0, 0)
	m, err := s.measure(bctx, key, req.Quality)
	if err != nil {
		return nil, siteFailure(err)
	}
	ref := streamRef(t, tr, key, m.quality)
	if err := s.write(ctx, library.AssignmentWrite{ItemType: library.ItemTypeMovie, MovieID: movieID,
		Rows: []library.TorrentAssignment{{ItemID: movieID, Stream: &ref, FileSize: m.size, Resolution: resolutionLabel(m.quality)}},
	}); err != nil {
		return nil, err
	}
	s.finishAssign(ctx, bctx, []streamSubtitles{
		{item: subtitle.Item{Type: subtitle.ItemTypeMovie, ID: movieID}, ref: ref, tracks: m.subtitles},
	})
	s.log.Info("HDRezka stream assigned", "movie_id", movieID, "post_id", t.PostID, "translator_id", tr.ID,
		"quality", m.quality.Label, "asked", req.Quality, "size", m.size, "subtitles", len(m.subtitles))
	return &HDRezkaAssigned{Quality: m.quality.Label, SizeBytes: m.size}, nil
}

// AssignShow makes HDRezka episodes the show's episodes' files. Everything
// is checked before the network: the items (1–30, no episode twice), that
// each episode is the show's, then with one request that HDRezka has each
// season and episode in the translation. Then each item is resolved and
// measured, four at a time within 25 s; those that worked are written at
// once, their subtitles recorded and the tree rebuilt once (see
// finishAssign), those that didn't are listed in Failed. When nothing
// worked, the first failure is the error.
func (s *HDRezkaService) AssignShow(ctx context.Context, showID int64, req HDRezkaShowAssign) (*HDRezkaShowResult, error) {
	path, err := req.validate()
	if err != nil {
		return nil, err
	}
	switch n := len(req.Items); {
	case n == 0:
		return nil, hdrezkaErr(HDRezkaBadRequest, "Send at least one item.")
	case n > MaxHDRezkaItems:
		return nil, hdrezkaErr(HDRezkaBadRequest, "Send at most %d episodes at a time.", MaxHDRezkaItems)
	}
	seen := map[int64]bool{}
	for _, it := range req.Items {
		if it.EpisodeID <= 0 || it.Season < 0 || it.Episode < 1 {
			return nil, hdrezkaErr(HDRezkaBadRequest, "Each item needs an episode_id, a season and an episode from 1.")
		}
		if seen[it.EpisodeID] {
			return nil, hdrezkaErr(HDRezkaBadRequest, "Episode %d is listed twice.", it.EpisodeID)
		}
		seen[it.EpisodeID] = true
	}
	seasons, err := s.shows.EpisodeList(ctx, showID)
	if err != nil {
		return nil, libraryFailure(err)
	}
	ours := map[int64]bool{}
	for _, season := range seasons {
		for _, e := range season.Episodes {
			ours[e.ID] = true
		}
	}
	for _, it := range req.Items {
		if !ours[it.EpisodeID] {
			return nil, hdrezkaErr(HDRezkaInvalid, "Episode %d isn't this show's.", it.EpisodeID)
		}
	}

	bctx, cancel := context.WithTimeout(ctx, s.assignBudget)
	defer cancel()
	t, tr, err := s.titleAndTranslator(bctx, path, req.TranslatorID, TranslatorCut{}, hdrezka.KindSeries)
	if err != nil {
		return nil, err
	}
	offered, err := s.seasonsOf(bctx, t, tr)
	if err != nil {
		return nil, siteFailure(err)
	}
	has := map[[2]int]bool{}
	for _, season := range offered {
		for _, e := range season.Episodes {
			has[[2]int{season.Number, e}] = true
		}
	}
	var missing []string
	for _, it := range req.Items {
		if !has[[2]int{it.Season, it.Episode}] {
			missing = append(missing, fmt.Sprintf("S%02dE%02d", it.Season, it.Episode))
		}
	}
	if len(missing) > 0 {
		return nil, hdrezkaErr(HDRezkaInvalid, "HDRezka has no %s in this voice-over.", strings.Join(missing, ", "))
	}

	type result struct {
		key hdrezka.StreamKey
		measured
		err error
	}
	results := make([]result, len(req.Items))
	var g errgroup.Group
	g.SetLimit(hdrezkaAssignWorkers)
	for i, it := range req.Items {
		g.Go(func() error {
			key := streamKeyOf(t, tr, it.Season, it.Episode)
			m, err := s.measure(bctx, key, req.Quality)
			results[i] = result{key: key, measured: m, err: err}
			return nil
		})
	}
	g.Wait()

	out := &HDRezkaShowResult{Assigned: []HDRezkaAssigned{}, Failed: []HDRezkaFailed{}}
	var rows []library.TorrentAssignment
	var subs []streamSubtitles
	var first *HDRezkaError
	for i, r := range results {
		id := req.Items[i].EpisodeID
		if r.err != nil {
			f := failureOf(r.err)
			if first == nil {
				first = f
			}
			s.log.Warn("HDRezka episode not assigned", "show_id", showID, "episode_id", id,
				"season", r.key.Season, "episode", r.key.Episode, "error", r.err)
			out.Failed = append(out.Failed, HDRezkaFailed{EpisodeID: id, Code: f.Code, Message: f.Message})
			continue
		}
		ref := streamRef(t, tr, r.key, r.quality)
		rows = append(rows, library.TorrentAssignment{ItemID: id, Stream: &ref, FileSize: r.size, Resolution: resolutionLabel(r.quality)})
		subs = append(subs, streamSubtitles{item: subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id}, ref: ref, tracks: r.subtitles})
		out.Assigned = append(out.Assigned, HDRezkaAssigned{EpisodeID: id, Quality: r.quality.Label, SizeBytes: r.size})
	}
	if len(rows) == 0 {
		if errors.Is(first, context.Canceled) {
			return nil, context.Canceled
		}
		return nil, first
	}
	if err := s.write(ctx, library.AssignmentWrite{ItemType: library.ItemTypeEpisode, ShowID: showID, Rows: rows}); err != nil {
		return nil, err
	}
	s.finishAssign(ctx, bctx, subs)
	s.log.Info("HDRezka streams assigned", "show_id", showID, "post_id", t.PostID, "translator_id", tr.ID,
		"assigned", len(out.Assigned), "failed", len(out.Failed))
	return out, nil
}

// titleAndTranslator fetches the title page, checks it's the kind wanted
// and finds the translator, which mustn't be premium.
func (s *HDRezkaService) titleAndTranslator(ctx context.Context, path string, translatorID int, cut TranslatorCut, want hdrezka.Kind) (*hdrezka.Title, hdrezka.Translator, error) {
	t, err := s.site.Title(ctx, path)
	if err != nil {
		return nil, hdrezka.Translator{}, siteFailure(err)
	}
	if t.Kind != want {
		if want == hdrezka.KindMovie {
			return nil, hdrezka.Translator{}, hdrezkaErr(HDRezkaInvalid, "That HDRezka link is a series; pick the movie's page.")
		}
		return nil, hdrezka.Translator{}, hdrezkaErr(HDRezkaInvalid, "That HDRezka link is a movie; pick the series' page.")
	}
	tr, err := pickTranslator(t, translatorID, cut)
	if err != nil {
		return nil, tr, err
	}
	if tr.Premium {
		return nil, tr, premiumErr()
	}
	return t, tr, nil
}

// measured is a resolved stream: the quality picked, its file's size and
// the stream's subtitle tracks.
type measured struct {
	quality   hdrezka.Quality
	size      int64
	subtitles []hdrezka.Subtitle
}

// measure resolves the stream, picks the quality (the label asked for,
// else the best one below it that plays without Premium) and measures its
// file. A link the CDN refuses is resolved again once.
func (s *HDRezkaService) measure(ctx context.Context, key hdrezka.StreamKey, label string) (measured, error) {
	for refreshed := false; ; refreshed = true {
		stream, err := s.resolver.Resolve(ctx, key)
		if err != nil {
			return measured{}, err
		}
		q, err := pickQuality(stream, label)
		if err != nil {
			return measured{quality: q}, err
		}
		size, err := s.site.ContentLength(ctx, q.MP4)
		if errors.Is(err, hdrezka.ErrLinkExpired) && !refreshed {
			s.resolver.Invalidate(key)
			continue
		}
		if err == nil && size <= 0 {
			err = errors.New("the HDRezka CDN didn't say the file's size")
		}
		return measured{quality: q, size: size, subtitles: stream.Subtitles}, err
	}
}

// pickQuality is the quality with the label, else the highest one at or
// below its resolution that plays without Premium. ErrPremium when only
// premium ones are left, ErrNoQuality when none is.
func pickQuality(stream *hdrezka.Stream, label string) (hdrezka.Quality, error) {
	label = strings.TrimSpace(label)
	if q, ok := stream.Find(label); ok && !q.Premium && q.MP4 != "" {
		return q, nil
	}
	limit := hdrezka.Resolution(label)
	if q, ok := stream.Find(label); ok && q.Resolution > 0 {
		limit = q.Resolution
	}
	var best hdrezka.Quality
	found, premium := false, false
	for _, q := range stream.Qualities {
		if q.Resolution <= 0 || q.Resolution > limit {
			continue
		}
		if q.Premium {
			premium = true
			continue
		}
		if q.MP4 != "" && (!found || q.Resolution > best.Resolution) {
			best, found = q, true
		}
	}
	switch {
	case found:
		return best, nil
	case premium:
		return best, hdrezka.ErrPremium
	}
	return best, hdrezka.ErrNoQuality
}

// write stores the streams. It runs even when the client has gone, since
// the streams are resolved; finishAssign rebuilds the tree.
func (s *HDRezkaService) write(ctx context.Context, w library.AssignmentWrite) error {
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hdrezkaWriteTimeout)
	defer cancel()
	if err := s.assignments.WriteStream(wctx, w); err != nil {
		return libraryFailure(err)
	}
	return nil
}

func (s *HDRezkaService) movieExists(id int64) error {
	movie, err := s.movies.GetByID(id)
	if err != nil {
		return err
	}
	if movie == nil {
		return hdrezkaErr(HDRezkaNotFound, "Movie not found.")
	}
	return nil
}

// streamKeyOf is the resolver's key for a movie (season and episode 0) or
// an episode of t in tr.
func streamKeyOf(t *hdrezka.Title, tr hdrezka.Translator, season, episode int) hdrezka.StreamKey {
	return hdrezka.StreamKey{PostID: t.PostID, Path: t.Path, TranslatorID: tr.ID,
		Camrip: tr.Camrip, Ads: tr.Ads, Director: tr.Director, Season: season, Episode: episode}
}

// streamRef is what the assignment stores: enough to resolve the stream
// again without the title page. Title is the original title when HDRezka
// gives one, else its (Russian) title.
func streamRef(t *hdrezka.Title, tr hdrezka.Translator, key hdrezka.StreamKey, q hdrezka.Quality) library.StreamRef {
	title := t.OriginalTitle
	if title == "" {
		title = t.Title
	}
	return library.StreamRef{
		Provider: string(library.AssignmentKindHDRezka), Path: t.Path, PostID: int64(t.PostID), Title: title,
		TranslatorID: tr.ID, TranslatorName: tr.Name, Camrip: tr.Camrip, Ads: tr.Ads, Director: tr.Director,
		Season: key.Season, Episode: key.Episode, Quality: q.Label,
	}
}

// resolutionLabel is the assignment's resolution: "1080p", "2160p".
func resolutionLabel(q hdrezka.Quality) string {
	if q.Resolution <= 0 {
		return ""
	}
	return fmt.Sprintf("%dp", q.Resolution)
}
