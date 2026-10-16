package torznab

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Torznab categories this client asks for. Nothing else is ever requested:
// in particular not the 6000 range (XXX).
const (
	CategoryMovies = 2000
	CategoryTV     = 5000
	CategoryAnime  = 5070
)

// allowedCategories are the only categories a request may name.
var allowedCategories = []int{CategoryMovies, CategoryTV, CategoryAnime}

// Query is one search on one indexer. Parameters the indexer's caps don't
// support are dropped rather than failing the request.
type Query struct {
	Mode Mode   // ModeSearch when empty
	Q    string // free text
	// IDs, sent only to indexers whose caps take them in this mode.
	IMDbID string // "tt0816692" (the "tt" is added when missing)
	TVDBID int
	TMDBID int
	// Season and Episode as Torznab sends them (ModeTV only): "1" and "5";
	// "0" for specials; "2026" and "10/16" for a daily show. Empty: none.
	Season  string
	Episode string
	// Categories narrows the request to some of CategoryMovies, CategoryTV
	// and CategoryAnime; others are ignored. Empty means the mode's
	// default: movies for ModeMovie, TV and anime for ModeTV, all three
	// for ModeSearch. Only categories the indexer declares are sent.
	Categories []int
	Limit      int // 0: the indexer's default; capped at its maximum
	Offset     int
}

// Result is one release an indexer returned.
type Result struct {
	Title string // HTML entities decoded
	GUID  string
	// Link is Jackett's download URL. It carries the API key: keep it on
	// the server, never log it or put it in a response. Download fetches it.
	Link        string `json:"-"`
	MagnetURI   string // "" when the indexer gave none; Download resolves Link
	InfoHash    string // lower-case hex; "" when unknown
	Size        int64
	Seeders     int
	Leechers    int       // peers − seeders, never negative
	PublishedAt time.Time // zero when unknown
	Categories  []int
	IndexerID   string
	IndexerName string
}

// Search runs q on one configured indexer and records the outcome in its
// health. Results in an adult category (6000–6999) are dropped.
// ErrUnsupported means no request was made.
func (c *Client) Search(ctx context.Context, indexerID string, q Query) ([]Result, error) {
	ix, err := c.indexer(ctx, indexerID)
	if err != nil {
		return nil, err
	}
	params, err := buildParams(ix, q)
	if err != nil {
		return nil, err
	}

	start := c.now()
	body, err := c.get(ctx, apiPath(ix.ID), params, c.requestTimeout)
	var results []Result
	if err == nil {
		results, err = parseResults(body, ix)
	}
	elapsed := c.now().Sub(start)

	switch {
	case errors.Is(err, context.Canceled):
		// The caller gave up; that says nothing about the indexer.
	case err != nil:
		c.health.record(ix.ID, outcomeOf(err), elapsed, c.SafeError(err), c.now())
		c.log.Warn("Torznab search failed", "component", "torznab", "indexer", ix.ID,
			"mode", params.Get("t"), "ms", elapsed.Milliseconds(), "error", c.SafeError(err))
	default:
		c.health.record(ix.ID, StatusOK, elapsed, "", c.now())
		c.log.Debug("Torznab search", "component", "torznab", "indexer", ix.ID,
			"mode", params.Get("t"), "ms", elapsed.Milliseconds(), "results", len(results))
	}
	return results, err
}

// outcomeOf classifies a failed request.
func outcomeOf(err error) Status {
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return StatusTimeout
	}
	return StatusError
}

// buildParams turns q into an indexer's request parameters, keeping only
// what its caps support. A mode it doesn't offer falls back to search; the
// categories still follow the mode asked for.
func buildParams(ix Indexer, q Query) (url.Values, error) {
	asked := q.Mode
	if asked == "" {
		asked = ModeSearch
	}
	mode := asked
	if !ix.Caps.Mode(mode).Available {
		mode = ModeSearch
	}
	caps := ix.Caps.Mode(mode)

	v := url.Values{"t": {string(mode)}}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if id := normalizeIMDb(q.IMDbID); id != "" && caps.Supports("imdbid") {
		v.Set("imdbid", id)
	}
	if q.TVDBID > 0 && caps.Supports("tvdbid") {
		v.Set("tvdbid", strconv.Itoa(q.TVDBID))
	}
	if q.TMDBID > 0 && caps.Supports("tmdbid") {
		v.Set("tmdbid", strconv.Itoa(q.TMDBID))
	}
	// Without text or an id, the request would list the indexer's latest
	// releases.
	if !v.Has("q") && !v.Has("imdbid") && !v.Has("tvdbid") && !v.Has("tmdbid") {
		return nil, ErrUnsupported
	}
	if mode == ModeTV {
		if q.Season != "" && caps.Supports("season") {
			v.Set("season", q.Season)
		}
		if q.Episode != "" && caps.Supports("ep") {
			v.Set("ep", q.Episode)
		}
	}

	if len(ix.Caps.Categories) > 0 {
		var cats []string
		for _, id := range wantedCategories(asked, q.Categories) {
			if ix.Caps.HasCategory(id) {
				cats = append(cats, strconv.Itoa(id))
			}
		}
		if len(cats) == 0 {
			return nil, ErrUnsupported
		}
		v.Set("cat", strings.Join(cats, ","))
	}

	if limit := q.Limit; limit > 0 {
		if ix.Caps.LimitMax > 0 && limit > ix.Caps.LimitMax {
			limit = ix.Caps.LimitMax
		}
		v.Set("limit", strconv.Itoa(limit))
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	return v, nil
}

// wantedCategories is the allowed subset of asked, or mode's default.
func wantedCategories(mode Mode, asked []int) []int {
	var cats []int
	for _, id := range allowedCategories {
		if slices.Contains(asked, id) {
			cats = append(cats, id)
		}
	}
	if len(cats) > 0 {
		return cats
	}
	switch mode {
	case ModeMovie:
		return []int{CategoryMovies}
	case ModeTV:
		return []int{CategoryTV, CategoryAnime}
	default:
		return allowedCategories
	}
}

func normalizeIMDb(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	if !strings.HasPrefix(id, "tt") {
		id = "tt" + id
	}
	return id
}

// feedDoc is a Torznab search answer: an RSS feed.
type feedDoc struct {
	XMLName xml.Name  `xml:"rss"`
	Items   []itemDoc `xml:"channel>item"`
}

type itemDoc struct {
	Title      string   `xml:"title"`
	GUID       string   `xml:"guid"`
	Link       string   `xml:"link"`
	PubDate    string   `xml:"pubDate"`
	Size       string   `xml:"size"`
	Categories []string `xml:"category"`
	Enclosure  struct {
		URL    string `xml:"url,attr"`
		Length string `xml:"length,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"attr"` // torznab:attr
}

func parseResults(body []byte, ix Indexer) ([]Result, error) {
	var doc feedDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("jackett answer is not a Torznab feed: %s", xmlProblem(err))
	}
	results := make([]Result, 0, len(doc.Items))
	for _, it := range doc.Items {
		if r, ok := parseItem(it, ix); ok {
			results = append(results, r)
		}
	}
	return results, nil
}

// parseItem reads one item; ok is false for an item without a title or in
// an adult category.
func parseItem(it itemDoc, ix Indexer) (Result, bool) {
	r := Result{
		Title:       strings.TrimSpace(html.UnescapeString(strings.TrimSpace(it.Title))),
		GUID:        strings.TrimSpace(it.GUID),
		Link:        strings.TrimSpace(it.Link),
		IndexerID:   ix.ID,
		IndexerName: ix.Title,
	}
	if r.Title == "" {
		return Result{}, false
	}

	attrs := map[string]string{}
	addCategory := func(s string) {
		if id, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && !slices.Contains(r.Categories, id) {
			r.Categories = append(r.Categories, id)
		}
	}
	for _, s := range it.Categories {
		addCategory(s)
	}
	for _, a := range it.Attrs {
		name := strings.ToLower(a.Name)
		if name == "category" {
			addCategory(a.Value)
			continue
		}
		if _, seen := attrs[name]; !seen {
			attrs[name] = strings.TrimSpace(a.Value)
		}
	}
	for _, id := range r.Categories {
		if id >= 6000 && id <= 6999 {
			return Result{}, false
		}
	}

	r.MagnetURI = firstMagnet(attrs["magneturl"], r.Link, it.Enclosure.URL, r.GUID)
	r.InfoHash = normalizeInfoHash(attrs["infohash"])
	if r.InfoHash == "" {
		r.InfoHash = magnetInfoHash(r.MagnetURI)
	}
	r.Size = firstInt64(it.Size, attrs["size"], it.Enclosure.Length)
	r.Seeders = atoiNonNegative(attrs["seeders"])
	if v, ok := attrs["leechers"]; ok {
		r.Leechers = atoiNonNegative(v)
	} else {
		r.Leechers = max(atoiNonNegative(attrs["peers"])-r.Seeders, 0)
	}
	r.PublishedAt = parsePubDate(it.PubDate)
	return r, true
}

func firstMagnet(candidates ...string) string {
	for _, s := range candidates {
		if isMagnet(s) {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// magnetInfoHash reads the BitTorrent info hash from a magnet's xt.
func magnetInfoHash(magnet string) string {
	if magnet == "" {
		return ""
	}
	u, err := url.Parse(magnet)
	if err != nil {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		if rest, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:"); ok {
			return normalizeInfoHash(rest)
		}
	}
	return ""
}

// normalizeInfoHash returns a v1 info hash as lower-case hex, from hex or
// base32; "" when s is neither.
func normalizeInfoHash(s string) string {
	s = strings.TrimSpace(s)
	switch len(s) {
	case 40:
		if _, err := hex.DecodeString(s); err == nil {
			return strings.ToLower(s)
		}
	case 32:
		if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(s)); err == nil {
			return hex.EncodeToString(b)
		}
	}
	return ""
}

func firstInt64(candidates ...string) int64 {
	for _, s := range candidates {
		if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func atoiNonNegative(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

var pubDateLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC3339}

func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range pubDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
