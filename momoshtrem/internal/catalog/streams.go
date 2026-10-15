package catalog

import "github.com/shapedtime/momoshtrem/internal/library"

// HDRezka files (PLAN §6 "HDRezka (#63)"). Never reuse releaseName for
// them: a stream has no magnet or torrent folder, and its made-up file path
// starts with "hdrezka/".

// sourceOf is where an assigned file comes from: torrent or hdrezka.
func sourceOf(a *library.TorrentAssignment) *string {
	if a == nil {
		return nil
	}
	source := SourceTorrent
	if !a.IsTorrent() {
		source = string(a.Kind)
	}
	return &source
}

// episodeStream is an episode's HDRezka stream; nil for a torrent file.
func episodeStream(a *library.TorrentAssignment) *EpisodeStream {
	if a == nil || a.IsTorrent() || a.Stream == nil {
		return nil
	}
	return &EpisodeStream{Translator: a.Stream.TranslatorName, Quality: a.Stream.Quality}
}

// titleStream is a movie's HDRezka stream; nil for a torrent file.
func (s *Service) titleStream(a *library.TorrentAssignment) *TitleStream {
	if a == nil || a.IsTorrent() || a.Stream == nil {
		return nil
	}
	r := a.Stream
	provider := r.Provider
	if provider == "" {
		provider = string(a.Kind)
	}
	return &TitleStream{
		Provider:   provider,
		Title:      r.Title,
		URL:        s.hdrezkaURL + r.Path,
		Translator: r.TranslatorName,
		Quality:    r.Quality,
		SizeBytes:  a.FileSize,
		AddedAt:    a.CreatedAt,
	}
}
