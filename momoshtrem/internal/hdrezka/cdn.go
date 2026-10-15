package hdrezka

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// cdnPath is the endpoint behind the player: streams for a movie or an
// episode, and a translation's episode list.
const cdnPath = "/ajax/get_cdn_series/"

// cdnResponse is get_cdn_series's answer. The site sends false for an
// absent string or map.
type cdnResponse struct {
	Success        bool       `json:"success"`
	Message        string     `json:"message"`
	PremiumContent flexBool   `json:"premium_content"`
	URL            flexString `json:"url"`
	Subtitle       flexString `json:"subtitle"`
	SubtitleLns    flexMap    `json:"subtitle_lns"`
	Seasons        flexString `json:"seasons"`
	Episodes       flexString `json:"episodes"`
}

// Movie fetches a movie's streams in one translation. The translation's
// flags go with the request, as the site's own player sends them.
func (c *Client) Movie(ctx context.Context, ref Ref, t Translator) (*Stream, error) {
	resp, err := c.cdn(ctx, ref, url.Values{
		"id":            {strconv.Itoa(ref.PostID)},
		"translator_id": {strconv.Itoa(t.ID)},
		"is_camrip":     {flag10(t.Camrip)},
		"is_ads":        {flag10(t.Ads)},
		"is_director":   {flag10(t.Director)},
		"action":        {"get_movie"},
	})
	if err != nil {
		return nil, err
	}
	return resp.stream()
}

// Episodes lists the seasons and episodes a translation of a series has.
func (c *Client) Episodes(ctx context.Context, ref Ref, translatorID int) ([]Season, error) {
	resp, err := c.cdn(ctx, ref, url.Values{
		"id":            {strconv.Itoa(ref.PostID)},
		"translator_id": {strconv.Itoa(translatorID)},
		"action":        {"get_episodes"},
	})
	if err != nil {
		return nil, err
	}
	doc, err := parseHTML([]byte(string(resp.Seasons) + string(resp.Episodes)))
	if err != nil {
		return nil, err
	}
	seasons := parseSeasons(doc)
	if len(seasons) == 0 {
		return nil, &ParseError{What: "episode list"}
	}
	return seasons, nil
}

// Episode fetches one episode's streams in one translation.
func (c *Client) Episode(ctx context.Context, ref Ref, translatorID, season, episode int) (*Stream, error) {
	resp, err := c.cdn(ctx, ref, url.Values{
		"id":            {strconv.Itoa(ref.PostID)},
		"translator_id": {strconv.Itoa(translatorID)},
		"season":        {strconv.Itoa(season)},
		"episode":       {strconv.Itoa(episode)},
		"action":        {"get_stream"},
	})
	if err != nil {
		return nil, err
	}
	return resp.stream()
}

// cdn posts form to get_cdn_series the way the player does: an XHR with a
// millisecond timestamp, from the title page.
func (c *Client) cdn(ctx context.Context, ref Ref, form url.Values) (*cdnResponse, error) {
	r := siteRequest{
		path: cdnPath + "?t=" + strconv.FormatInt(c.now().UnixMilli(), 10),
		form: form,
		ajax: true,
	}
	if ref.Path != "" {
		r.referer = c.baseURL + ref.Path
	}
	body, err := c.fetch(ctx, r)
	if err != nil {
		return nil, err
	}
	var resp cdnResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &ParseError{What: "stream response"}
	}
	if !resp.Success {
		return nil, &CDNError{Message: resp.Message}
	}
	return &resp, nil
}

// stream turns a movie or episode answer into a Stream.
func (r *cdnResponse) stream() (*Stream, error) {
	if r.PremiumContent {
		return nil, ErrPremium
	}
	if r.URL == "" {
		return nil, fmt.Errorf("%w: no stream", ErrNotFound)
	}
	plain, legacy, err := decodeStreamURL(string(r.URL))
	if err != nil {
		return nil, err
	}
	s := &Stream{
		Qualities: parseQualities(plain),
		Subtitles: parseSubtitles(string(r.Subtitle), r.SubtitleLns),
		Legacy:    legacy,
	}
	if len(s.Qualities) == 0 {
		return nil, &ParseError{What: "stream qualities"}
	}
	for _, q := range s.Qualities {
		if q.MP4 != "" {
			return s, nil
		}
	}
	return nil, ErrNoMP4
}

func flag10(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// flexString is a JSON string that the site sends as false (or null) when
// there is none.
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	if b = bytes.TrimSpace(b); len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	*s = ""
	return nil
}

// flexBool is a flag the site sends as 0/1 or false/true.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	switch string(bytes.TrimSpace(b)) {
	case "1", "true", `"1"`:
		*f = true
	default:
		*f = false
	}
	return nil
}

// flexMap is a string map the site sends as false when there is none.
type flexMap map[string]string

func (m *flexMap) UnmarshalJSON(b []byte) error {
	if b = bytes.TrimSpace(b); len(b) > 0 && b[0] == '{' {
		var v map[string]string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*m = v
		return nil
	}
	*m = nil
	return nil
}
