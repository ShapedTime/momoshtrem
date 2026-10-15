package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache TTLs.
const (
	DetailsTTL       = 7 * 24 * time.Hour  // Movie and show details
	SeasonTTLAiring  = 24 * time.Hour      // Seasons of shows still airing
	SeasonTTLEnded   = 30 * 24 * time.Hour // Seasons of ended or cancelled shows
	EpisodeGroupsTTL = 7 * 24 * time.Hour  // Episode group lists and groups
)

// cacheVersion prefixes every key; bump it when the cached types change
// incompatibly.
const cacheVersion = "tmdb:v1"

// defaultFetchTimeout bounds one upstream fetch, retries included. The fetch
// outlives the request that started it so other waiters still get a result.
const defaultFetchTimeout = 60 * time.Second

// SeasonTTL is how long a season stays fresh: a day while the show airs,
// 30 days once it has ended or been cancelled.
func SeasonTTL(showStatus string) time.Duration {
	switch showStatus {
	case "Ended", "Canceled", "Cancelled":
		return SeasonTTLEnded
	}
	return SeasonTTLAiring
}

// Cache stores JSON payloads by key. A zero expiresAt never expires.
// library.TMDBCacheRepository implements it on Postgres.
type Cache interface {
	Get(ctx context.Context, key string) (payload []byte, expiresAt time.Time, found bool, err error)
	Set(ctx context.Context, key string, payload []byte, expiresAt time.Time) error
}

type bypassKey struct{}

// Bypass returns a context that makes Cached skip fresh cache entries and
// fetch from TMDB (a user-initiated refresh). The result is still cached,
// and errors are returned rather than hidden behind stale data.
func Bypass(ctx context.Context) context.Context {
	return context.WithValue(ctx, bypassKey{}, true)
}

func isBypass(ctx context.Context) bool {
	b, _ := ctx.Value(bypassKey{}).(bool)
	return b
}

// Cached wraps a Client with a persistent cache:
//   - a fresh entry is returned without a request;
//   - concurrent misses for the same key share one request (singleflight);
//   - when TMDB fails and an expired entry exists, the stale entry is
//     returned instead of the error (except for ErrNotFound).
type Cached struct {
	client       *Client
	cache        Cache
	group        singleflight.Group
	now          func() time.Time
	fetchTimeout time.Duration
	log          *slog.Logger
}

// NewCached wraps client with cache.
func NewCached(client *Client, cache Cache) *Cached {
	return &Cached{
		client:       client,
		cache:        cache,
		now:          time.Now,
		fetchTimeout: defaultFetchTimeout,
		log:          slog.With("component", "tmdb-cache"),
	}
}

// Client returns the uncached client.
func (c *Cached) Client() *Client { return c.client }

// Ping checks TMDB directly, never the cache.
func (c *Cached) Ping(ctx context.Context) error { return c.client.Ping(ctx) }

// MovieDetails returns a movie's details, cached for DetailsTTL.
func (c *Cached) MovieDetails(ctx context.Context, id int) (*MovieDetails, error) {
	return cachedGet(ctx, c, c.key(fmt.Sprintf("movie/%d", id)), DetailsTTL,
		func(ctx context.Context) (*MovieDetails, error) { return c.client.MovieDetails(ctx, id) })
}

// ShowDetails returns a show's details, cached for DetailsTTL.
func (c *Cached) ShowDetails(ctx context.Context, id int) (*ShowDetails, error) {
	return cachedGet(ctx, c, c.key(fmt.Sprintf("tv/%d", id)), DetailsTTL,
		func(ctx context.Context) (*ShowDetails, error) { return c.client.ShowDetails(ctx, id) })
}

// Season returns a season (0 for specials), cached for SeasonTTL(showStatus).
// showStatus is the show's TMDB status ("Ended", "Returning Series", ...).
func (c *Cached) Season(ctx context.Context, showID, seasonNumber int, showStatus string) (*Season, error) {
	return cachedGet(ctx, c, c.key(fmt.Sprintf("tv/%d/season/%d", showID, seasonNumber)), SeasonTTL(showStatus),
		func(ctx context.Context) (*Season, error) { return c.client.Season(ctx, showID, seasonNumber) })
}

// episodeGroupList wraps the group list so cachedGet can work with a pointer.
type episodeGroupList struct {
	Results []EpisodeGroupSummary `json:"results"`
}

// EpisodeGroups lists a show's episode groups, cached for EpisodeGroupsTTL.
func (c *Cached) EpisodeGroups(ctx context.Context, showID int) ([]EpisodeGroupSummary, error) {
	list, err := cachedGet(ctx, c, c.key(fmt.Sprintf("tv/%d/episode_groups", showID)), EpisodeGroupsTTL,
		func(ctx context.Context) (*episodeGroupList, error) {
			groups, err := c.client.EpisodeGroups(ctx, showID)
			if err != nil {
				return nil, err
			}
			return &episodeGroupList{Results: groups}, nil
		})
	if err != nil {
		return nil, err
	}
	return list.Results, nil
}

// alternativeTitleList wraps the titles so cachedGet can work with a pointer.
type alternativeTitleList struct {
	Results []AlternativeTitle `json:"results"`
}

// ShowAlternativeTitles returns a show's alternative titles, cached for
// DetailsTTL.
func (c *Cached) ShowAlternativeTitles(ctx context.Context, showID int) ([]AlternativeTitle, error) {
	list, err := cachedGet(ctx, c, c.key(fmt.Sprintf("tv/%d/alternative_titles", showID)), DetailsTTL,
		func(ctx context.Context) (*alternativeTitleList, error) {
			titles, err := c.client.ShowAlternativeTitles(ctx, showID)
			if err != nil {
				return nil, err
			}
			return &alternativeTitleList{Results: titles}, nil
		})
	if err != nil {
		return nil, err
	}
	return list.Results, nil
}

// EpisodeGroup returns one episode group, cached for EpisodeGroupsTTL.
func (c *Cached) EpisodeGroup(ctx context.Context, id string) (*EpisodeGroup, error) {
	return cachedGet(ctx, c, c.key("tv/episode_group/"+id), EpisodeGroupsTTL,
		func(ctx context.Context) (*EpisodeGroup, error) { return c.client.EpisodeGroup(ctx, id) })
}

// key builds a cache key. Responses depend on the language, so it's part of
// the key.
func (c *Cached) key(path string) string {
	lang := c.client.Language()
	if lang == "" {
		lang = "default"
	}
	return cacheVersion + ":" + lang + ":" + path
}

// cachedGet implements the cache policy described on Cached for one key.
func cachedGet[T any](ctx context.Context, c *Cached, key string, ttl time.Duration, fetch func(context.Context) (*T, error)) (*T, error) {
	bypass := isBypass(ctx)

	var stale []byte
	payload, expiresAt, found, err := c.cache.Get(ctx, key)
	switch {
	case err != nil:
		c.log.Warn("TMDB cache read failed; fetching", "key", key, "error", err)
	case found && !bypass && (expiresAt.IsZero() || c.now().Before(expiresAt)):
		v, err := decode[T](payload)
		if err == nil {
			return v, nil
		}
		c.log.Warn("Undecodable TMDB cache entry; fetching", "key", key, "error", err)
	case found && !bypass:
		stale = payload
	}

	ch := c.group.DoChan(key, func() (any, error) {
		// Detached from the caller: others may be waiting on this fetch.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.fetchTimeout)
		defer cancel()

		v, err := fetch(fctx)
		if err != nil {
			return nil, err
		}
		payload, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to encode %s for the cache: %w", key, err)
		}
		if err := c.cache.Set(fctx, key, payload, c.now().Add(ttl)); err != nil {
			c.log.Warn("TMDB cache write failed", "key", key, "error", err)
		}
		return payload, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			if stale != nil && !errors.Is(res.Err, ErrNotFound) {
				if v, err := decode[T](stale); err == nil {
					c.log.Warn("TMDB unavailable; serving stale cache entry", "key", key, "error", res.Err)
					return v, nil
				}
			}
			return nil, res.Err
		}
		// Each caller decodes its own copy, so nobody shares mutable state.
		return decode[T](res.Val.([]byte))
	}
}

func decode[T any](payload []byte) (*T, error) {
	v := new(T)
	if err := json.Unmarshal(payload, v); err != nil {
		return nil, fmt.Errorf("failed to decode cached TMDB payload: %w", err)
	}
	return v, nil
}
