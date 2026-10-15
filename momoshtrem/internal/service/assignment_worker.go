package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Worker timings.
const (
	// idlePoll is how often an idle worker looks for queued jobs without
	// being woken.
	idlePoll = 5 * time.Second
	// metadataStaleAfter: a show's metadata older than this is refreshed
	// before its files are matched.
	metadataStaleAfter = 24 * time.Hour
	// refreshCooldown: a show whose files don't all match isn't refreshed
	// again within this long of its last refresh.
	refreshCooldown = time.Hour
	// refreshBudget bounds a metadata refresh; extrasBudget the TMDB
	// alternative titles and episode groups.
	refreshBudget = 45 * time.Second
	extrasBudget  = 10 * time.Second
	// maxEpisodeGroups caps the episode groups read for alternative
	// orderings.
	maxEpisodeGroups = 3
	// resolveBudget bounds one AI run.
	resolveBudget = 90 * time.Second
	// dryRunTTL is how long #6's dry-run jobs are kept.
	dryRunTTL = 24 * time.Hour
	// janitorEvery is how often old dry runs are removed.
	janitorEvery = time.Hour
)

// AI errors, shown to the user.
const (
	msgAITimeout     = "The AI didn't answer in time, so nothing changed."
	msgAIFailed      = "The AI check failed, so nothing changed."
	msgAIInterrupted = "The AI check was interrupted by a restart, so nothing changed."
)

// errJobMoved stops processing a job someone else moved on (discarded, or
// deleted with its title).
var errJobMoved = errors.New("the job moved on")

// Start recovers jobs a previous run left behind and starts the workers.
// Jobs before resolving start again from queued; an interrupted resolving
// job keeps the rules' proposals, with ai_error set.
func (s *JobService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	s.recoverInterrupted(ctx)
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.work(ctx)
	}
	s.wg.Add(1)
	go s.janitor(ctx)
	s.notify()
	s.log.Info("Assignment workers started", "workers", s.workers)
}

// Close stops the workers and waits for them. A job in flight stays where
// it is and is recovered on the next start. Call it before closing the
// torrent service.
func (s *JobService) Close() {
	if s.stop == nil {
		return
	}
	s.stop()
	s.wg.Wait()
	s.log.Info("Assignment workers stopped")
}

// notify wakes a worker.
func (s *JobService) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// work claims queued jobs one at a time (FOR UPDATE SKIP LOCKED, so
// workers never share one) until ctx ends.
func (s *JobService) work(ctx context.Context) {
	defer s.wg.Done()
	for {
		job, err := s.jobs.Claim(ctx)
		switch {
		case err == nil:
			s.process(ctx, job)
			continue
		case ctx.Err() != nil:
			return
		case !errors.Is(err, library.ErrJobNotFound):
			s.log.Error("Failed to claim an assignment job", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-time.After(idlePoll):
		}
	}
}

// janitor removes old dry runs.
func (s *JobService) janitor(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(janitorEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.jobs.DeleteDryRuns(ctx, dryRunTTL); err != nil {
				s.log.Warn("Failed to remove old dry runs", "error", err)
			} else if n > 0 {
				s.log.Info("Removed old dry runs", "jobs", n)
			}
		}
	}
}

// recoverInterrupted puts jobs a stopped worker left behind back on track.
func (s *JobService) recoverInterrupted(ctx context.Context) {
	if n, err := s.jobs.Requeue(ctx); err != nil {
		s.log.Error("Failed to requeue interrupted assignment jobs", "error", err)
	} else if n > 0 {
		s.log.Info("Requeued interrupted assignment jobs", "jobs", n)
	}
	ids, err := s.jobs.IDsWithStatus(ctx, library.JobResolving)
	if err != nil {
		s.log.Error("Failed to list interrupted AI runs", "error", err)
		return
	}
	for _, id := range ids {
		err := s.jobs.Locked(ctx, id, func(t *library.JobTx) error {
			if t.Job.Status != library.JobResolving {
				return nil
			}
			t.Job.AIError = msgAIInterrupted
			t.Job.Status = settledStatus(listed(t.Proposals))
			return t.SaveJob()
		})
		if err != nil {
			s.log.Error("Failed to recover an interrupted AI run", "job_id", id, "error", err)
		}
	}
}

// process runs a claimed job (in fetching_metadata) as far as it goes.
func (s *JobService) process(ctx context.Context, job *library.AssignmentJob) {
	log := s.log.With("job_id", job.ID, "info_hash", job.InfoHash)
	var err error
	switch job.MediaType {
	case library.JobTV:
		err = s.processShow(ctx, job)
	case library.JobMovie:
		err = s.processMovie(ctx, job)
	default:
		err = fmt.Errorf("unknown media type %q", job.MediaType)
	}
	switch {
	case err == nil, errors.Is(err, errJobMoved), errors.Is(err, library.ErrJobNotFound):
	case ctx.Err() != nil:
		log.Info("Assignment job interrupted; it resumes on the next start", "status", job.Status)
	default:
		log.Error("Assignment job failed", "error", err)
		if ferr := s.fail(ctx, job, failInternal); ferr != nil {
			log.Error("Failed to mark the job failed", "error", ferr)
		}
	}
}

// fail ends a working job with f. A job that isn't working any more (it
// was classified, or discarded) is left alone.
func (s *JobService) fail(ctx context.Context, job *library.AssignmentJob, f failure) error {
	ok, err := s.jobs.Fail(ctx, job.ID, f.code, f.message)
	if err != nil || !ok {
		return err
	}
	s.log.Warn("Assignment job failed", "job_id", job.ID, "code", f.code, "reason", f.message)
	s.releaseTorrent(ctx, job.InfoHash, job.ID)
	return nil
}

// processShow fetches the torrent's file list (refreshing the show's
// metadata and reading its TMDB extras meanwhile), classifies the files,
// stores the proposals and, when AI is available and something is unsure,
// asks it. A job that needs nothing reviewed applies on its own.
func (s *JobService) processShow(ctx context.Context, job *library.AssignmentJob) error {
	if job.ShowID == nil {
		return s.fail(ctx, job, failTitleGone)
	}
	show, err := s.shows.GetWithSeasonsAndEpisodesContext(ctx, *job.ShowID)
	if err != nil {
		return err
	}
	if show == nil {
		return s.fail(ctx, job, failTitleGone)
	}

	// A refresh is tried at most once per job, whether or not it works:
	// one that failed would fail again, for up to refreshBudget more.
	var (
		wg           sync.WaitGroup
		extras       CatalogExtras
		refreshTried bool
		refreshed    bool
	)
	if metadataStale(show, s.now()) {
		refreshTried = true
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		if refreshTried {
			refreshed = s.refreshShow(ctx, show)
		}
	}()
	go func() {
		defer wg.Done()
		extras = s.catalogExtras(ctx, show)
	}()
	info, addErr := s.addTorrent(ctx, job.MagnetURI, job.InfoHash)
	wg.Wait()
	if addErr != nil {
		return s.addFailed(ctx, job, addErr)
	}
	if !hasVideo(info.Files) {
		return s.fail(ctx, job, failNoVideos)
	}
	if refreshed {
		if show, err = s.reloadShow(ctx, show.ID); err != nil || show == nil {
			return s.failOr(ctx, job, err)
		}
	}

	classify := func() identify.Result {
		return identify.Classify(identify.Input{
			TorrentName: info.Name,
			Files:       info.Files,
			Show:        ShowCatalog(show, extras),
			Hint:        identifyHint(job.Hint),
			Now:         s.now(),
		})
	}
	res := classify()
	// A just-aired episode isn't in stale metadata: refresh and try again.
	if !refreshTried && unmatchedVideos(res.Proposals) && refreshDue(show, s.now()) && s.refreshShow(ctx, show) {
		if show, err = s.reloadShow(ctx, show.ID); err != nil || show == nil {
			return s.failOr(ctx, job, err)
		}
		res = classify()
	}

	drafts := showDrafts(res.Proposals)
	if err := s.setFileList(ctx, job, info, drafts); err != nil {
		return err
	}

	settings := s.aiSettings(ctx)
	job.ClassifierMode = show.ClassifierMode
	scope := unsurePaths(res.Proposals, show.ClassifierMode)
	resolving := s.aiAvailable(settings, job) && len(scope) > 0
	err = s.jobs.Locked(ctx, job.ID, func(t *library.JobTx) error {
		if t.Job.Status != library.JobClassifying {
			return errJobMoved
		}
		if err := t.ReplaceProposals(drafts); err != nil {
			return err
		}
		t.Job.ReleaseGroup = res.Pack.Group
		t.Job.SuggestedHints = capHints(res.SuggestedHints)
		if resolving {
			n := len(scope)
			t.Job.Status, t.Job.AIFiles = library.JobResolving, &n
		} else {
			t.Job.Status = settledStatus(listed(t.Proposals))
		}
		return t.SaveJob()
	})
	if err != nil {
		return err
	}
	s.log.Info("Assignment job classified", "job_id", job.ID, "files", len(drafts), "asking_ai", resolving)

	if resolving {
		cat := ShowCatalog(show, extras)
		if !s.resolveFirst(ctx, job, cat, info, res.Proposals, scope) {
			return nil
		}
	}
	return s.autoApply(ctx, job.ID, settings)
}

// processMovie fetches the torrent's file list, ranks its videos as the
// film and stores the proposals. A file picked up front is accepted and
// the job applies; otherwise one plausible film applies on its own.
func (s *JobService) processMovie(ctx context.Context, job *library.AssignmentJob) error {
	if job.MovieID == nil {
		return s.fail(ctx, job, failTitleGone)
	}
	movie, err := s.movies.GetByID(*job.MovieID)
	if err != nil {
		return err
	}
	if movie == nil {
		return s.fail(ctx, job, failTitleGone)
	}
	info, err := s.addTorrent(ctx, job.MagnetURI, job.InfoHash)
	if err != nil {
		return s.addFailed(ctx, job, err)
	}
	if !hasVideo(info.Files) {
		return s.fail(ctx, job, failNoVideos)
	}

	// The classifier reads the extras, subtitles and release group; the
	// film's own catalog is just its title.
	res := identify.Classify(identify.Input{
		TorrentName: info.Name,
		Files:       info.Files,
		Show:        &identify.ShowCatalog{Title: movie.Title, Year: movie.Year},
		Now:         s.now(),
	})
	candidates := identify.RankMovieFiles(info.Files, identify.MovieInfo{Title: movie.Title, Year: movie.Year, RuntimeMin: movie.Runtime})
	drafts := movieDrafts(res.Proposals, candidates)
	if err := s.setFileList(ctx, job, info, drafts); err != nil {
		return err
	}

	picked := false
	err = s.jobs.Locked(ctx, job.ID, func(t *library.JobTx) error {
		if t.Job.Status != library.JobClassifying {
			return errJobMoved
		}
		if err := t.ReplaceProposals(drafts); err != nil {
			return err
		}
		if t.Job.FilePath != "" {
			changed := pickMovie(t.Proposals, t.Job.FilePath)
			picked = len(changed) > 0
			if err := t.UpdateProposals(changed); err != nil {
				return err
			}
		}
		t.Job.ReleaseGroup = res.Pack.Group
		t.Job.Status = settledStatus(listed(t.Proposals))
		return t.SaveJob()
	})
	if err != nil {
		return err
	}
	s.log.Info("Assignment job classified", "job_id", job.ID, "files", len(drafts), "candidates", len(candidates))
	if picked {
		_, err := s.applyJob(ctx, job.ID, applyReview)
		var c *JobConflictError
		if errors.As(err, &c) {
			return nil // something still needs a look: it waits for review
		}
		return err
	}
	return s.autoApply(ctx, job.ID, s.aiSettings(ctx))
}

// setFileList stores the torrent's file list and moves the job on to
// classifying.
func (s *JobService) setFileList(ctx context.Context, job *library.AssignmentJob, info *torrent.TorrentInfo, drafts []library.ProposalDraft) error {
	var size int64
	for _, f := range info.Files {
		size += f.Size
	}
	ok, err := s.jobs.SetFileList(ctx, job.ID, library.FileList{
		Name: info.Name, SizeBytes: size, FileCount: len(info.Files), VideoCount: videoCount(drafts),
	})
	if err != nil {
		return err
	}
	if !ok {
		// Discarded while the file list was on its way.
		s.releaseTorrent(ctx, job.InfoHash, job.ID)
		return errJobMoved
	}
	return nil
}

// autoApply applies a classified job on its own when the setting allows
// and nothing needs a look.
func (s *JobService) autoApply(ctx context.Context, id int64, settings library.AISettings) error {
	if !settings.AutoApplyHighConfidence {
		return nil
	}
	_, err := s.applyJob(ctx, id, applyAuto)
	var c *JobConflictError
	if errors.As(err, &c) {
		return nil // reviewed or discarded meanwhile
	}
	return err
}

// addTorrent adds the torrent and returns its file list. Adds of one
// torrent are serialized: the client drops a torrent whose metadata times
// out, which would kill a concurrent add of the same one. AddTorrent takes
// no context, so on ctx's end the add is left to finish on its own.
func (s *JobService) addTorrent(ctx context.Context, magnetURI, infoHash string) (*torrent.TorrentInfo, error) {
	if s.torrents == nil {
		return nil, library.ErrTorrentServiceUnavailable
	}
	type result struct {
		info *torrent.TorrentInfo
		err  error
	}
	done := make(chan result, 1)
	go func() {
		unlock := s.hashLocks.Lock(infoHash)
		defer unlock()
		info, err := s.torrents.AddTorrent(magnetURI)
		done <- result{info, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		if r.err == nil && r.info == nil {
			return nil, torrent.ErrTorrentNotFound
		}
		return r.info, r.err
	}
}

// addFailed fails the job for an AddTorrent error (or returns ctx's error
// on shutdown, leaving the job to recovery).
func (s *JobService) addFailed(ctx context.Context, job *library.AssignmentJob, err error) error {
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, torrent.ErrMetadataTimeout):
		return s.fail(ctx, job, failMetadataTimeout)
	case errors.Is(err, library.ErrTorrentServiceUnavailable):
		return s.fail(ctx, job, failNoTorrents)
	case errors.Is(err, torrent.ErrInvalidMagnet):
		return s.fail(ctx, job, failInvalidMagnet)
	}
	s.log.Warn("Failed to add a torrent", "job_id", job.ID, "error", err)
	return s.fail(ctx, job, failTorrent)
}

// failOr fails the job when the show is gone, else returns err.
func (s *JobService) failOr(ctx context.Context, job *library.AssignmentJob, err error) error {
	if err != nil {
		return err
	}
	return s.fail(ctx, job, failTitleGone)
}

// refreshShow refreshes the show's metadata from TMDB; failures are logged
// and the stored metadata is used.
func (s *JobService) refreshShow(ctx context.Context, show *library.Show) bool {
	if s.refresher == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, refreshBudget)
	defer cancel()
	if _, err := s.refresher.RefreshShow(ctx, show.ID); err != nil {
		s.log.Warn("Metadata refresh failed; matching against the stored episodes", "show_id", show.ID, "error", err)
		return false
	}
	return true
}

func (s *JobService) reloadShow(ctx context.Context, id int64) (*library.Show, error) {
	return s.shows.GetWithSeasonsAndEpisodesContext(ctx, id)
}

// catalogExtras reads the show's alternative titles and its season-like
// episode groups from TMDB (cached). Whatever fails is left out.
func (s *JobService) catalogExtras(ctx context.Context, show *library.Show) CatalogExtras {
	var extras CatalogExtras
	if s.tmdb == nil || show.TMDBID == 0 {
		return extras
	}
	ctx, cancel := context.WithTimeout(ctx, extrasBudget)
	defer cancel()

	titles, err := s.tmdb.ShowAlternativeTitles(ctx, show.TMDBID)
	if err != nil {
		s.log.Debug("No alternative titles", "show_id", show.ID, "error", err)
	}
	extras.AlternativeTitles = titles

	var ids []string
	if show.EpisodeGroupID != "" {
		ids = append(ids, show.EpisodeGroupID)
	}
	summaries, err := s.tmdb.EpisodeGroups(ctx, show.TMDBID)
	if err != nil {
		s.log.Debug("No episode groups", "show_id", show.ID, "error", err)
	}
	for _, g := range summaries {
		if seasonLikeGroup(g.Type) && g.ID != show.EpisodeGroupID {
			ids = append(ids, g.ID)
		}
	}
	for _, id := range ids {
		if len(extras.EpisodeGroups) == maxEpisodeGroups {
			break
		}
		g, err := s.tmdb.EpisodeGroup(ctx, id)
		if err != nil {
			s.log.Debug("Episode group unavailable", "show_id", show.ID, "group", id, "error", err)
			continue
		}
		extras.EpisodeGroups = append(extras.EpisodeGroups, g)
	}
	return extras
}

// seasonLikeGroup reports episode group types that split a show into
// seasons: absolute numbering and story arcs don't.
func seasonLikeGroup(t int) bool {
	switch t {
	case tmdb.GroupOriginalAirDate, tmdb.GroupDVD, tmdb.GroupDigital, tmdb.GroupProduction, tmdb.GroupTV:
		return true
	}
	return false
}

// resolveFirst runs the first AI run on the unsure files and merges its
// answers into the proposals nobody has touched. Groups are set from the
// merged answer: the first run is part of classification. It reports
// whether the run succeeded (and the job is still resolving).
func (s *JobService) resolveFirst(ctx context.Context, job *library.AssignmentJob, cat *identify.ShowCatalog,
	info *torrent.TorrentInfo, rules []identify.Proposal, scope []string) bool {
	hint := ""
	if job.Hint != nil {
		hint = job.Hint.Text
	}
	rctx, cancel := context.WithTimeout(ctx, resolveBudget)
	out, aiErr := s.resolver.Resolve(rctx, identify.ResolveInput{
		TorrentName: info.Name, Files: info.Files, Show: cat, Rules: rules, Scope: scope, Hint: hint,
	})
	timedOut := errors.Is(rctx.Err(), context.DeadlineExceeded)
	cancel()
	if ctx.Err() != nil {
		return false // shutting down: recovery sets ai_error
	}

	ok := false
	err := s.jobs.Locked(ctx, job.ID, func(t *library.JobTx) error {
		if t.Job.Status != library.JobResolving {
			return nil // discarded meanwhile
		}
		if aiErr != nil {
			s.log.Warn("AI run failed; keeping the rules' proposals", "job_id", job.ID, "error", aiErr)
			t.Job.AIError = msgAIFailed
			if timedOut {
				t.Job.AIError = msgAITimeout
			}
		} else {
			merged := showDrafts(identify.MergeAI(rules, out.Files, cat))
			if err := t.UpdateProposals(mergeFirstRun(t.Proposals, merged)); err != nil {
				return err
			}
			ok = true
		}
		t.Job.Status = settledStatus(listed(t.Proposals))
		return t.SaveJob()
	})
	if err != nil {
		s.log.Error("Failed to store the AI's answers", "job_id", job.ID, "error", err)
		return false
	}
	return ok
}

// mergeFirstRun returns the proposals the first AI run changes: those
// still pending and untouched by the user take the merged answer (kind,
// group, targets, confidence, source and reason, as their suggestion too)
// with the new group's default decision. Linked subtitles follow their
// video, so they're left alone.
func mergeFirstRun(current []*library.AssignmentProposal, merged []library.ProposalDraft) []*library.AssignmentProposal {
	byPath := make(map[string]library.ProposalDraft, len(merged))
	for _, d := range merged {
		byPath[d.FilePath] = d
	}
	var changed []*library.AssignmentProposal
	for _, p := range current {
		d, ok := byPath[p.FilePath]
		if !ok || !p.Listed() || p.Decision != library.DecisionPending || p.UserDecided {
			continue
		}
		if d.Kind == p.Kind && d.Group == p.Group && d.Source == p.Source && d.Reason == p.Reason &&
			d.Confidence == p.Confidence && equalIDs(d.EpisodeIDs, p.EpisodeIDs) {
			continue
		}
		next := *p
		next.Kind, next.Group, next.ExtraType = d.Kind, d.Group, d.ExtraType
		next.EpisodeIDs, next.Confidence, next.Source, next.Reason = d.EpisodeIDs, d.Confidence, d.Source, d.Reason
		next.SuggestedEpisodeIDs, next.SuggestedConfidence = d.EpisodeIDs, d.Confidence
		next.SuggestedSource, next.SuggestedReason = d.Source, d.Reason
		next.Decision = d.Decision
		changed = append(changed, &next)
	}
	return changed
}

// pickMovie accepts the movie candidate at path (the torrent's path, or
// the path inside the torrent's folder) and ignores the other candidates.
// It returns the changed proposals; none when no candidate is at path.
func pickMovie(ps []*library.AssignmentProposal, path string) []*library.AssignmentProposal {
	var pick *library.AssignmentProposal
	for _, p := range ps {
		if p.Kind == library.KindMovie && (p.FilePath == path || hasPathSuffix(p.FilePath, path)) {
			pick = p
			break
		}
	}
	if pick == nil {
		return nil
	}
	var changed []*library.AssignmentProposal
	for _, p := range ps {
		if p.Kind != library.KindMovie {
			continue
		}
		decision := library.DecisionIgnored
		if p == pick {
			decision = library.DecisionAccepted
		}
		if p.Decision == decision {
			continue
		}
		next := *p
		next.Decision = decision
		next.UserDecided = decision != p.Group.DefaultDecision()
		changed = append(changed, &next)
	}
	return changed
}

func hasPathSuffix(full, suffix string) bool {
	return len(full) > len(suffix) && full[len(full)-len(suffix)-1] == '/' && full[len(full)-len(suffix):] == suffix
}

// metadataStale reports a show never synced, or synced over a day ago.
func metadataStale(show *library.Show, now time.Time) bool {
	return show.MetadataUpdatedAt == nil || now.Sub(*show.MetadataUpdatedAt) > metadataStaleAfter
}

// refreshDue reports a show not refreshed within the cooldown.
func refreshDue(show *library.Show, now time.Time) bool {
	return show.MetadataUpdatedAt == nil || now.Sub(*show.MetadataUpdatedAt) > refreshCooldown
}

// unmatchedVideos reports a video the rules couldn't place: maybe an
// episode past the last one the stored metadata knows.
func unmatchedVideos(props []identify.Proposal) bool {
	for _, p := range props {
		if isMainVideo(p) && !p.HasTarget() {
			return true
		}
	}
	return false
}

// unsurePaths lists the files the AI should look at: videos that aren't
// extras, without a high-confidence answer or in a conflict (every one
// with classifier_mode ai_always).
func unsurePaths(props []identify.Proposal, mode library.ClassifierMode) []string {
	conflicts := identify.Conflicts(props)
	var out []string
	for i, p := range props {
		if !isMainVideo(p) {
			continue
		}
		if mode == library.ClassifierAIAlways || p.Confidence != identify.ConfidenceHigh || len(conflicts[i]) > 0 {
			out = append(out, p.Path)
		}
	}
	return out
}

func isMainVideo(p identify.Proposal) bool {
	switch p.Kind {
	case identify.KindEpisode, identify.KindSpecial, identify.KindUnknown:
		return identify.ClassOf(p.Path) == identify.ClassVideo
	}
	return false
}

func hasVideo(files []identify.TorrentFile) bool {
	for _, f := range files {
		if identify.ClassOf(f.Path) == identify.ClassVideo {
			return true
		}
	}
	return false
}

func capHints(hints []string) []string {
	if len(hints) > maxSuggestedHints {
		return hints[:maxSuggestedHints]
	}
	return hints
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// keyedMutex is a mutex per key (an info hash).
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	sync.Mutex
	refs int
}

// Lock locks key and returns its unlock.
func (k *keyedMutex) Lock(key string) (unlock func()) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedLock{}
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()

	l.Lock()
	return func() {
		l.Unlock()
		k.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}
