package vfs

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// subtitleLister loads subtitles for the tree: one query for every item.
type subtitleLister interface {
	GetByItems(ctx context.Context, items []subtitle.Item) (map[subtitle.Item][]*subtitle.Subtitle, error)
}

var _ subtitleLister = (*subtitle.Repository)(nil)

// namedSubtitle is a subtitle with its file name next to the video.
type namedSubtitle struct {
	Name string
	Sub  *subtitle.Subtitle
}

// subtitleFileNames names an item's subtitles next to its video, whose file
// name without the extension is base.
//
// The first subtitle in each language and format keeps
// "{base}.{lang}.{fmt}", the name players match to the video; downloaded
// subtitles come before torrent ones, then the oldest first. Later ones are
// "{base}.{n}.{lang}.{fmt}", n counting from 2, so a downloaded and a
// torrent subtitle (or two unknown-language files) don't overwrite each
// other. Counting per format keeps a VobSub .idx/.sub pair on one stem.
func subtitleFileNames(base string, subs []*subtitle.Subtitle) []namedSubtitle {
	sorted := slices.Clone(subs)
	slices.SortStableFunc(sorted, func(a, b *subtitle.Subtitle) int {
		return cmp.Or(
			cmp.Compare(subtitleLanguage(a), subtitleLanguage(b)),
			cmp.Compare(sourceRank(a.Source), sourceRank(b.Source)),
			cmp.Compare(a.ID, b.ID),
		)
	})

	out := make([]namedSubtitle, 0, len(sorted))
	used := make(map[string]bool, len(sorted))
	seen := make(map[string]int, len(sorted))
	for _, sub := range sorted {
		lang := subtitleLanguage(sub)
		format := strings.ToLower(sub.Format)
		if format == "" {
			format = "srt"
		}
		numbered := func(n int) string { return base + "." + strconv.Itoa(n) + "." + lang + "." + format }

		key := lang + "." + format
		seen[key]++
		name := base + "." + key
		if seen[key] > 1 {
			name = numbered(seen[key])
		}
		for used[name] {
			seen[key]++
			name = numbered(seen[key])
		}
		used[name] = true
		out = append(out, namedSubtitle{Name: name, Sub: sub})
	}
	return out
}

func subtitleLanguage(sub *subtitle.Subtitle) string {
	if sub.LanguageCode == "" {
		return "unknown"
	}
	return strings.ToLower(sub.LanguageCode)
}

// sourceRank puts downloaded subtitles before torrent ones.
func sourceRank(s subtitle.Source) int {
	if s == subtitle.SourceOpenSubtitles {
		return 0
	}
	return 1
}

// subtitleDir is a video in the tree whose subtitles go next to it.
type subtitleDir struct {
	dir      *VirtualDir
	dirPath  string
	base     string // The video's file name without its extension
	item     subtitle.Item
	infoHash string // The video's torrent: only its own torrent subtitles show
	// more are the other episodes a grouped entry ("S01E01-E02") plays:
	// their subtitles are named after it too.
	more []subtitle.Item
}

// subtitles collects the subtitles of the video's items from subs.
func (d subtitleDir) subtitles(subs map[subtitle.Item][]*subtitle.Subtitle) []*subtitle.Subtitle {
	if len(d.more) == 0 {
		return subs[d.item]
	}
	out := append([]*subtitle.Subtitle(nil), subs[d.item]...)
	for _, item := range d.more {
		out = append(out, subs[item]...)
	}
	return out
}

// loadSubtitles fetches the subtitles of every video in dirs in one query.
// It returns nil when subtitles aren't configured or the query fails.
func (fs *LibraryFS) loadSubtitles(dirs []subtitleDir) map[subtitle.Item][]*subtitle.Subtitle {
	fs.mu.RLock()
	repo := fs.subtitleRepo
	fs.mu.RUnlock()
	if repo == nil || len(dirs) == 0 {
		return nil
	}
	items := make([]subtitle.Item, 0, len(dirs))
	for _, d := range dirs {
		items = append(items, d.item)
		items = append(items, d.more...)
	}
	subs, err := repo.GetByItems(context.Background(), items)
	if err != nil {
		slog.Error("Failed to get subtitles for VFS", "items", len(items), "error", err)
		return nil
	}
	return subs
}

// addSubtitleFiles puts the video's current subtitles next to it. The caller
// owns tree (a tree being built) or holds fs.mu.
func addSubtitleFiles(tree *DirectoryTree, d subtitleDir, subs []*subtitle.Subtitle) {
	for _, ns := range subtitleFileNames(d.base, subtitle.Current(subs, d.infoHash)) {
		var entry Entry
		if ns.Sub.Source == subtitle.SourceTorrent {
			// Streamed from the torrent on open.
			entry = NewTorrentSubtitleFile(ns.Name, ns.Sub.FilePath, ns.Sub.FileSize, ns.Sub.InfoHash)
		} else {
			// Downloaded from OpenSubtitles to local storage.
			entry = NewSubtitleFile(ns.Name, ns.Sub.FilePath, ns.Sub.FileSize)
		}
		d.dir.children[ns.Name] = entry
		tree.pathMap[d.dirPath+"/"+ns.Name] = entry
	}
}
