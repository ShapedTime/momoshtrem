package service

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// HDRezkaStreamer opens HDRezka files' bytes. *hdrezka.Streamer implements
// it.
type HDRezkaStreamer interface {
	Open(ctx context.Context, f hdrezka.File, off, size int64) (io.ReadCloser, error)
}

var _ HDRezkaStreamer = (*hdrezka.Streamer)(nil)

// HDRezkaSource streams the WebDAV tree's HDRezka files: it turns an
// assignment's stored stream into the resolver's key and opens it with the
// streamer, at the size measured when it was assigned.
type HDRezkaSource struct {
	streamer HDRezkaStreamer
	log      *slog.Logger
}

var _ vfs.RemoteSource = (*HDRezkaSource)(nil)

// NewHDRezkaSource creates an HDRezkaSource.
func NewHDRezkaSource(streamer HDRezkaStreamer) *HDRezkaSource {
	return &HDRezkaSource{streamer: streamer, log: slog.With("component", "hdrezka-stream")}
}

// OpenRemote opens a's stream from off. Failures are logged with the
// stream (never its signed link); a size that no longer matches is logged
// too, so the item can be assigned again to measure it.
func (s *HDRezkaSource) OpenRemote(ctx context.Context, a *library.TorrentAssignment, off int64) (io.ReadCloser, error) {
	if a.Stream == nil {
		s.log.Error("HDRezka file without a stream", "item_type", a.ItemType, "item_id", a.ItemID, "file_path", a.FilePath)
		return nil, errors.New("the assignment has no HDRezka stream")
	}
	body, err := s.streamer.Open(ctx, StreamFile(*a.Stream), off, a.FileSize)
	if errors.Is(err, context.Canceled) {
		return nil, err // the player went away
	}
	if err != nil {
		r := a.Stream
		attrs := []any{"item_type", a.ItemType, "item_id", a.ItemID, "post_id", r.PostID, "translator_id", r.TranslatorID,
			"season", r.Season, "episode", r.Episode, "quality", r.Quality, "offset", off, "size", a.FileSize, "error", err}
		if errors.Is(err, hdrezka.ErrSizeMismatch) {
			s.log.Error("HDRezka file changed size since it was assigned; assign it again", attrs...)
		} else {
			s.log.Error("Can't stream from HDRezka", attrs...)
		}
		return nil, err
	}
	return body, nil
}

// StreamFile is the streamer's file for a stored stream.
func StreamFile(r library.StreamRef) hdrezka.File {
	return hdrezka.File{
		Key: hdrezka.StreamKey{PostID: int(r.PostID), Path: r.Path, TranslatorID: r.TranslatorID,
			Camrip: r.Camrip, Ads: r.Ads, Director: r.Director, Season: r.Season, Episode: r.Episode},
		Quality: r.Quality,
	}
}
