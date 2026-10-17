package activity

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Detail returns a torrent's list item plus its detail: the magnet, piece
// map, files (with what each plays as, or why it's ignored), trackers and
// totals. It never loads the torrent: a loaded one's facts come from the
// client, else from its stored metainfo, else from the newest job's file
// list. torrent.ErrInvalidInfoHash for a malformed hash,
// torrent.ErrTorrentNotFound when nothing knows it.
func (s *Service) Detail(ctx context.Context, infoHash string) (*Detail, error) {
	hash := torrent.NormalizeInfoHash(infoHash)
	if hash == "" {
		return nil, fmt.Errorf("%w: %q", torrent.ErrInvalidInfoHash, infoHash)
	}
	known, st, err := s.lookup(ctx, hash)
	if err != nil {
		return nil, err
	}
	d := &Detail{Item: s.item(hash, known, st, s.torrents.Pauses(), s.streams(ctx), s.snapshot())}

	var live *torrent.Detail
	if st != nil {
		if live, err = s.torrents.Detail(hash); err != nil {
			live = nil // dropped meanwhile: read it as not loaded
		}
	}
	magnet, err := s.library.TorrentMagnet(ctx, hash)
	if err != nil {
		return nil, err
	}
	if magnet == "" && live != nil {
		magnet = live.Magnet
	}
	if magnet != "" {
		d.MagnetURI = &magnet
	}
	mappings, err := s.library.ActiveMappings(ctx, hash)
	if err != nil {
		return nil, err
	}
	jobFiles, err := s.library.JobFiles(ctx, hash)
	if err != nil {
		return nil, err
	}

	var stored *torrent.StoredInfo
	if live == nil || !live.HasInfo {
		stored = s.storedInfo(hash)
	}
	var files []torrent.FileEntry
	switch {
	case live != nil && live.HasInfo:
		files = live.Files
		d.PieceLength, d.PiecesTotal, d.PiecesHave = &live.PieceLength, &live.PiecesTotal, &live.PiecesHave
		d.Pieces = live.Pieces
	case stored != nil:
		files = stored.Files
		d.PieceLength, d.PiecesTotal = &stored.PieceLength, &stored.PiecesTotal
	default:
		for _, f := range jobFiles {
			files = append(files, torrent.FileEntry{Path: f.Path, Size: f.Size})
		}
	}
	d.Files = detailFiles(files, live != nil && live.HasInfo, mappings, jobFiles)

	switch {
	case live != nil:
		d.Trackers = trackers(live.Trackers)
	case stored != nil:
		d.Trackers = trackers(stored.Trackers)
	default:
		d.Trackers = trackers(magnetTrackers(magnet))
	}
	if live != nil {
		down, up := live.DownloadedBytes, live.UploadedBytes
		d.DownloadedBytes, d.UploadedBytes = &down, &up
		if down > 0 {
			ratio := float64(up) / float64(down)
			d.Ratio = &ratio
		}
		d.Swarm = &Swarm{ConnectedPeers: live.Peers, ConnectedSeeders: live.Seeders}
	}
	return d, nil
}

// detailFiles is the files in the torrent's order, each with what its
// active assignments play it as, and the newest job's reason for leaving
// out the rest. Progress needs the client (loaded).
func detailFiles(files []torrent.FileEntry, loaded bool, mappings []library.FileMapping, jobFiles []library.JobFile) []DetailFile {
	plays := map[string][]PlaysAs{}
	for _, m := range mappings {
		switch {
		case m.MovieID != nil:
			plays[m.FilePath] = append(plays[m.FilePath], PlaysAs{MediaType: string(library.JobMovie), ItemID: *m.MovieID, Label: "Movie"})
		case m.Episode != nil:
			e := m.Episode
			season, number := e.Season, e.Number
			p := PlaysAs{MediaType: string(library.JobTV), ItemID: e.ID, SeasonNumber: &season, EpisodeNumber: &number,
				Label: fmt.Sprintf("S%02dE%02d", season, number)}
			if e.Name != "" {
				name := e.Name
				p.EpisodeName = &name
			}
			plays[m.FilePath] = append(plays[m.FilePath], p)
		}
	}
	ignored := map[string]*Ignored{}
	for _, f := range jobFiles {
		if ig := ignoredOf(f); ig != nil {
			ignored[f.Path] = ig
		}
	}

	out := make([]DetailFile, 0, len(files))
	for _, f := range files {
		df := DetailFile{Path: f.Path, SizeBytes: f.Size, PlaysAs: plays[f.Path]}
		if loaded {
			progress := 1.0
			if f.Size > 0 {
				progress = float64(f.BytesCompleted) / float64(f.Size)
			}
			df.Progress = &progress
		}
		if df.PlaysAs == nil {
			df.PlaysAs = []PlaysAs{}
			df.Ignored = ignored[f.Path]
		}
		out = append(out, df)
	}
	return out
}

// ignoredOf is why a job left a file out; nil when it didn't, and for a
// subtitle that follows its video.
func ignoredOf(f library.JobFile) *Ignored {
	if f.Decision != library.DecisionIgnored || f.Linked {
		return nil
	}
	switch {
	case f.Kind == library.KindExtra && f.ExtraType != "":
		return &Ignored{Reason: f.ExtraType, Label: "Ignored (" + extraLabel(f.ExtraType) + ")"}
	case f.Kind == library.KindExtra:
		return &Ignored{Reason: "other", Label: "Ignored (extra)"}
	case f.Kind == library.KindSubtitle:
		return &Ignored{Reason: "subtitle", Label: "Ignored (subtitle)"}
	default:
		return &Ignored{Reason: "user", Label: "Ignored"}
	}
}

// extraLabel names an extra's type in "Ignored (…)".
func extraLabel(extraType string) string {
	switch extraType {
	case "opening":
		return "NCOP"
	case "ending":
		return "NCED"
	case "info", "image":
		return "not video"
	case "superseded":
		return "older version"
	case "other":
		return "extra"
	default:
		return extraType // sample, trailer, featurette, font
	}
}

// trackers lists tracker URLs, each once. anacrolix keeps its announce
// results to itself, so every status is unknown.
func trackers(urls []string) []Tracker {
	out := make([]Tracker, 0, len(urls))
	seen := map[string]bool{}
	for _, u := range urls {
		if u = strings.TrimSpace(u); u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, Tracker{URL: u, Status: "unknown"})
	}
	return out
}

// magnetTrackers are a magnet's tr parameters.
func magnetTrackers(magnet string) []string {
	u, err := url.Parse(magnet)
	if err != nil {
		return nil
	}
	return u.Query()["tr"]
}
