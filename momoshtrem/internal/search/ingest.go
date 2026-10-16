package search

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/release"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// maxSpan is the longest episode run a release may claim; a longer one
// ("1-999999") is a misreading, so the release covers nothing.
const maxSpan = 10000

// item is one release in a search's results: what the read path needs,
// without the Torznab link or attributes.
type item struct {
	id       string
	title    string // the raw release name
	infoHash string // lower-case hex; "" when unknown
	// magnet is the indexer's, or one built from the info hash; "" until
	// resolve fetches the .torrent.
	magnet    string
	size      int64
	seeders   int
	leechers  int
	published time.Time // zero when unknown
	indexer   string    // the indexer's name
	rel       release.Release

	// TV only. targets are the TMDB (catalog) episodes the release covers,
	// in season and episode order; seasons counts the regular seasons
	// among them.
	targets         []identify.Target
	seasons         int
	coversRequested bool
	sceneNote       string
}

// ingester turns a search's Torznab results into items: parsed, checked
// against the title, mapped to TMDB episodes and kept when in scope. It's
// used by one search at a time (fan-out reports are serialised).
type ingester struct {
	tc        *service.TitleContext
	plan      *QueryPlan
	tv        bool
	numbering *identify.Numbering // TV only
	titles    map[string]bool     // titleKey of every known title
	countries map[string]bool     // lower-case country tags ("us")
	today     string              // YYYY-MM-DD
	// showSeasons counts the catalog's regular seasons.
	showSeasons int
	// seasonAired are the requested season's aired episodes (scope
	// season): a release has the whole season when it has all of them.
	seasonAired []int
}

func newIngester(tc *service.TitleContext, plan *QueryPlan, now time.Time) *ingester {
	in := &ingester{
		tc:        tc,
		plan:      plan,
		tv:        tc.MediaType == library.JobTV,
		titles:    knownTitles(tc),
		countries: map[string]bool{"us": true, "uk": true},
		today:     now.Format(time.DateOnly),
	}
	if !in.tv || tc.Catalog == nil {
		return in
	}
	in.numbering = identify.NewNumbering(tc.Catalog)
	for _, c := range tc.Catalog.Countries {
		in.countries[strings.ToLower(c)] = true
	}
	seasons := map[int]bool{}
	var all, aired []int
	for _, ep := range tc.Catalog.Episodes {
		if ep.Season > 0 {
			seasons[ep.Season] = true
		}
		if plan.Scope == ScopeSeason && ep.Season == *plan.Season {
			all = append(all, ep.Number)
			if in.aired(ep) {
				aired = append(aired, ep.Number)
			}
		}
	}
	in.showSeasons = len(seasons)
	in.seasonAired = aired
	if len(aired) == 0 {
		in.seasonAired = all // no air dates: every episode counts
	}
	return in
}

func (in *ingester) aired(ep identify.Episode) bool {
	return ep.AirDate != "" && ep.AirDate <= in.today
}

// ingest reads one result of query q. ok is false when it's dropped: a
// text-query result naming another title (unless the user wrote the
// query), a movie result with a season or episode or from another year,
// or a TV result outside the scope.
func (in *ingester) ingest(r torznab.Result, q Query) (*item, bool) {
	rel := release.Parse(r.Title)
	if !q.IsID() && in.plan.Q == "" {
		if !in.titleMatches(rel) {
			return nil, false
		}
		if !in.tv && rel.Year != 0 && in.tc.Year != 0 && abs(rel.Year-in.tc.Year) > 1 {
			return nil, false
		}
	}
	if !in.tv && rel.Pack != nil {
		return nil, false
	}

	it := &item{
		id:        releaseID(r),
		title:     r.Title,
		infoHash:  r.InfoHash,
		magnet:    magnetOf(r),
		size:      r.Size,
		seeders:   r.Seeders,
		leechers:  r.Leechers,
		published: r.PublishedAt,
		indexer:   r.IndexerName,
		rel:       rel,
	}
	if it.indexer == "" {
		it.indexer = r.IndexerID
	}
	if !in.tv {
		return it, true
	}

	it.targets = in.cover(rel.Pack)
	if !in.inScope(rel.Pack, it.targets) {
		return nil, false
	}
	seasons := map[int]bool{}
	for _, t := range it.targets {
		if t.Season > 0 {
			seasons[t.Season] = true
		}
	}
	it.seasons = len(seasons)
	switch in.plan.Scope {
	case ScopeEpisode:
		it.coversRequested = true // in scope: it has the episode
	case ScopeSeason:
		it.coversRequested = coversAll(it.targets, *in.plan.Season, in.seasonAired)
	}
	it.sceneNote = sceneNote(rel.Pack, it.targets)
	return it, true
}

// inScope applies the scope: episode keeps releases with the episode,
// packs included; season keeps any release that isn't a single episode
// and has an episode of the season; any keeps everything.
func (in *ingester) inScope(p *release.Pack, targets []identify.Target) bool {
	switch in.plan.Scope {
	case ScopeEpisode:
		return slices.ContainsFunc(targets, func(t identify.Target) bool {
			return t.Season == *in.plan.Season && t.Episode == *in.plan.Episode
		})
	case ScopeSeason:
		return p != nil && p.Type != release.PackEpisode && slices.ContainsFunc(targets, func(t identify.Target) bool {
			return t.Season == *in.plan.Season
		})
	}
	return true
}

// cover maps a release's pack to TMDB episodes with the classifier's
// numbering rules (identify.Numbering.Cover): Frieren "S02" is TMDB
// S01E29–E38. A complete pack without seasons is every aired regular
// episode, and a dated one the episodes that aired that day.
func (in *ingester) cover(p *release.Pack) []identify.Target {
	if p == nil || in.numbering == nil || p.Type == release.PackUnknown {
		return nil
	}
	switch {
	case p.Type == release.PackComplete && len(p.Seasons) == 0:
		return in.episodes(func(ep identify.Episode) bool { return ep.Season > 0 && in.aired(ep) })
	case p.Date != "":
		return in.episodes(func(ep identify.Episode) bool { return ep.AirDate == p.Date })
	}
	span := identify.Span{Seasons: p.Seasons}
	if e := p.Episodes; e != nil {
		if e.To < e.From || e.To-e.From > maxSpan {
			return nil
		}
		r := identify.Range{From: e.From, To: e.To}
		if p.Absolute {
			span.Absolute = &r
		} else {
			span.Episodes = &r
		}
	}
	if len(span.Seasons) == 0 && span.Episodes == nil && span.Absolute == nil {
		return nil
	}
	return in.numbering.Cover(span)
}

// episodes are the catalog's episodes that keep holds for, as targets in
// season and episode order.
func (in *ingester) episodes(keep func(identify.Episode) bool) []identify.Target {
	var out []identify.Target
	for _, ep := range in.tc.Catalog.Episodes {
		if keep(ep) {
			out = append(out, identify.Target{EpisodeID: ep.ID, Season: ep.Season, Episode: ep.Number})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Season != out[j].Season {
			return out[i].Season < out[j].Season
		}
		return out[i].Episode < out[j].Episode
	})
	return out
}

// coversAll reports whether targets have every one of season's episodes.
func coversAll(targets []identify.Target, season int, episodes []int) bool {
	if len(episodes) == 0 {
		return false
	}
	have := map[int]bool{}
	for _, t := range targets {
		if t.Season == season {
			have[t.Episode] = true
		}
	}
	for _, e := range episodes {
		if !have[e] {
			return false
		}
	}
	return true
}

// sceneNote explains a release whose seasons and episodes aren't TMDB's:
// `Scene "Season 2" = TMDB S01E29–E38`. "" when the release numbers its
// episodes as TMDB does, or by absolute number.
func sceneNote(p *release.Pack, targets []identify.Target) string {
	if p == nil || p.Absolute || len(p.Seasons) == 0 || len(targets) == 0 {
		return ""
	}
	literal := true
	for _, t := range targets {
		inRun := p.Episodes == nil || len(p.Seasons) != 1 || t.Episode >= p.Episodes.From && t.Episode <= p.Episodes.To
		if !slices.Contains(p.Seasons, t.Season) || !inRun {
			literal = false
			break
		}
	}
	if literal {
		return ""
	}
	return `Scene "` + p.Label + `" = TMDB ` + spanText(targets)
}

// spanText writes targets as "S01E29", "S01E29–E38" or "S01E29–S02E05".
func spanText(targets []identify.Target) string {
	first, last := targets[0], targets[len(targets)-1]
	s := fmt.Sprintf("S%02dE%02d", first.Season, first.Episode)
	switch {
	case len(targets) == 1:
		return s
	case first.Season == last.Season:
		return s + fmt.Sprintf("–E%02d", last.Episode)
	}
	return s + fmt.Sprintf("–S%02dE%02d", last.Season, last.Episode)
}

// knownTitles are the keys of the title's names: its title, original title,
// romaji and aliases and, for anime, their short forms ("Frieren"), which
// fansub releases use.
func knownTitles(tc *service.TitleContext) map[string]bool {
	names := append([]string{tc.Title, tc.OriginalTitle, tc.Romaji}, tc.Aliases...)
	keys := map[string]bool{}
	for _, n := range names {
		if k := titleKey(n); k != "" {
			keys[k] = true
		}
		if tc.IsAnime {
			if k := titleKey(shortTitle(n)); k != "" {
				keys[k] = true
			}
		}
	}
	return keys
}

// titleMatches reports whether the release names one of the title's
// names, maybe with a country tag ("The Office US"). Each part of a
// slash-separated name counts ("Фрирен / Sousou no Frieren / …").
func (in *ingester) titleMatches(rel release.Release) bool {
	for _, part := range strings.Split(rel.Title, "/") {
		k := titleKey(part)
		if k == "" {
			continue
		}
		if in.titles[k] {
			return true
		}
		if f := strings.Fields(k); len(f) > 1 && in.countries[f[len(f)-1]] && in.titles[strings.Join(f[:len(f)-1], " ")] {
			return true
		}
	}
	return false
}

// titleKey compares titles: the planner's normalisation (queryTitle),
// lower-cased, with hyphens as spaces ("Spider-Man" is "Spider.Man") and
// without a leading "the".
func titleKey(s string) string {
	f := strings.Fields(strings.ReplaceAll(strings.ToLower(queryTitle(s)), "-", " "))
	if len(f) > 1 && f[0] == "the" {
		f = f[1:]
	}
	return strings.Join(f, " ")
}

// normName is a name's letters and digits, lower-cased, one space between
// words.
func normName(s string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(s) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		gap = false
	}
	return b.String()
}

// releaseID is a result's stable id: a hash of its indexer and GUID (else
// its link, else its title and size), the same in every search.
func releaseID(r torznab.Result) string {
	src := r.GUID
	if src == "" {
		src = r.Link
	}
	if src == "" {
		src = r.Title + "\x00" + strconv.FormatInt(r.Size, 10)
	}
	sum := sha256.Sum256([]byte(r.IndexerID + "\x00" + src))
	return hex.EncodeToString(sum[:10])
}

// magnetOf is the indexer's magnet or, with only an info hash, one built
// from it. "" when the .torrent has to be fetched.
func magnetOf(r torznab.Result) string {
	switch {
	case r.MagnetURI != "":
		return r.MagnetURI
	case r.InfoHash != "":
		return "magnet:?xt=urn:btih:" + r.InfoHash + "&dn=" + url.QueryEscape(r.Title)
	}
	return ""
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
