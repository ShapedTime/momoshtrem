package downloads

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
)

// The API's service (GET, POST, PATCH and DELETE /downloads, and the
// estimate; PLAN §6 "Downloads (#77)"), the storage governor's usage, and
// what smart downloads (P5 #203) build on. Every change runs a pass before
// it answers, so the answer is the download as the manager now plans it.

// ErrTargetNotFound is a Keep or an estimate on something the library
// hasn't got: 404.
var ErrTargetNotFound = errors.New("not in the library")

// ErrSmartPick is a PATCH of a smart pick, which has no duration of its own
// (Keep it with POST first): 422 validation_error.
var ErrSmartPick = errors.New("a smart pick has no duration: keep it first")

var _ storagecap.Downloads = (*Service)(nil)
var _ storagecap.CommitGuard = (*Service)(nil)

// DownloadUsage is what downloads hold (storagecap.Downloads): on disk,
// and what kept downloads commit, from the latest pass.
func (s *Service) DownloadUsage(context.Context) (storagecap.DownloadUsage, error) {
	u := s.Snapshot().Usage()
	return storagecap.DownloadUsage{KeptBytes: u.KeptBytes, SmartBytes: u.SmartBytes, CommittedKeptBytes: u.CommittedKeptBytes}, nil
}

// LockCommits holds Keeps (and passes) until unlock (storagecap.
// CommitGuard): a storage PUT checks what's committed and stores its caps
// while no Keep can commit more.
func (s *Service) LockCommits() (unlock func()) {
	s.passMu.Lock()
	return s.passMu.Unlock
}

// List is GET /downloads, from the latest pass; with none yet, one runs.
func (s *Service) List(ctx context.Context) (List, error) {
	if s.Snapshot() == nil {
		if err := s.Refresh(ctx); err != nil {
			return List{}, err
		}
	}
	return s.Snapshot().List(), nil
}

// Keep is POST /downloads: a Keep on a movie, show, season or episode. It
// answers the download and whether it's new (201): already kept is the
// download unchanged, and a smart pick turned kept is the same row, created
// now with the duration asked for (200). Errors, in the contract's order:
// ErrNotAnObject, *FieldError, ErrTargetNotFound, ErrNotDownloadable,
// *StorageFullError.
func (s *Service) Keep(ctx context.Context, body []byte) (Download, bool, error) {
	req, err := parseKeep(body, s.clock.Now())
	if err != nil {
		return Download{}, false, err
	}
	target, err := s.resolveTarget(ctx, req.target)
	if err != nil {
		return Download{}, false, err
	}
	if hdrezkaOnly(target) {
		return Download{}, false, ErrNotDownloadable
	}

	s.passMu.Lock()
	defer s.passMu.Unlock()
	// The releases due now go first: a Keep on an until-watched download
	// that was just watched is a new Keep (201), not that one (200).
	p, err := s.plan(ctx)
	if err != nil {
		return Download{}, false, err
	}
	if err := s.writeReleases(ctx, p); err != nil {
		return Download{}, false, err
	}
	// This plan isn't applied, and the next one won't see those releases:
	// their watched files go first when the cache trims, from now.
	s.demote(p.stage.released, true, p.refs)
	existing, err := s.d.Store.ActiveDownloadFor(ctx, target.DownloadRecord)
	if err != nil {
		return Download{}, false, err
	}
	if existing != nil && existing.Reason == library.DownloadKept {
		d, err := s.viewAfterPass(ctx, existing.ID)
		return d, false, err
	}
	if needed, available := s.room(p, target); needed > available {
		return Download{}, false, &StorageFullError{NeededBytes: needed, AvailableBytes: available}
	}

	now := s.clock.Now()
	until := p.settings.Kept.DefaultUntilWatched
	if req.untilWatched != nil {
		until = *req.untilWatched
	}
	var expires *time.Time
	switch {
	case req.setExpiry:
		expires = req.expiresAt
	case p.settings.Kept.DefaultDays != nil:
		at := now.AddDate(0, 0, *p.settings.Kept.DefaultDays)
		expires = &at
	}
	var rec library.DownloadRecord
	created := existing == nil
	if created {
		rec = target.DownloadRecord
		rec.Reason, rec.UntilWatched, rec.ExpiresAt, rec.CreatedAt = library.DownloadKept, until, expires, now
		rec, err = s.d.Store.CreateDownload(ctx, rec)
	} else {
		rec, err = s.d.Store.KeepSmartDownload(ctx, existing.ID, until, expires, now)
	}
	if err != nil {
		return Download{}, false, err
	}
	s.log.Info("Kept", "download_id", rec.ID, "scope", rec.Scope, "title", target.Title.Title,
		"until_watched", until, "expires_at", expires, "from_smart", !created)
	d, err := s.viewAfterPass(ctx, rec.ID)
	return d, created, err
}

// Estimate is GET /downloads/estimate: what a Keep on the target would
// want and need, against what the kept cap has left. It writes nothing.
func (s *Service) Estimate(ctx context.Context, t library.DownloadTarget) (Estimate, error) {
	target, err := s.resolveTarget(ctx, t)
	if err != nil {
		return Estimate{}, err
	}
	s.passMu.Lock()
	defer s.passMu.Unlock()
	p, err := s.plan(ctx)
	if err != nil {
		return Estimate{}, err
	}
	existing, err := s.d.Store.ActiveDownloadFor(ctx, target.DownloadRecord)
	if err != nil {
		return Estimate{}, err
	}
	est := Estimate{Downloadable: !hdrezkaOnly(target)}
	for _, f := range s.targetFiles(target) {
		est.WantedBytes += f.size
	}
	est.NeededBytes, est.AvailableBytes = s.room(p, target)
	if existing != nil && existing.Reason == library.DownloadKept {
		est.NeededBytes = 0
	}
	return est, nil
}

// Update is PATCH /downloads/{id}: a kept download's duration. A change
// that ends the download (until watched, on a movie watched since) answers
// its final view; the next GET doesn't list it. Errors, in the contract's
// order: ErrNotFound, ErrNotAnObject, ErrSmartPick, *FieldError.
func (s *Service) Update(ctx context.Context, id int64, body []byte) (Download, error) {
	rec, err := s.listed(ctx, id)
	if err != nil {
		return Download{}, err
	}
	fields, err := object(body)
	if err != nil {
		return Download{}, err
	}
	if rec.Reason == library.DownloadSmart {
		return Download{}, ErrSmartPick
	}
	patch, err := parseDuration(fields, s.clock.Now())
	if err != nil {
		return Download{}, err
	}
	s.passMu.Lock()
	defer s.passMu.Unlock()
	if _, err := s.d.Store.UpdateDownloadDuration(ctx, id, library.DownloadDurationPatch{
		UntilWatched: patch.untilWatched, SetExpiresAt: patch.setExpiry, ExpiresAt: patch.expiresAt,
	}); err != nil {
		if errors.Is(err, library.ErrDownloadNotFound) {
			return Download{}, ErrNotFound
		}
		return Download{}, err
	}
	return s.viewAfterPass(ctx, id)
}

// Delete is DELETE /downloads/{id}: it releases the download (deleted).
// Its data stays as ordinary cache; a smart pick released this way is
// never picked again. ErrNotFound for an unknown or released id.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.listed(ctx, id); err != nil {
		return err
	}
	return s.release(ctx, id, library.ReleaseDeleted)
}

// UpsertSmart adds a smart pick of an episode, for P5 (#203): until
// watched, at most SmartLifetime. created is false when the episode has an
// active download of its own already (kept or smart; id is that one). The
// re-pick rules come first: ErrRepickBlocked (wrapping why: watched,
// dismissed or expired) for an episode they rule out. A pass follows soon.
func (s *Service) UpsertSmart(ctx context.Context, episodeID int64, source library.SmartSource) (id int64, created bool, err error) {
	if source != library.SmartUpNext && source != library.SmartStreaming {
		return 0, false, fmt.Errorf("unknown smart source %q", source)
	}
	why, err := s.d.Store.SmartPickBlocked(ctx, episodeID)
	if err != nil {
		return 0, false, err
	}
	if why != "" {
		return 0, false, fmt.Errorf("%w: %s", ErrRepickBlocked, why)
	}
	s.passMu.Lock()
	now := s.clock.Now()
	id, created, err = s.d.Store.UpsertSmartDownload(ctx, episodeID, source, now, now.Add(SmartLifetime))
	s.passMu.Unlock()
	if err != nil {
		return 0, false, err
	}
	if created {
		s.log.Info("Smart pick", "download_id", id, "episode_id", episodeID, "source", source)
		s.Wake()
	}
	return id, created, nil
}

// Release ends an active download (P5 releases smart picks that stop
// being candidates; reason is why), then runs a pass. ErrNotFound for an
// unknown or released id.
func (s *Service) Release(ctx context.Context, id int64, reason library.ReleaseReason) error {
	if _, err := s.d.Store.GetDownload(ctx, id); err != nil {
		if errors.Is(err, library.ErrDownloadNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.release(ctx, id, reason)
}

func (s *Service) release(ctx context.Context, id int64, reason library.ReleaseReason) error {
	s.passMu.Lock()
	defer s.passMu.Unlock()
	ids, err := s.d.Store.ReleaseDownloads(ctx, []library.DownloadRelease{{ID: id, Reason: reason}}, s.clock.Now())
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return ErrNotFound
	}
	s.log.Info("Download released", "download_id", id, "reason", reason)
	_, err = s.passLocked(ctx, true)
	return err
}

// listed is the active download with the id, as the API sees it: a smart
// pick while smart downloads are off isn't.
func (s *Service) listed(ctx context.Context, id int64) (library.DownloadRecord, error) {
	rec, err := s.d.Store.GetDownload(ctx, id)
	if errors.Is(err, library.ErrDownloadNotFound) {
		return rec, ErrNotFound
	}
	if err != nil {
		return rec, err
	}
	if rec.Reason == library.DownloadSmart {
		settings, _, err := s.d.Limits.Limits(ctx)
		if err != nil {
			return rec, err
		}
		if !settings.Smart.Enabled {
			return rec, ErrNotFound
		}
	}
	return rec, nil
}

// viewAfterPass runs a pass (passMu held) and answers the download as it
// planned it; one the pass released (a PATCH that made a watched download
// until watched) as it read just before.
func (s *Service) viewAfterPass(ctx context.Context, id int64) (Download, error) {
	p, err := s.passLocked(ctx, true)
	if err != nil {
		return Download{}, err
	}
	if d, ok := p.snapshot.Download(id); ok {
		return d, nil
	}
	if d, ok := p.final[id]; ok {
		return d, nil
	}
	return Download{}, fmt.Errorf("download %d isn't planned", id)
}

func (s *Service) resolveTarget(ctx context.Context, t library.DownloadTarget) (*library.ResolvedDownload, error) {
	target, err := s.d.Store.ResolveTarget(ctx, t)
	switch {
	case errors.Is(err, library.ErrMovieNotFound), errors.Is(err, library.ErrShowNotFound),
		errors.Is(err, library.ErrSeasonNotFound), errors.Is(err, library.ErrEpisodeNotFound):
		return nil, fmt.Errorf("%w: %w", ErrTargetNotFound, err)
	case err != nil:
		return nil, err
	}
	return target, nil
}

// hdrezkaOnly says every file the target covers plays from HDRezka (and it
// covers one): POST's 422 not_downloadable.
func hdrezkaOnly(t *library.ResolvedDownload) bool {
	files := 0
	for _, it := range t.Items {
		if it.File == nil {
			continue
		}
		if it.File.Kind != library.AssignmentKindHDRezka {
			return false
		}
		files++
	}
	return files > 0
}

// targetFiles are the downloadable torrent files a new Keep on the target
// would want: all it covers (nothing is watched since it would be created
// now).
func (s *Service) targetFiles(t *library.ResolvedDownload) []fileWant {
	d := *t
	d.CreatedAt = s.clock.Now()
	return cover(&d, fileInput{pieces: s.pieceMap}).wants
}

// room is the 409's maths: what a Keep on the target adds to what kept
// downloads commit (files a kept download has already count nothing), and
// what the kept cap has left.
func (s *Service) room(p *passPlan, t *library.ResolvedDownload) (needed, available int64) {
	for _, f := range s.targetFiles(t) {
		if !p.stage.adm.kept[f.key] {
			needed += f.size
		}
	}
	return needed, max(0, p.limits.KeptMaxBytes-p.stage.adm.committedKept)
}
