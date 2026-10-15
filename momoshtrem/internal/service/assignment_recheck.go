package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// RecheckScope is what a re-check looks at (POST
// /assignment-jobs/{id}/reclassify).
type RecheckScope string

const (
	// RecheckUnresolved re-checks the files the user hasn't decided that
	// are pending (unmatched ones included) or in a conflict.
	RecheckUnresolved RecheckScope = "unresolved"
	// RecheckAll re-checks every file the user hasn't decided.
	RecheckAll RecheckScope = "all"
)

// Errors the API maps to status codes.
var (
	// ErrAIUnavailable: the job can't be re-checked by the AI (503): no
	// key, the classifier off, a rules_only show, or a movie job.
	ErrAIUnavailable = errors.New("AI isn't available for this job")
	// ErrInvalidRecheck is a re-check with an unknown scope (400).
	ErrInvalidRecheck = errors.New(`the scope is "unresolved" or "all"`)
)

// ReclassifyRequest is a re-check: its scope and the user's hint, which
// replaces the job's text hint (its season and episode stay).
type ReclassifyRequest struct {
	Scope RecheckScope
	Hint  string
}

// Reclassify starts a re-check with the AI and returns once the job is in
// resolving: the AI runs in the background, then the status is recomputed.
//   - Starting it clears ai_error and every proposal's updated and hinted.
//   - The AI looks at the listed episode files the user hasn't decided:
//     pending or conflicting ones (unresolved), or all of them. Extras and
//     subtitles keep the rules' reading: a re-check never changes a
//     proposal's kind, group or decision.
//   - A changed proposal gets the new targets (as its suggestion too),
//     confidence, source and reason, with updated (and hinted, with a
//     hint). The rules' high answers are never overridden: one the AI
//     disagrees with keeps its target, and its reason names the AI's
//     alternative.
//   - When the AI fails, ai_error is set and nothing else changes.
//
// It answers a JobConflictError (job_closed, job_working),
// ErrAIUnavailable or ErrInvalidRecheck/ErrInvalidHint.
func (s *JobService) Reclassify(ctx context.Context, id int64, req ReclassifyRequest) error {
	if req.Scope != RecheckUnresolved && req.Scope != RecheckAll {
		return ErrInvalidRecheck
	}
	hint := strings.TrimSpace(req.Hint)
	if len([]rune(hint)) > maxHintText {
		return fmt.Errorf("%w: keep the text under %d characters", ErrInvalidHint, maxHintText)
	}
	settings := s.aiSettings(ctx)

	started := false
	err := s.jobs.Locked(ctx, id, func(t *library.JobTx) error {
		switch {
		case !t.Job.Status.Open():
			return conflict(CodeJobClosed, fmt.Sprintf("This mapping job is already %s.", t.Job.Status))
		case t.Job.Status.Working():
			return conflict(CodeJobWorking, "The files are still being matched.")
		case !s.aiAvailable(settings, t.Job):
			return ErrAIUnavailable
		}
		t.Job.Hint = withHintText(t.Job.Hint, hint)
		t.Job.AIError = ""
		var reset []*library.AssignmentProposal
		for _, p := range t.Proposals {
			if p.Updated || p.Hinted {
				next := *p
				next.Updated, next.Hinted = false, false
				reset = append(reset, &next)
			}
		}
		if err := t.UpdateProposals(reset); err != nil {
			return err
		}
		n := len(recheckScope(t.Proposals, req.Scope))
		t.Job.AIFiles = &n
		if n == 0 {
			t.Job.Status = settledStatus(listed(t.Proposals))
		} else {
			t.Job.Status, started = library.JobResolving, true
		}
		return t.SaveJob()
	})
	if err != nil {
		return err
	}
	s.log.Info("Re-check started", "job_id", id, "scope", req.Scope, "hint", hint != "", "asking_ai", started)
	if started {
		s.wg.Add(1)
		go s.recheck(id, req.Scope, settings)
	}
	return nil
}

// recheck runs a started re-check. A failure the AI isn't to blame for (the
// database) still settles the job, with ai_error, so it doesn't stay in
// resolving; on shutdown, recovery does that.
func (s *JobService) recheck(id int64, scope RecheckScope, settings library.AISettings) {
	defer s.wg.Done()
	ctx := s.runContext()
	err := s.runRecheck(ctx, id, scope, settings)
	if err == nil || ctx.Err() != nil {
		return
	}
	s.log.Error("Re-check failed", "job_id", id, "error", err)
	err = s.jobs.Locked(ctx, id, func(t *library.JobTx) error {
		if t.Job.Status != library.JobResolving {
			return nil
		}
		t.Job.AIError = msgAIFailed
		t.Job.Status = settledStatus(listed(t.Proposals))
		return t.SaveJob()
	})
	if err != nil && !errors.Is(err, library.ErrJobNotFound) {
		s.log.Error("Failed to settle a failed re-check", "job_id", id, "error", err)
	}
}

func (s *JobService) runRecheck(ctx context.Context, id int64, scope RecheckScope, settings library.AISettings) error {
	job, err := s.jobs.Get(ctx, id)
	if errors.Is(err, library.ErrJobNotFound) {
		return nil // gone with its show
	}
	if err != nil {
		return err
	}
	if job.Status != library.JobResolving || job.ShowID == nil {
		return nil
	}
	show, err := s.shows.GetWithSeasonsAndEpisodesContext(ctx, *job.ShowID)
	if err != nil {
		return err
	}
	if show == nil {
		return nil
	}
	cat := ShowCatalog(show, s.catalogExtras(ctx, show))
	byJob, err := s.jobs.Proposals(ctx, []int64{id})
	if err != nil {
		return err
	}
	ps := byJob[id]
	files := make([]identify.TorrentFile, len(ps))
	for i, p := range ps {
		files[i] = identify.TorrentFile{Path: p.FilePath, Size: p.FileSize}
	}
	// The rules, run again (they're pure and cheap), say which answers are
	// theirs at high confidence: those stay protected whatever an earlier
	// re-check did to the stored row.
	fresh := identify.Classify(identify.Input{
		TorrentName: job.TorrentName, Files: files, Show: cat, Hint: identifyHint(job.Hint), Now: s.now(),
	}).Proposals
	rules := currentAnswers(ps, cat, fresh)
	var paths []string
	for _, p := range recheckScope(ps, scope) {
		paths = append(paths, p.FilePath)
	}

	rctx, cancel := context.WithTimeout(ctx, s.resolveTime(id, len(paths)))
	out, aiErr := s.resolver.Resolve(rctx, resolveInput(job, show, cat, job.TorrentName, files, rules, paths, settings))
	timedOut := errors.Is(rctx.Err(), context.DeadlineExceeded)
	cancel()
	if ctx.Err() != nil {
		return nil // shutting down: recovery sets ai_error
	}

	hinted := job.Hint != nil && job.Hint.Text != ""
	return s.jobs.Locked(ctx, id, func(t *library.JobTx) error {
		if t.Job.Status != library.JobResolving {
			return nil // discarded meanwhile
		}
		if aiErr != nil {
			s.log.Warn("Re-check failed", "job_id", id, "answers_kept", len(out.Files), "error", aiErr)
			t.Job.AIError = aiErrorMessage(aiErr, timedOut, len(out.Files) > 0)
		}
		// A run that failed partway keeps the answers it got.
		if aiErr == nil || len(out.Files) > 0 {
			merged, disputes := s.mergeAnswers(id, rules, out.Files, cat)
			disputed := make(map[string]bool, len(disputes))
			for _, d := range disputes {
				disputed[d.Path] = true
			}
			agreed := identify.Agreements(rules, out.Files, cat)
			changed := recheckChanges(t.Proposals, rules, merged, scope, hinted, disputed, agreed)
			if err := t.UpdateProposals(changed); err != nil {
				return err
			}
			s.log.Info("Re-check done", "job_id", id, "asked", len(paths), "answers", len(out.Files),
				"changed", len(changed), "disputed", len(disputes), "cost_usd", out.Usage.CostUSD)
		}
		t.Job.Status = settledStatus(listed(t.Proposals))
		return t.SaveJob()
	})
}

// maxResolveWaves caps the time an AI run gets (15 minutes): a bigger
// scope keeps the answers it got in time, as a partial run.
const maxResolveWaves = 10

// resolveTime is the time an AI run over n files gets: resolveBudget per
// wave of requests (ai.Waves), so a big ai_always pack can finish, up to
// maxResolveWaves waves.
func (s *JobService) resolveTime(jobID int64, n int) time.Duration {
	waves := ai.Waves(n)
	if waves > maxResolveWaves {
		s.log.Warn("AI run capped: answers past the time limit are left out", "job_id", jobID, "files", n,
			"waves", waves, "limit", resolveBudget*maxResolveWaves)
		waves = maxResolveWaves
	}
	return resolveBudget * time.Duration(waves)
}

// resolveInput is what the resolver gets for a job: the files and the
// current answers, the scope, the hint (season, episode and text), the
// cache key's fields, the job (for its usage) and the model setting.
func resolveInput(job *library.AssignmentJob, show *library.Show, cat *identify.ShowCatalog, name string,
	files []identify.TorrentFile, rules []identify.Proposal, scope []string, settings library.AISettings) identify.ResolveInput {
	in := identify.ResolveInput{
		TorrentName: name, Files: files, Show: cat, Rules: rules, Scope: scope,
		Hint: ai.HintText(identifyHint(job.Hint)), InfoHash: job.InfoHash, JobID: job.ID,
		Model: settings.ClassifierModel,
	}
	if show != nil {
		in.ShowID = show.ID
		if show.MetadataUpdatedAt != nil {
			in.MetadataUpdatedAt = *show.MetadataUpdatedAt
		}
	}
	return in
}

// mergeAnswers merges the AI's answers (identify.MergeAI) and sends the
// high rules answers it disagrees with to review
// (identify.FlagDisagreements).
func (s *JobService) mergeAnswers(jobID int64, rules []identify.Proposal, answers []identify.AIFile, cat *identify.ShowCatalog) ([]identify.Proposal, []identify.Dispute) {
	merged, disputes := identify.FlagDisagreements(identify.MergeAI(rules, answers, cat), answers, cat)
	if len(disputes) > 0 {
		s.log.Info("The AI disagrees with high rules answers; sent to review", "job_id", jobID, "files", len(disputes))
	}
	return merged, disputes
}

// aiErrorMessage is the ai_error a failed AI run leaves; partial when it
// got some answers before failing.
func aiErrorMessage(err error, timedOut, partial bool) string {
	switch {
	case partial:
		return msgAIPartial
	case errors.Is(err, ai.ErrBudgetExceeded):
		return msgAIBudget
	case timedOut:
		return msgAITimeout
	}
	return msgAIFailed
}

// withHintText replaces a hint's text, keeping its season and episode.
func withHintText(h *library.JobHint, text string) *library.JobHint {
	var out library.JobHint
	if h != nil {
		out = *h
	}
	out.Text = text
	return normalizeHint(&out)
}

// recheckScope lists the proposals a re-check asks about: listed episode
// files (videos) the user hasn't decided; for unresolved, only those
// pending or in a conflict.
func recheckScope(ps []*library.AssignmentProposal, scope RecheckScope) []*library.AssignmentProposal {
	conflicts := conflictIDs(listed(ps))
	var out []*library.AssignmentProposal
	for _, p := range ps {
		if !p.Listed() || p.UserDecided || p.Kind != library.KindEpisode || identify.ClassOf(p.FilePath) != identify.ClassVideo {
			continue
		}
		if scope == RecheckAll || p.Decision == library.DecisionPending || conflicts[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

// currentAnswers is the job's proposals as the classifier's, for the AI's
// prompt and for MergeAI. fresh is the rules' answer for the same files,
// run again: a proposal they place with high confidence on the targets it
// has is theirs, high and protected (even after an earlier re-check flagged
// it and dropped it to medium). Any other answer isn't protected, so a
// re-check (with the user's hint, say) may replace an earlier AI answer.
func currentAnswers(ps []*library.AssignmentProposal, cat *identify.ShowCatalog, fresh []identify.Proposal) []identify.Proposal {
	episodes := make(map[int64]identify.Episode, len(cat.Episodes))
	for _, ep := range cat.Episodes {
		episodes[ep.ID] = ep
	}
	pathOf := make(map[int64]string, len(ps))
	for _, p := range ps {
		pathOf[p.ID] = p.FilePath
	}
	out := make([]identify.Proposal, len(ps))
	for i, p := range ps {
		r := identify.Proposal{Path: p.FilePath, Size: p.FileSize, Reason: p.Reason, Source: identify.SourceRules,
			Confidence: identify.Confidence(p.Confidence)}
		if p.Source == library.SourceAI {
			r.Source = identify.SourceAI
		}
		switch p.Kind {
		case library.KindExtra:
			r.Kind, r.ExtraType, r.Confidence = identify.KindExtra, identify.ExtraType(p.ExtraType), identify.ConfidenceNone
			if r.ExtraType == identify.ExtraSample {
				r.Kind = identify.KindSample
			}
		case library.KindSubtitle:
			r.Kind = identify.KindSubtitle
			if p.LinkedProposalID != nil {
				r.LinkedVideo = pathOf[*p.LinkedProposalID]
			}
		default:
			for _, id := range p.EpisodeIDs {
				if ep, ok := episodes[id]; ok {
					r.Targets = append(r.Targets, identify.Target{EpisodeID: id, Season: ep.Season, Episode: ep.Number})
				}
			}
			switch {
			case len(r.Targets) == 0:
				r.Kind, r.Confidence = identify.KindUnknown, identify.ConfidenceNone
			case allSpecials(r.Targets):
				r.Kind = identify.KindSpecial
			default:
				r.Kind = identify.KindEpisode
			}
			if p.Source != library.SourceRules && r.Confidence == identify.ConfidenceHigh {
				r.Confidence = identify.ConfidenceMedium
			}
			if i < len(fresh) && rulesHigh(fresh[i], p) {
				r.Confidence, r.Source, r.Reason = identify.ConfidenceHigh, identify.SourceRules, fresh[i].Reason
			}
		}
		out[i] = r
	}
	return out
}

// rulesHigh reports whether the rules' fresh answer for a file is high
// confidence on the proposal's own targets.
func rulesHigh(r identify.Proposal, p *library.AssignmentProposal) bool {
	return r.Path == p.FilePath && r.Confidence == identify.ConfidenceHigh &&
		(r.Kind == identify.KindEpisode || r.Kind == identify.KindSpecial) && equalIDs(targetIDs(r.Targets), p.EpisodeIDs)
}

// recheckChanges returns the proposals a re-check changes: those still in
// scope (the user may have decided some meanwhile) that the AI's answer
// touched (the merged answer differs from the one it was shown) and that
// end up different from what's stored. Kind and group stay, and an
// accepted proposal never loses its target. The decision stays too, except
// that a disputed proposal on its group's default acceptance goes back to
// pending, as on the first run, so the job waits for the user.
//
// When the AI agrees with a high rules answer an earlier re-check flagged
// (agreed), the flag is lifted: the rules' confidence and reason come back,
// and the decision stays (pending) for the user.
func recheckChanges(current []*library.AssignmentProposal, before, after []identify.Proposal, scope RecheckScope,
	hinted bool, disputed, agreed map[string]bool) []*library.AssignmentProposal {
	type pair struct{ before, after identify.Proposal }
	byPath := make(map[string]pair, len(before))
	for i := range before {
		byPath[before[i].Path] = pair{before[i], after[i]}
	}
	var changed []*library.AssignmentProposal
	for _, p := range recheckScope(current, scope) {
		pr, ok := byPath[p.FilePath]
		if !ok || (sameAnswer(pr.before, pr.after) && !agreed[p.FilePath]) {
			continue
		}
		a := pr.after
		ids := targetIDs(a.Targets)
		if len(ids) == 0 && p.Decision == library.DecisionAccepted {
			continue
		}
		conf := string(a.Confidence)
		if len(ids) == 0 {
			conf = ""
		}
		source, reason := sourceOf(a.Source), clipReason(a.Reason)
		if equalIDs(ids, p.EpisodeIDs) && conf == p.Confidence && source == p.Source && reason == p.Reason {
			continue // the AI said what's already there
		}
		next := *p
		next.EpisodeIDs, next.Confidence, next.Source, next.Reason = ids, conf, source, reason
		next.SuggestedEpisodeIDs, next.SuggestedConfidence = ids, conf
		next.SuggestedSource, next.SuggestedReason = next.Source, next.Reason
		next.Updated, next.Hinted = true, hinted
		if disputed[p.FilePath] && p.Decision == library.DecisionAccepted && p.Decision == p.Group.DefaultDecision() {
			next.Decision = library.DecisionPending
		}
		changed = append(changed, &next)
	}
	return changed
}

func sameAnswer(a, b identify.Proposal) bool {
	return a.Kind == b.Kind && a.Confidence == b.Confidence && a.Source == b.Source && a.Reason == b.Reason &&
		equalIDs(targetIDs(a.Targets), targetIDs(b.Targets))
}
