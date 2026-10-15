package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// scriptedResolver answers with out (or err), after gate is closed when it
// has one, recording its inputs.
type scriptedResolver struct {
	mu    sync.Mutex
	out   identify.ResolveOutput
	err   error
	gate  chan struct{}
	calls []identify.ResolveInput
}

func (r *scriptedResolver) Resolve(ctx context.Context, in identify.ResolveInput) (identify.ResolveOutput, error) {
	r.mu.Lock()
	r.calls = append(r.calls, in)
	out, err, gate := r.out, r.err, r.gate
	r.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return identify.ResolveOutput{}, ctx.Err()
		}
	}
	return out, err
}

func (r *scriptedResolver) answer(files ...identify.AIFile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.out, r.err = identify.ResolveOutput{Files: files}, nil
}

func (r *scriptedResolver) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.out, r.err = identify.ResolveOutput{}, err
}

func (r *scriptedResolver) last() identify.ResolveInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1]
}

func aiEpisode(path string, season, episode int, conf identify.Confidence, reason string) identify.AIFile {
	return identify.AIFile{Path: path, Kind: identify.KindEpisode, Season: &season, Episodes: []int{episode}, Confidence: conf, Reason: reason}
}

func targetLabels(p *ProposalView) string {
	var labels []string
	for _, t := range p.Targets {
		labels = append(labels, se(t.Season, t.Episode))
	}
	return strings.Join(labels, "+")
}

// In ai_always the AI sees the files the rules matched with high
// confidence too. One it disagrees with keeps the rules' target but goes to
// review, so the job doesn't apply on its own; one it agrees with stays.
// The resolver gets the cache key's fields, the job and the model setting.
func TestJob_AIAlwaysDisputesGoToReview(t *testing.T) {
	hash := strings.Repeat("a1", 20)
	root := "Always.Show.S01.1080p.WEB-GRP"
	one, two := root+"/Always.Show.S01E01.1080p.WEB-GRP.mkv", root+"/Always.Show.S01E02.1080p.WEB-GRP.mkv"
	e := newJobEnv(t, info(hash, root, file("Always.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Always.Show.S01E02.1080p.WEB-GRP.mkv", gb)))
	showID, _ := e.show("Always Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	if _, err := e.db.Exec(`UPDATE shows SET classifier_mode = 'ai_always' WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	e.setAI(`{"classifier_model": "claude-sonnet-5-5"}`)
	resolver := &scriptedResolver{}
	resolver.answer(
		aiEpisode(one, 1, 3, identify.ConfidenceMedium, "AI: the third, by its runtime"),
		aiEpisode(two, 1, 2, identify.ConfidenceHigh, "S01E02"),
	)
	e.svc = e.service(JobDeps{Resolver: resolver})

	id := e.assignShow(showID, hash, &library.JobHint{Season: intPtr(1), Text: "a WEB pack"})
	d := e.detail(id)
	if d.Job.Status != library.JobNeedsReview {
		t.Fatalf("status = %s, want needs_review (a dispute blocks auto-apply)", d.Job.Status)
	}
	disputed := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv")
	if targetLabels(disputed) != "S01E01" || disputed.Group != "needs_review" || disputed.Decision != "pending" ||
		disputed.Source != "rules" || *disputed.Confidence != "medium" || disputed.Reason != "AI suggests S01E03: the third, by its runtime" {
		t.Errorf("disputed = %s %+v", targetLabels(disputed), disputed)
	}
	agreed := proposalByPath(d, "S01E02.1080p.WEB-GRP.mkv")
	if targetLabels(agreed) != "S01E02" || agreed.Group != "matched" || agreed.Decision != "accepted" || *agreed.Confidence != "high" {
		t.Errorf("agreed = %+v", agreed)
	}

	in := resolver.last()
	job := e.job(id)
	if len(in.Scope) != 2 || in.InfoHash != hash || in.ShowID != showID || in.JobID != id || in.MetadataUpdatedAt.IsZero() ||
		in.Model != "claude-sonnet-5-5" || in.Hint != "The files are from season 1. a WEB pack" || job.ClassifierMode != library.ClassifierAIAlways {
		t.Errorf("resolver input = scope %q hash %q show %d job %d meta %v model %q hint %q", in.Scope, in.InfoHash, in.ShowID, in.JobID,
			in.MetadataUpdatedAt, in.Model, in.Hint)
	}
}

// A run the budget stops leaves the rules' proposals, with ai_error saying
// so.
func TestJob_AIBudgetSpent(t *testing.T) {
	hash := strings.Repeat("b2", 20)
	root := "Budget.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root, file("Budget.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Something Odd.mkv", gb)))
	showID, _ := e.show("Budget Show", [][3]any{{1, 1, "One"}})
	resolver := &scriptedResolver{}
	resolver.fail(fmt.Errorf("%w: $5.00 of $5.00", ai.ErrBudgetExceeded))
	e.svc = e.service(JobDeps{Resolver: resolver})

	d := e.detail(e.assignShow(showID, hash, nil))
	if d.Job.AIError == nil || *d.Job.AIError != msgAIBudget || proposalByPath(d, "Something Odd.mkv").Group != "unmatched" {
		t.Errorf("ai_error = %v", d.Job.AIError)
	}
}

// recheckEnv is a job classified with an unmatched file, a high match and a
// high match the user ignored, ready for re-checks.
type recheckEnv struct {
	*jobEnv
	resolver           *scriptedResolver
	id, showID         int64
	strange, one, two  string
	special, three, ep map[string]int64
}

func newRecheckEnv(t *testing.T) *recheckEnv {
	hash := strings.Repeat("c3", 20)
	root := "Recheck.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Recheck.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Recheck.Show.S01E02.1080p.WEB-GRP.mkv", gb),
		file("Something Strange.mkv", gb)))
	showID, eps := e.show("Recheck Show", [][3]any{{0, 1, "The Mystery Special"}, {1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	resolver := &scriptedResolver{}
	resolver.answer() // the first run answers nothing
	e.svc = e.service(JobDeps{Resolver: resolver})
	id := e.assignShow(showID, hash, &library.JobHint{Season: intPtr(1), Text: "old text"})
	e.decide(id, map[string]library.Decision{"S01E02.1080p.WEB-GRP.mkv": library.DecisionIgnored})
	return &recheckEnv{jobEnv: e, resolver: resolver, id: id, showID: showID, ep: eps,
		strange: root + "/Something Strange.mkv", one: root + "/Recheck.Show.S01E01.1080p.WEB-GRP.mkv",
		two: root + "/Recheck.Show.S01E02.1080p.WEB-GRP.mkv"}
}

// recheck starts a re-check and waits for it.
func (r *recheckEnv) recheck(scope RecheckScope, hint string) *JobDetail {
	r.t.Helper()
	if err := r.svc.Reclassify(r.ctx, r.id, ReclassifyRequest{Scope: scope, Hint: hint}); err != nil {
		r.t.Fatalf("reclassify: %v", err)
	}
	r.svc.Close() // waits for the re-check
	return r.detail(r.id)
}

func TestReclassify(t *testing.T) {
	r := newRecheckEnv(t)
	before := r.detail(r.id)
	if before.Job.Status.Working() || proposalByPath(before, "Something Strange.mkv").Group != "unmatched" {
		t.Fatalf("before = %s, %+v", before.Job.Status, before.Proposals)
	}

	// While the AI works the job is resolving; then its answers land.
	r.resolver.answer(
		identify.AIFile{Path: r.strange, Kind: identify.KindSpecial, Season: intPtr(0), Episodes: []int{1}, Confidence: identify.ConfidenceHigh, Reason: "The special, by its title"},
		aiEpisode(r.one, 1, 3, identify.ConfidenceHigh, "the third"),
		aiEpisode(r.two, 1, 3, identify.ConfidenceHigh, "the third"),
	)
	r.resolver.gate = make(chan struct{})
	if err := r.svc.Reclassify(r.ctx, r.id, ReclassifyRequest{Scope: RecheckUnresolved, Hint: "  the bonus is the special  "}); err != nil {
		t.Fatal(err)
	}
	mid := r.detail(r.id)
	if mid.Job.Status != library.JobResolving || mid.Job.AIError != nil || mid.Job.Progress.AIFiles == nil || *mid.Job.Progress.AIFiles != 1 ||
		mid.Job.Hint == nil || mid.Job.Hint.Text != "the bonus is the special" || mid.Job.Hint.Season == nil || *mid.Job.Hint.Season != 1 {
		t.Errorf("while resolving: %s ai_files %v hint %+v", mid.Job.Status, mid.Job.Progress.AIFiles, mid.Job.Hint)
	}
	if err := r.svc.Reclassify(r.ctx, r.id, ReclassifyRequest{Scope: RecheckAll}); conflictCode(err) != CodeJobWorking {
		t.Errorf("a second re-check while resolving = %v, want job_working", err)
	}
	close(r.resolver.gate)
	r.svc.Close()
	r.resolver.gate = nil
	d := r.detail(r.id)

	strange := proposalByPath(d, "Something Strange.mkv")
	if targetLabels(strange) != "S00E01" || strange.Source != "ai" || *strange.Confidence != "high" || !strange.Updated || !strange.Hinted ||
		strange.Group != "unmatched" || strange.Decision != "pending" || strange.Reason != "The special, by its title" ||
		len(strange.SuggestedTargets) != 1 || strange.SuggestedTargets[0].Season != 0 {
		t.Errorf("re-checked = %s %+v", targetLabels(strange), strange)
	}
	if one := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv"); targetLabels(one) != "S01E01" || one.Updated || one.Source != "rules" {
		t.Errorf("accepted, out of scope = %+v", one)
	}
	if two := proposalByPath(d, "S01E02.1080p.WEB-GRP.mkv"); targetLabels(two) != "S01E02" || two.Updated || two.Decision != "ignored" {
		t.Errorf("user decided = %+v", two)
	}
	if d.Job.Status != library.JobNeedsReview || d.Job.AIError != nil {
		t.Errorf("after = %s ai_error %v", d.Job.Status, d.Job.AIError)
	}
	in := r.resolver.last()
	if len(in.Scope) != 1 || in.Scope[0] != r.strange || in.JobID != r.id || in.ShowID != r.showID || in.Model != library.DefaultClassifierModel ||
		!strings.Contains(in.Hint, "the bonus is the special") || len(in.Rules) != 3 || len(in.Files) != 3 {
		t.Errorf("resolver input = scope %q job %d model %q hint %q rules %d", in.Scope, in.JobID, in.Model, in.Hint, len(in.Rules))
	}

	// All: the accepted high match is asked about too. The AI disagrees
	// with it: it keeps its target (and suggestion, so Undo restores it),
	// its reason names the AI's pick, and it goes back to pending so the
	// job waits for the user. The special is already what the AI says: not
	// updated. The hint's text goes; updated and hinted reset first.
	d = r.recheck(RecheckAll, "")
	one := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv")
	if targetLabels(one) != "S01E01" || one.Decision != "pending" || one.UserDecided || one.Source != "rules" || *one.Confidence != "medium" ||
		one.Reason != "AI suggests S01E03: the third" || !one.Updated || one.Hinted ||
		len(one.SuggestedTargets) != 1 || one.SuggestedTargets[0].Episode != 1 || one.Group != "matched" {
		t.Errorf("disputed = %+v", one)
	}
	if d.Job.Status != library.JobNeedsReview {
		t.Errorf("with a dispute the job is %s, want needs_review", d.Job.Status)
	}
	if strange := proposalByPath(d, "Something Strange.mkv"); strange.Updated || strange.Hinted || targetLabels(strange) != "S00E01" {
		t.Errorf("unchanged by the second re-check = %+v", strange)
	}
	if two := proposalByPath(d, "S01E02.1080p.WEB-GRP.mkv"); two.Updated || targetLabels(two) != "S01E02" {
		t.Errorf("user decided = %+v", two)
	}
	if d.Job.Hint == nil || d.Job.Hint.Text != "" || *d.Job.Hint.Season != 1 || *d.Job.Progress.AIFiles != 2 {
		t.Errorf("hint %+v ai_files %v", d.Job.Hint, d.Job.Progress.AIFiles)
	}

	// The AI fails: ai_error, and nothing changes.
	r.resolver.fail(errors.New("overloaded"))
	d = r.recheck(RecheckAll, "try again")
	if d.Job.AIError == nil || *d.Job.AIError != msgAIFailed || d.Job.Status != library.JobNeedsReview {
		t.Errorf("failed = %s ai_error %v", d.Job.Status, d.Job.AIError)
	}
	for _, p := range d.Proposals {
		if p.Updated || p.Hinted {
			t.Errorf("a failed re-check changed %+v", p)
		}
	}
	if one := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv"); targetLabels(one) != "S01E01" || *one.Confidence != "medium" {
		t.Errorf("after a failure = %+v", one)
	}

	// Nothing in scope (the pending files accepted): the job settles at
	// once, without asking.
	calls := len(r.resolver.calls)
	r.decide(r.id, map[string]library.Decision{"Something Strange.mkv": library.DecisionAccepted, "S01E01.1080p.WEB-GRP.mkv": library.DecisionAccepted})
	if err := r.svc.Reclassify(r.ctx, r.id, ReclassifyRequest{Scope: RecheckUnresolved}); err != nil {
		t.Fatal(err)
	}
	r.svc.Close()
	if d := r.detail(r.id); d.Job.Status.Working() || len(r.resolver.calls) != calls || *d.Job.Progress.AIFiles != 0 {
		t.Errorf("empty scope: %s after %d calls", d.Job.Status, len(r.resolver.calls)-calls)
	}
}

// A second re-check never overrides a high rules answer the first one
// flagged (and dropped to medium): the rules, run again, still own it.
func TestReclassify_SecondRecheckKeepsFlaggedHigh(t *testing.T) {
	r := newRecheckEnv(t)
	r.resolver.answer(aiEpisode(r.one, 1, 3, identify.ConfidenceHigh, "the third"))
	for i, label := range []string{"first", "second", "third"} {
		d := r.recheck(RecheckAll, "")
		one := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv")
		if targetLabels(one) != "S01E01" || one.Source != "rules" || *one.Confidence != "medium" || one.Decision != "pending" ||
			one.Reason != "AI suggests S01E03: the third" || d.Job.Status != library.JobNeedsReview {
			t.Errorf("%s re-check: %s %s %s %s %q, job %s", label, targetLabels(one), *one.Confidence, one.Source, one.Decision, one.Reason, d.Job.Status)
		}
		// Only the first changes it; the others find it flagged already.
		if one.Updated != (i == 0) {
			t.Errorf("%s re-check: updated %v", label, one.Updated)
		}
	}
	in := r.resolver.last()
	for _, p := range in.Rules {
		if strings.HasSuffix(p.Path, "S01E01.1080p.WEB-GRP.mkv") && (p.Confidence != identify.ConfidenceHigh || p.Source != identify.SourceRules) {
			t.Errorf("the AI was shown the flagged rules answer as %s %s", p.Confidence, p.Source)
		}
	}
}

// A PATCH while a re-check is resolving survives the AI's answer.
func TestReclassify_DecisionDuringResolving(t *testing.T) {
	r := newRecheckEnv(t)
	r.resolver.answer(identify.AIFile{Path: r.strange, Kind: identify.KindSpecial, Season: intPtr(0), Episodes: []int{1},
		Confidence: identify.ConfidenceHigh, Reason: "The special"})
	r.resolver.gate = make(chan struct{})
	if err := r.svc.Reclassify(r.ctx, r.id, ReclassifyRequest{Scope: RecheckUnresolved}); err != nil {
		t.Fatal(err)
	}
	// As slice 4's PATCH does during resolving: the decision, not the
	// status.
	err := r.jobs.Locked(r.ctx, r.id, func(tx *library.JobTx) error {
		for _, p := range tx.Proposals {
			if strings.HasSuffix(p.FilePath, "Something Strange.mkv") {
				next := *p
				next.Decision, next.UserDecided = library.DecisionIgnored, true
				return tx.UpdateProposals([]*library.AssignmentProposal{&next})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	close(r.resolver.gate)
	r.svc.Close()
	d := r.detail(r.id)
	if s := proposalByPath(d, "Something Strange.mkv"); s.Updated || targetLabels(s) != "" || s.Decision != "ignored" {
		t.Errorf("the AI overwrote a decision made while it worked: %+v", s)
	}
}

// A run that fails partway (one of its requests) keeps the answers it got,
// with ai_error saying the rest are unchanged.
func TestReclassify_PartialAnswers(t *testing.T) {
	r := newRecheckEnv(t)
	r.resolver.mu.Lock()
	r.resolver.out = identify.ResolveOutput{Files: []identify.AIFile{{Path: r.strange, Kind: identify.KindSpecial, Season: intPtr(0),
		Episodes: []int{1}, Confidence: identify.ConfidenceMedium, Reason: "The special"}}}
	r.resolver.err = fmt.Errorf("1 of 2 requests failed: %w", ai.ErrUnavailable)
	r.resolver.mu.Unlock()
	d := r.recheck(RecheckUnresolved, "")
	if s := proposalByPath(d, "Something Strange.mkv"); targetLabels(s) != "S00E01" || !s.Updated {
		t.Errorf("the answer it got = %+v", s)
	}
	if d.Job.AIError == nil || *d.Job.AIError != msgAIPartial || d.Job.Status.Working() {
		t.Errorf("ai_error %v, status %s", d.Job.AIError, d.Job.Status)
	}
}

// The first run keeps partial answers too, and doesn't apply on its own.
func TestJob_FirstRunPartialAnswers(t *testing.T) {
	hash := strings.Repeat("d4", 20)
	root := "Partial.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root, file("Partial.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Something Odd.mkv", gb)))
	showID, _ := e.show("Partial Show", [][3]any{{0, 1, "The Odd Special"}, {1, 1, "One"}})
	resolver := &scriptedResolver{}
	resolver.out = identify.ResolveOutput{Files: []identify.AIFile{{Path: root + "/Something Odd.mkv", Kind: identify.KindSpecial,
		Season: intPtr(0), Episodes: []int{1}, Confidence: identify.ConfidenceHigh, Reason: "The special"}}}
	resolver.err = fmt.Errorf("1 of 2 requests failed: %w", ai.ErrUnavailable)
	e.svc = e.service(JobDeps{Resolver: resolver})

	d := e.detail(e.assignShow(showID, hash, nil))
	if odd := proposalByPath(d, "Something Odd.mkv"); targetLabels(odd) != "S00E01" || odd.Source != "ai" {
		t.Errorf("the answer it got = %+v", odd)
	}
	if d.Job.Status == library.JobApplied || d.Job.AIError == nil || *d.Job.AIError != msgAIPartial {
		t.Errorf("status %s ai_error %v: a partial run doesn't apply on its own", d.Job.Status, d.Job.AIError)
	}
}

// An AI run gets 90 seconds a wave of requests, so a big pack finishes.
func TestResolveTime(t *testing.T) {
	for _, tt := range []struct {
		files int
		want  time.Duration
	}{{1, 90 * time.Second}, {240, 90 * time.Second}, {241, 180 * time.Second}, {1000, 450 * time.Second}} {
		if got := resolveTime(tt.files); got != tt.want {
			t.Errorf("resolveTime(%d) = %v, want %v", tt.files, got, tt.want)
		}
	}
}

func TestReclassify_Refused(t *testing.T) {
	r := newRecheckEnv(t)
	ctx := context.Background()

	if err := r.svc.Reclassify(ctx, r.id, ReclassifyRequest{Scope: "some"}); !errors.Is(err, ErrInvalidRecheck) {
		t.Errorf("an unknown scope = %v", err)
	}
	if err := r.svc.Reclassify(ctx, r.id, ReclassifyRequest{Scope: RecheckAll, Hint: strings.Repeat("x", maxHintText+1)}); !errors.Is(err, ErrInvalidHint) {
		t.Errorf("a long hint = %v", err)
	}
	if err := r.svc.Reclassify(ctx, 999_999, ReclassifyRequest{Scope: RecheckAll}); !errors.Is(err, library.ErrJobNotFound) {
		t.Errorf("an unknown job = %v", err)
	}

	// No AI: no resolver, the classifier off, or a rules_only show.
	noAI := r.service(JobDeps{})
	if err := noAI.Reclassify(ctx, r.id, ReclassifyRequest{Scope: RecheckAll}); !errors.Is(err, ErrAIUnavailable) {
		t.Errorf("without a resolver = %v", err)
	}
	r.setAI(`{"classifier_enabled": false}`)
	if err := r.svc.Reclassify(ctx, r.id, ReclassifyRequest{Scope: RecheckAll}); !errors.Is(err, ErrAIUnavailable) {
		t.Errorf("classifier off = %v", err)
	}
	r.setAI(`{}`)
	if _, err := r.db.Exec(`UPDATE shows SET classifier_mode = 'rules_only' WHERE id = $1`, r.showID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Reclassify(ctx, r.id, ReclassifyRequest{Scope: RecheckAll}); !errors.Is(err, ErrAIUnavailable) {
		t.Errorf("rules_only = %v", err)
	}
	if _, err := r.db.Exec(`UPDATE shows SET classifier_mode = 'auto' WHERE id = $1`, r.showID); err != nil {
		t.Fatal(err)
	}

	// A working job, then a closed one.
	if _, err := r.db.Exec(`UPDATE assignment_jobs SET status = 'classifying' WHERE id = $1`, r.id); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Reclassify(ctx, r.id, ReclassifyRequest{Scope: RecheckAll}); conflictCode(err) != CodeJobWorking {
		t.Errorf("working = %v", err)
	}
	if _, err := r.db.Exec(`UPDATE assignment_jobs SET status = 'needs_review' WHERE id = $1`, r.id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Discard(ctx, r.id); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Reclassify(ctx, r.id, ReclassifyRequest{Scope: RecheckAll}); conflictCode(err) != CodeJobClosed {
		t.Errorf("closed = %v", err)
	}
}

// An interrupted re-check (a restart) settles with ai_error, as an
// interrupted first run does.
func TestReclassify_InterruptedByShutdown(t *testing.T) {
	r := newRecheckEnv(t)
	r.resolver.gate = make(chan struct{}) // never answers
	r.svc.Start()
	if err := r.svc.Reclassify(r.ctx, r.id, ReclassifyRequest{Scope: RecheckUnresolved}); err != nil {
		t.Fatal(err)
	}
	r.svc.Close() // cancels the re-check: the job stays resolving for recovery
	if st := r.job(r.id).Status; st != library.JobResolving {
		t.Fatalf("after shutdown = %s", st)
	}
	r.svc = r.service(JobDeps{Resolver: r.resolver})
	r.start()
	if d := r.detail(r.id); d.Job.Status.Working() || d.Job.AIError == nil || *d.Job.AIError != msgAIInterrupted {
		t.Errorf("recovered = %s ai_error %v", d.Job.Status, d.Job.AIError)
	}
}

// The resolver's Postgres side: answers cached, usage recorded against the
// job (cache writes counted as input), and the monthly budget.
func TestClassifierAIStore(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	settings := library.NewSettingsRepository(db)
	s := NewClassifierAIStore(library.NewAIStore(db), settings)

	if err := s.Put(ctx, "classifier:k", []byte(`[]`), 0); err != nil {
		t.Fatal(err)
	}
	if payload, ok, err := s.Get(ctx, "classifier:k"); !ok || err != nil || string(payload) != "[]" {
		t.Errorf("cache = %s %v %v", payload, ok, err)
	}

	if err := s.Check(ctx); err != nil {
		t.Errorf("no budget: %v", err)
	}
	if err := s.Record(ctx, identify.Usage{Model: "claude-haiku-5-5", InputTokens: 100, CacheWriteTokens: 50, CacheReadTokens: 2000,
		OutputTokens: 30, CostUSD: 0.6}, 7); err != nil {
		t.Fatal(err)
	}
	var input, read, out int
	var ref int64
	if err := db.QueryRow(`SELECT input_tokens, cache_read_tokens, output_tokens, ref_id FROM ai_usage WHERE feature = 'classifier'`).
		Scan(&input, &read, &out, &ref); err != nil {
		t.Fatal(err)
	}
	if input != 150 || read != 2000 || out != 30 || ref != 7 {
		t.Errorf("row = in %d read %d out %d ref %d", input, read, out, ref)
	}

	if err := settings.Set(ctx, library.AISettingsKey, []byte(`{"monthly_budget_usd": 1}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); err != nil {
		t.Errorf("under budget: %v", err)
	}
	if err := settings.Set(ctx, library.AISettingsKey, []byte(`{"monthly_budget_usd": 0.5}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx); !errors.Is(err, ai.ErrBudgetExceeded) {
		t.Errorf("over budget: %v", err)
	}
}

func intPtr(n int) *int { return &n }
