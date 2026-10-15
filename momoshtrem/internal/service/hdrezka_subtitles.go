package service

import (
	"context"
	"errors"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// The subtitles of HDRezka streams (#68): an assign downloads the tracks
// each stream offers (their links expire like the MP4s), and the subtitle
// service converts them to SRT and stores them as the items' HDRezka
// subtitles, served next to the videos as local files.

// HDRezkaSubtitleStore stores the subtitles that come with HDRezka streams.
// *subtitle.Service implements it.
type HDRezkaSubtitleStore interface {
	SaveHDRezka(ctx context.Context, item subtitle.Item, stream subtitle.HDRezkaStream, tracks []subtitle.HDRezkaTrack) ([]*subtitle.Subtitle, error)
}

var _ HDRezkaSubtitleStore = (*subtitle.Service)(nil)

// streamSubtitles is an item just assigned and the subtitle tracks its
// stream offers.
type streamSubtitles struct {
	item   subtitle.Item
	ref    library.StreamRef
	tracks []hdrezka.Subtitle
}

// finishAssign records the subtitles of the streams just written and
// rebuilds the tree. When they're in before the assign's budget (bctx)
// runs out, the tree is rebuilt once, with them. Otherwise it's rebuilt
// now, so the videos show at once, and again when the subtitles are in:
// they go on in the background, within hdrezkaSubtitleBudget. A show's 30
// episodes with three tracks each needn't fit in the 25 s, and the assign
// mustn't answer after the web proxy's 30 s. A track that fails is logged
// and never fails the assign.
func (s *HDRezkaService) finishAssign(ctx, bctx context.Context, streams []streamSubtitles) {
	tracks := 0
	for _, st := range streams {
		tracks += len(st.tracks)
	}
	if s.subtitles == nil || tracks == 0 {
		s.tree.InvalidateTree()
		return
	}

	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.subtitleBudget)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		s.recordSubtitles(rctx, streams)
	}()
	select {
	case <-done:
		s.tree.InvalidateTree()
	case <-bctx.Done():
		s.log.Info("HDRezka subtitles are still downloading; the tree is rebuilt again when they're in",
			"items", len(streams), "tracks", tracks)
		s.tree.InvalidateTree()
		go func() {
			<-done
			s.tree.InvalidateTree()
		}()
	}
}

// recordSubtitles downloads every item's tracks, hdrezkaSubtitleWorkers at
// a time and each within subtitleTimeout, and stores an item's as soon as
// its last track is in.
func (s *HDRezkaService) recordSubtitles(ctx context.Context, streams []streamSubtitles) {
	var g errgroup.Group
	g.SetLimit(hdrezkaSubtitleWorkers)
	for _, st := range streams {
		if len(st.tracks) == 0 {
			continue
		}
		got := make([]*subtitle.HDRezkaTrack, len(st.tracks))
		left := new(atomic.Int32)
		left.Store(int32(len(st.tracks)))
		for i, track := range st.tracks {
			g.Go(func() error {
				got[i] = s.fetchTrack(ctx, st, track)
				// The last of the item's tracks stores them all: the
				// atomic orders the others' writes to got before it.
				if left.Add(-1) == 0 {
					s.saveTracks(ctx, st, got)
				}
				return nil
			})
		}
	}
	g.Wait()
}

// fetchTrack downloads one track; nil when it failed (logged, without the
// signed link).
func (s *HDRezkaService) fetchTrack(ctx context.Context, st streamSubtitles, track hdrezka.Subtitle) *subtitle.HDRezkaTrack {
	fctx, cancel := context.WithTimeout(ctx, s.subtitleTimeout)
	defer cancel()
	vtt, err := s.site.Subtitle(fctx, track.URL)
	if err != nil {
		s.log.Warn("HDRezka subtitle not downloaded", "item_type", st.item.Type, "item_id", st.item.ID,
			"language", track.Code, "name", track.Language, "error", err)
		return nil
	}
	return &subtitle.HDRezkaTrack{Code: track.Code, Name: track.Language, VTT: vtt}
}

// saveTracks stores the tracks of an item that were downloaded.
func (s *HDRezkaService) saveTracks(ctx context.Context, st streamSubtitles, got []*subtitle.HDRezkaTrack) {
	var tracks []subtitle.HDRezkaTrack
	for _, t := range got {
		if t != nil {
			tracks = append(tracks, *t)
		}
	}
	if len(tracks) == 0 {
		return
	}
	stream := subtitle.HDRezkaStream{File: st.ref.FilePath(), Release: st.ref.Label()}
	_, err := s.subtitles.SaveHDRezka(ctx, st.item, stream, tracks)
	switch {
	case errors.Is(err, subtitle.ErrStaleStream):
		s.log.Info("HDRezka subtitles not stored: the item got another file meanwhile",
			"item_type", st.item.Type, "item_id", st.item.ID)
	case err != nil:
		s.log.Error("Failed to store HDRezka subtitles", "item_type", st.item.Type, "item_id", st.item.ID, "error", err)
	}
}
