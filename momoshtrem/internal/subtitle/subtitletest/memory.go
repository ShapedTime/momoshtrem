// Package subtitletest has test doubles for the subtitle package.
package subtitletest

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// Repo is an in-memory subtitle.SubtitleRepository with the same keys as
// the Postgres one: one downloaded subtitle per item and language, torrent
// subtitles per torrent and file, one HDRezka subtitle per item and
// language.
type Repo struct {
	mu     sync.Mutex
	nextID int64
	rows   []*subtitle.Subtitle
	Now    func() time.Time
	// Streams, when set, are the items' active HDRezka files:
	// UpsertHDRezka fails with ErrStaleStream for another file. Unset, any
	// file is the item's.
	Streams map[subtitle.Item]string
}

var _ subtitle.SubtitleRepository = (*Repo)(nil)

// NewRepo returns an empty repository.
func NewRepo() *Repo {
	return &Repo{Now: time.Now}
}

// Add stores a row as is (assigning an ID), for seeding.
func (r *Repo) Add(sub subtitle.Subtitle) *subtitle.Subtitle {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	sub.ID = r.nextID
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = r.Now()
	}
	stored := sub
	r.rows = append(r.rows, &stored)
	return &stored
}

// All returns copies of every row.
func (r *Repo) All() []subtitle.Subtitle {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]subtitle.Subtitle, len(r.rows))
	for i, s := range r.rows {
		out[i] = *s
	}
	return out
}

func (r *Repo) UpsertDownloaded(_ context.Context, sub *subtitle.Subtitle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub.Source = subtitle.SourceOpenSubtitles
	sub.InfoHash = ""
	sub.CreatedAt = r.Now()
	for _, row := range r.rows {
		if row.Source == subtitle.SourceOpenSubtitles && row.Item() == sub.Item() && row.LanguageCode == sub.LanguageCode {
			sub.ID = row.ID
			*row = *sub
			return nil
		}
	}
	r.nextID++
	sub.ID = r.nextID
	stored := *sub
	r.rows = append(r.rows, &stored)
	return nil
}

func (r *Repo) UpsertTorrent(_ context.Context, sub *subtitle.Subtitle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub.Source = subtitle.SourceTorrent
	r.rows = slices.DeleteFunc(r.rows, func(row *subtitle.Subtitle) bool {
		return row.Source == subtitle.SourceTorrent && row.Item() == sub.Item() && row.InfoHash != sub.InfoHash
	})
	for _, row := range r.rows {
		if row.Source == subtitle.SourceTorrent && row.Item() == sub.Item() && row.InfoHash == sub.InfoHash && row.FilePath == sub.FilePath {
			sub.ID, sub.CreatedAt = row.ID, row.CreatedAt
			*row = *sub
			return nil
		}
	}
	r.nextID++
	sub.ID = r.nextID
	sub.CreatedAt = r.Now()
	stored := *sub
	r.rows = append(r.rows, &stored)
	return nil
}

func (r *Repo) UpsertHDRezka(_ context.Context, item subtitle.Item, streamFile string, subs []*subtitle.Subtitle, place func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Streams != nil && r.Streams[item] != streamFile {
		return subtitle.ErrStaleStream
	}
	if place != nil {
		if err := place(); err != nil {
			return err
		}
	}
	for _, sub := range subs {
		sub.Source = subtitle.SourceHDRezka
		sub.InfoHash = ""
		sub.CreatedAt = r.Now()
		replaced := false
		for _, row := range r.rows {
			if row.Source == subtitle.SourceHDRezka && row.Item() == sub.Item() && row.LanguageCode == sub.LanguageCode {
				sub.ID = row.ID
				*row = *sub
				replaced = true
				break
			}
		}
		if !replaced {
			r.nextID++
			sub.ID = r.nextID
			stored := *sub
			r.rows = append(r.rows, &stored)
		}
	}
	return nil
}

func (r *Repo) GetByID(_ context.Context, id int64) (*subtitle.Subtitle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.ID == id {
			c := *row
			return &c, nil
		}
	}
	return nil, nil
}

func (r *Repo) GetByItem(ctx context.Context, itemType subtitle.ItemType, itemID int64) ([]*subtitle.Subtitle, error) {
	item := subtitle.Item{Type: itemType, ID: itemID}
	m, _ := r.GetByItems(ctx, []subtitle.Item{item})
	return m[item], nil
}

func (r *Repo) GetByItems(_ context.Context, items []subtitle.Item) (map[subtitle.Item][]*subtitle.Subtitle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := make(map[subtitle.Item]bool, len(items))
	for _, it := range items {
		want[it] = true
	}
	out := make(map[subtitle.Item][]*subtitle.Subtitle)
	for _, row := range r.rows {
		if want[row.Item()] {
			c := *row
			out[row.Item()] = append(out[row.Item()], &c)
		}
	}
	for _, subs := range out {
		slices.SortFunc(subs, func(a, b *subtitle.Subtitle) int {
			return cmp.Or(
				cmp.Compare(a.LanguageCode, b.LanguageCode),
				cmp.Compare(torrentLast(a), torrentLast(b)),
				cmp.Compare(a.ID, b.ID),
			)
		})
	}
	return out, nil
}

// torrentLast orders downloaded subtitles before those that came with the
// file (torrent or HDRezka).
func torrentLast(s *subtitle.Subtitle) int {
	if s.Source == subtitle.SourceOpenSubtitles {
		return 0
	}
	return 1
}

func (r *Repo) GetDownloaded(_ context.Context, itemType subtitle.ItemType, itemID int64, languageCode string) (*subtitle.Subtitle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.Source == subtitle.SourceOpenSubtitles && row.ItemType == itemType && row.ItemID == itemID && row.LanguageCode == languageCode {
			c := *row
			return &c, nil
		}
	}
	return nil, nil
}

func (r *Repo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.rows)
	r.rows = slices.DeleteFunc(r.rows, func(row *subtitle.Subtitle) bool { return row.ID == id })
	if len(r.rows) == n {
		return subtitle.ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteByItem(_ context.Context, itemType subtitle.ItemType, itemID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = slices.DeleteFunc(r.rows, func(row *subtitle.Subtitle) bool {
		return row.ItemType == itemType && row.ItemID == itemID
	})
	return nil
}

func (r *Repo) DeleteByItems(_ context.Context, items []subtitle.Item, sources ...subtitle.Source) ([]*subtitle.Subtitle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted []*subtitle.Subtitle
	r.rows = slices.DeleteFunc(r.rows, func(row *subtitle.Subtitle) bool {
		if !slices.Contains(items, row.Item()) || (len(sources) > 0 && !slices.Contains(sources, row.Source)) {
			return false
		}
		c := *row
		deleted = append(deleted, &c)
		return true
	})
	return deleted, nil
}

// Fetcher is a fake opensubtitles.SubtitleFetcher.
type Fetcher struct {
	mu sync.Mutex

	// Response answers every search; SearchErr fails it.
	Response  *opensubtitles.SearchResponse
	SearchErr error
	// Files answers downloads by file ID with (content, file name).
	Files       map[int]File
	DownloadErr error

	// Searches records every search's parameters.
	Searches []opensubtitles.SearchParams
}

// File is a downloadable subtitle file.
type File struct {
	Name    string
	Content []byte
}

var _ opensubtitles.SubtitleFetcher = (*Fetcher)(nil)

func (f *Fetcher) IsConfigured() bool { return true }

func (f *Fetcher) Search(_ context.Context, params opensubtitles.SearchParams) (*opensubtitles.SearchResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Searches = append(f.Searches, params)
	if f.SearchErr != nil {
		return nil, f.SearchErr
	}
	if f.Response == nil {
		return &opensubtitles.SearchResponse{}, nil
	}
	return f.Response, nil
}

func (f *Fetcher) Download(_ context.Context, fileID int) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DownloadErr != nil {
		return nil, "", f.DownloadErr
	}
	file, ok := f.Files[fileID]
	if !ok {
		return nil, "", errNoFile
	}
	return file.Content, file.Name, nil
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

const errNoFile = fakeErr("no such file")
