package search

import (
	"cmp"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/quality"
	"github.com/shapedtime/momoshtrem/internal/release"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// Facets, in the response's order.
const (
	facetResolution = iota
	facetSource
	facetCodec
	facetHDR
	facetAudio
	facetPack
	facetIndexer
	facetGroup
	numFacets
)

// allowedValues are each facet's values in the order buckets list them;
// nil for indexer and group, which list by count.
var allowedValues = [numFacets][]string{
	facetResolution: {"2160p", "1080p", "720p", "576p", "480p", release.Unknown},
	facetSource:     {"REMUX", "BluRay", "WEB-DL", "WEBRip", "WEB", "HDTV", "DVD", "CAM", release.Unknown},
	facetCodec:      {"x265", "x264", "AV1", "other", release.Unknown},
	facetHDR:        {"DV", "HDR10+", "HDR10", "HDR", hdrSDR},
	facetAudio:      {"Atmos", "TrueHD", "DTS-HD", "DTS", "DD+", "DD", "AAC", "FLAC", "Opus"},
	facetPack: {release.PackEpisode, release.PackMultiEpisode, release.PackSeason, release.PackMultiSeason,
		release.PackComplete, release.PackUnknown},
}

// hdrSDR is the hdr value of a release whose name gives no HDR format.
const hdrSDR = "SDR"

// maxGroupBuckets caps the group facet (selected groups are always listed).
const maxGroupBuckets = 20

// candidate is a release the profile doesn't hide, with what the read
// needs to filter, count and sort it.
type candidate struct {
	it      item
	ev      quality.Evaluation
	missing int // episodes of the missing run it has
	vals    [numFacets][]string
	// perItem is the size per episode for TV (size over the episodes it
	// covers, or the whole size when that's unknown), the size for movies.
	perItem int64
}

// read builds a response from a search's snapshot: every release scored
// against profile (tc is this request's context, for the files the title
// has now and its missing run), the hidden ones dropped, then filters,
// facets, the best match, sort and paging.
func read(snap snapshot, tc *service.TitleContext, req SearchRequest, profile library.QualityProfile, now time.Time, warnings []string) *SearchResponse {
	tv := snap.mediaType == library.JobTV
	plan := snap.plan
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
		Queries:  nonNil(snap.queries),
		Indexers: nonNil(snap.indexers),
		Complete: snap.complete,
		CachedAt: snap.startedAt,
		Warnings: nonNil(append(snap.warnings, warnings...)),
	}

	missing := missingFor(tc, plan)
	have := filedEpisodes(tc)
	visible := make([]candidate, 0, len(snap.items))
	for _, it := range snap.items {
		c := candidate{it: it, perItem: it.size}
		f := quality.Facts{
			Seeders: it.seeders, SizeBytes: it.size, PublishedAt: it.published, Now: now,
			MediaType: snap.mediaType, RuntimeMinutes: tc.Runtime,
		}
		if tv {
			f.Scope = quality.Scope(plan.Scope)
			f.Season, f.Episode = deref(plan.Season), deref(plan.Episode)
			f.Episodes, f.Seasons, f.ShowSeasons = len(it.targets), it.seasons, snap.showSeasons
			f.CoversRequested = it.coversRequested
			f.SceneNote = it.sceneNote
			for _, t := range it.targets {
				if have[seKey{t.Season, t.Episode}] {
					f.Have++
				}
				if missing != nil && t.Season == missing.SeasonNumber && t.Episode >= missing.From && t.Episode <= missing.To {
					c.missing++
				}
			}
			if missing != nil {
				f.MissingTotal, f.MissingCovered = missing.Count, c.missing
			}
			if n := len(it.targets); n > 0 {
				c.perItem = it.size / int64(n)
			}
		}
		c.ev = quality.Evaluate(it.rel, f, profile)
		if c.ev.Hidden {
			resp.Hidden++
			continue
		}
		c.vals = valuesOf(it, tv)
		visible = append(visible, c)
	}
	resp.TotalUnfiltered = len(visible)

	sel := selectedValues(req.Filters)
	var filtered []candidate
	for _, c := range visible {
		if matches(c, sel, req.Filters, -1) {
			filtered = append(filtered, c)
		}
	}
	resp.Facets = facetsOf(visible, sel, req.Filters, tv)

	evals := make([]quality.Evaluation, len(filtered))
	for i, c := range filtered {
		evals[i] = c.ev
	}
	bestID := ""
	if b := quality.Best(evals); b >= 0 {
		bestID = filtered[b].it.id
		resp.BestID = &bestID
		if missing != nil {
			resp.Missing = &MissingRun{MissingRun: *missing, BestCovers: filtered[b].missing >= missing.Count}
		}
	}
	if missing != nil && resp.Missing == nil {
		resp.Missing = &MissingRun{MissingRun: *missing}
	}

	sortCandidates(filtered, req.Sort, bestID)
	resp.Total = len(filtered)
	from := min((req.Page-1)*req.PageSize, len(filtered))
	to := min(from+req.PageSize, len(filtered))
	resp.Items = make([]Release, 0, to-from)
	for _, c := range filtered[from:to] {
		resp.Items = append(resp.Items, toRelease(c))
	}
	return resp
}

type seKey struct{ season, episode int }

// filedEpisodes are the title's episodes that have a file now (a torrent's
// or an HDRezka stream), by season and episode number: a search's targets
// may come from a catalog whose ids differ, such as before the show was
// added.
func filedEpisodes(tc *service.TitleContext) map[seKey]bool {
	out := map[seKey]bool{}
	if tc.Catalog == nil || len(tc.Files) == 0 {
		return out
	}
	for _, ep := range tc.Catalog.Episodes {
		if tc.Files[ep.ID] != "" {
			out[seKey{ep.Season, ep.Number}] = true
		}
	}
	return out
}

// missingFor is the title's missing run when the request falls in it: the
// run's season, and its episode when one is asked for. Nil for movies,
// titles not in the library, one-episode runs and other seasons.
func missingFor(tc *service.TitleContext, plan *QueryPlan) *catalog.MissingRun {
	m := tc.Missing
	switch {
	case m == nil, tc.MediaType != library.JobTV, m.Count < 2, plan.Season == nil, *plan.Season != m.SeasonNumber:
		return nil
	case plan.Episode != nil && (*plan.Episode < m.From || *plan.Episode > m.To):
		return nil
	}
	run := *m
	return &run
}

// valuesOf is a release's value in each facet: none, one, or several for
// hdr and audio. A release whose name gives no HDR format is SDR.
func valuesOf(it item, tv bool) [numFacets][]string {
	var v [numFacets][]string
	r := it.rel
	v[facetResolution] = []string{orUnknown(r.Resolution)}
	v[facetSource] = []string{orUnknown(r.Source)}
	v[facetCodec] = []string{orUnknown(r.Codec)}
	v[facetHDR] = slices.Compact(slices.Clone(r.HDR))
	if len(v[facetHDR]) == 0 {
		v[facetHDR] = []string{hdrSDR}
	}
	v[facetAudio] = slices.Compact(slices.Clone(r.Audio))
	if tv && r.Pack != nil {
		v[facetPack] = []string{r.Pack.Type}
	}
	v[facetIndexer] = []string{it.indexer}
	if r.Group != "" {
		v[facetGroup] = []string{r.Group}
	}
	return v
}

func orUnknown(s string) string {
	if s == "" {
		return release.Unknown
	}
	return s
}

func selectedValues(f Filters) [numFacets][]string {
	var sel [numFacets][]string
	sel[facetResolution], sel[facetSource], sel[facetCodec] = f.Resolution, f.Source, f.Codec
	sel[facetHDR], sel[facetAudio], sel[facetPack] = f.HDR, f.Audio, f.Pack
	sel[facetIndexer], sel[facetGroup] = f.Indexer, f.Group
	return sel
}

// matches applies the filters, leaving out facet skip (-1: none): a facet
// with selected values needs one of them, and the numeric filters always
// apply.
func matches(c candidate, sel [numFacets][]string, f Filters, skip int) bool {
	for i := range numFacets {
		if i == skip || len(sel[i]) == 0 {
			continue
		}
		if !slices.ContainsFunc(c.vals[i], func(v string) bool { return slices.Contains(sel[i], v) }) {
			return false
		}
	}
	switch {
	case f.MinSeeders != nil && c.it.seeders < *f.MinSeeders:
		return false
	case f.MinSizeGB != nil && float64(c.perItem) < *f.MinSizeGB*1e9:
		return false
	case f.MaxSizeGB != nil && float64(c.perItem) > *f.MaxSizeGB*1e9:
		return false
	}
	return true
}

// facetsOf counts each facet disjunctively: over the releases that pass
// every other filter, so a count is what selecting the value would give.
// Buckets list the values present in the unfiltered scope plus the
// selected ones; pack is empty for movies.
func facetsOf(visible []candidate, sel [numFacets][]string, f Filters, tv bool) Facets {
	var out [numFacets][]FacetBucket
	for i := range numFacets {
		if i == facetPack && !tv {
			out[i] = []FacetBucket{}
			continue
		}
		present := map[string]bool{}
		counts := map[string]int{}
		for _, c := range visible {
			for _, v := range c.vals[i] {
				present[v] = true
			}
			if matches(c, sel, f, i) {
				for _, v := range c.vals[i] {
					counts[v]++
				}
			}
		}
		out[i] = buckets(i, present, counts, sel[i])
	}
	return Facets{
		Resolution: out[facetResolution], Source: out[facetSource], Codec: out[facetCodec],
		HDR: out[facetHDR], Audio: out[facetAudio], Pack: out[facetPack],
		Indexer: out[facetIndexer], Group: out[facetGroup],
	}
}

// buckets orders a facet's values: the allowed values in their order, then
// selected values the facet doesn't know; indexer and group by count (then
// name), group capped at maxGroupBuckets plus any selected.
func buckets(facet int, present map[string]bool, counts map[string]int, selected []string) []FacetBucket {
	for _, v := range selected {
		present[v] = true
	}
	out := []FacetBucket{}
	if allowed := allowedValues[facet]; allowed != nil {
		for _, v := range allowed {
			if present[v] {
				out = append(out, FacetBucket{Value: v, Count: counts[v]})
				delete(present, v)
			}
		}
		rest := make([]string, 0, len(present))
		for v := range present {
			rest = append(rest, v)
		}
		slices.Sort(rest)
		for _, v := range rest {
			out = append(out, FacetBucket{Value: v, Count: counts[v]})
		}
		return out
	}

	for v := range present {
		out = append(out, FacetBucket{Value: v, Count: counts[v]})
	}
	slices.SortFunc(out, func(a, b FacetBucket) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Value, b.Value))
	})
	if facet != facetGroup || len(out) <= maxGroupBuckets {
		return out
	}
	top := out[:maxGroupBuckets:maxGroupBuckets]
	for _, b := range out[maxGroupBuckets:] {
		if slices.Contains(selected, b.Value) {
			top = append(top, b)
		}
	}
	return top
}

// sortCandidates orders releases: best puts the best match first, then
// score and seeders; seeders most first; size smallest first (unknown
// last); newest by published time (unknown last). Ties go to seeders, then
// the id, so pages are stable.
func sortCandidates(cs []candidate, by Sort, bestID string) {
	slices.SortStableFunc(cs, func(a, b candidate) int {
		var c int
		switch by {
		case SortSeeders:
			c = cmp.Or(cmp.Compare(b.it.seeders, a.it.seeders), cmp.Compare(b.ev.Score, a.ev.Score))
		case SortSize:
			c = cmp.Or(boolFirst(a.it.size > 0, b.it.size > 0), cmp.Compare(a.it.size, b.it.size))
		case SortNewest:
			c = cmp.Or(boolFirst(!a.it.published.IsZero(), !b.it.published.IsZero()), b.it.published.Compare(a.it.published))
		default:
			c = cmp.Or(boolFirst(a.it.id == bestID, b.it.id == bestID), cmp.Compare(b.ev.Score, a.ev.Score))
		}
		return cmp.Or(c, cmp.Compare(b.it.seeders, a.it.seeders), cmp.Compare(a.it.id, b.it.id))
	})
}

// boolFirst orders true before false.
func boolFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// toRelease is a candidate as the response shows it. A release without a
// magnet is flagged needs_torrent_fetch.
func toRelease(c candidate) Release {
	it := c.it
	r := Release{
		ID:           it.id,
		RawTitle:     it.title,
		DisplayTitle: it.rel.DisplayTitle,
		SizeBytes:    it.size,
		Seeders:      it.seeders,
		Leechers:     it.leechers,
		Indexer:      it.indexer,
		Attrs:        it.rel,
		Score:        c.ev.Score,
		ScoreReasons: nonNil(c.ev.Reasons),
		Notes:        nonNil(c.ev.Notes),
		Warnings:     nonNil(c.ev.Warnings),
		Flags:        append([]string{}, c.ev.Flags...),
	}
	if r.DisplayTitle == "" {
		r.DisplayTitle = it.title
	}
	if it.infoHash != "" {
		hash := it.infoHash
		r.InfoHash = &hash
	}
	if it.magnet != "" {
		magnet := it.magnet
		r.MagnetURI = &magnet
	} else {
		r.Flags = append(r.Flags, FlagNeedsTorrentFetch)
	}
	if !it.published.IsZero() {
		published := it.published
		r.PublishedAt = &published
	}
	return r
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
