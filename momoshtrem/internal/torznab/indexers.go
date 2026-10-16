package torznab

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Mode is a Torznab search function.
type Mode string

const (
	ModeSearch Mode = "search"   // free text; every indexer has it
	ModeTV     Mode = "tvsearch" // q, season, ep and TV ids
	ModeMovie  Mode = "movie"    // q and movie ids
)

// Indexer is one configured Jackett indexer.
type Indexer struct {
	ID    string // e.g. "thepiratebay"; the path segment in its Torznab URL
	Title string // e.g. "The Pirate Bay"
	Type  string // public, semi-private or private
	Caps  Caps
}

// Caps is what an indexer can search for.
type Caps struct {
	Search      SearchMode
	TVSearch    SearchMode
	MovieSearch SearchMode
	// Categories lists every category id it declares: top-level (2000),
	// subcategories (2040) and its own (100000 and up).
	Categories []int
	// LimitMax is the most results one request returns; 0 when unknown.
	LimitMax int
}

// SearchMode is one search function's availability and parameters.
type SearchMode struct {
	Available bool
	Params    []string // supportedParams, lower-cased: q, season, ep, imdbid, tvdbid, tmdbid…
}

// Supports reports whether the function is available and takes param.
func (m SearchMode) Supports(param string) bool {
	return m.Available && slices.Contains(m.Params, param)
}

// Mode returns the caps of one search function.
func (c Caps) Mode(m Mode) SearchMode {
	switch m {
	case ModeTV:
		return c.TVSearch
	case ModeMovie:
		return c.MovieSearch
	default:
		return c.Search
	}
}

// Supports reports whether search function m takes param, e.g.
// Supports(ModeMovie, "imdbid").
func (c Caps) Supports(m Mode, param string) bool {
	return c.Mode(m).Supports(param)
}

// HasCategory reports whether the indexer declares category id.
func (c Caps) HasCategory(id int) bool {
	return slices.Contains(c.Categories, id)
}

// Indexers returns Jackett's configured indexers, reusing the list for
// IndexersTTL. The slice is shared: don't modify it.
func (c *Client) Indexers(ctx context.Context) ([]Indexer, error) {
	c.mu.Lock()
	list, fresh := c.indexers, c.indexers != nil && c.now().Sub(c.fetchedAt) < IndexersTTL
	c.mu.Unlock()
	if fresh {
		return list, nil
	}
	return c.FetchIndexers(ctx)
}

// FetchIndexers asks Jackett for its configured indexers
// (t=indexers&configured=true) and refreshes the list Indexers reuses. One
// request also checks the API key, which makes it the status probe.
// Concurrent callers share one request; each stops waiting when its own ctx
// ends.
func (c *Client) FetchIndexers(ctx context.Context) ([]Indexer, error) {
	ch := c.group.DoChan("indexers", func() (any, error) {
		// Detached: other callers may be waiting on this request.
		body, err := c.get(context.WithoutCancel(ctx), apiPath("all"),
			url.Values{"t": {"indexers"}, "configured": {"true"}}, IndexersTimeout)
		var list []Indexer
		if err == nil {
			list, err = parseIndexers(body)
		}
		if err != nil {
			c.log.Warn("Jackett indexer list failed", "component", "torznab", "error", c.SafeError(err))
			return nil, err
		}
		c.mu.Lock()
		c.indexers, c.fetchedAt = list, c.now()
		c.mu.Unlock()
		return list, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.([]Indexer), nil
	}
}

// indexer returns the configured indexer with id.
func (c *Client) indexer(ctx context.Context, id string) (Indexer, error) {
	list, err := c.Indexers(ctx)
	if err != nil {
		return Indexer{}, err
	}
	for _, ix := range list {
		if ix.ID == id {
			return ix, nil
		}
	}
	return Indexer{}, ErrUnknownIndexer
}

// indexersDoc is the t=indexers answer.
type indexersDoc struct {
	XMLName  xml.Name `xml:"indexers"`
	Indexers []struct {
		ID         string `xml:"id,attr"`
		Configured string `xml:"configured,attr"`
		Title      string `xml:"title"`
		Type       string `xml:"type"`
		Caps       struct {
			Limits struct {
				Max string `xml:"max,attr"`
			} `xml:"limits"`
			Searching struct {
				Search      searchModeDoc `xml:"search"`
				TVSearch    searchModeDoc `xml:"tv-search"`
				MovieSearch searchModeDoc `xml:"movie-search"`
			} `xml:"searching"`
			Categories []struct {
				ID      string `xml:"id,attr"`
				Subcats []struct {
					ID string `xml:"id,attr"`
				} `xml:"subcat"`
			} `xml:"categories>category"`
		} `xml:"caps"`
	} `xml:"indexer"`
}

type searchModeDoc struct {
	Available       string `xml:"available,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

func (d searchModeDoc) mode() SearchMode {
	m := SearchMode{Available: d.Available == "yes"}
	for _, p := range strings.Split(d.SupportedParams, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			m.Params = append(m.Params, p)
		}
	}
	return m
}

func parseIndexers(body []byte) ([]Indexer, error) {
	var doc indexersDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("jackett indexer list is not Torznab XML: %s", xmlProblem(err))
	}
	list := make([]Indexer, 0, len(doc.Indexers))
	for _, d := range doc.Indexers {
		if d.ID == "" || d.Configured == "false" {
			continue
		}
		ix := Indexer{ID: d.ID, Title: strings.TrimSpace(d.Title), Type: strings.TrimSpace(d.Type)}
		if ix.Title == "" {
			ix.Title = d.ID
		}
		ix.Caps.Search = d.Caps.Searching.Search.mode()
		ix.Caps.TVSearch = d.Caps.Searching.TVSearch.mode()
		ix.Caps.MovieSearch = d.Caps.Searching.MovieSearch.mode()
		ix.Caps.LimitMax, _ = strconv.Atoi(d.Caps.Limits.Max)
		for _, cat := range d.Caps.Categories {
			if id, err := strconv.Atoi(cat.ID); err == nil {
				ix.Caps.Categories = append(ix.Caps.Categories, id)
			}
			for _, sub := range cat.Subcats {
				if id, err := strconv.Atoi(sub.ID); err == nil {
					ix.Caps.Categories = append(ix.Caps.Categories, id)
				}
			}
		}
		list = append(list, ix)
	}
	return list, nil
}
