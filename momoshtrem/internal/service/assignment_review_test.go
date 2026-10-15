package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// PATCH (PLAN §6 "Assignment"), against a real job in Postgres.

func decision(d library.Decision) *library.Decision { return &d }

func change(id int64, d library.Decision, episodes ...int64) ProposalChange {
	c := ProposalChange{ID: id, EpisodeIDs: episodes}
	if d != "" {
		c.Decision = decision(d)
	}
	return c
}

func (e *jobEnv) patch(id int64, changes ...ProposalChange) *JobDetail {
	e.t.Helper()
	d, err := e.svc.PatchProposals(e.ctx, id, changes)
	if err != nil {
		e.t.Fatalf("patch %+v: %v", changes, err)
	}
	return d
}

func validationMessage(err error) string {
	var v *ValidationError
	if errors.As(err, &v) {
		return v.Message
	}
	return ""
}

// A ready job's three files: picking another episode is a manual pick,
// picking the suggestion again restores it, and user_decided follows
// whether the proposal is back where it started.
func TestPatch_ManualPickAndUndo(t *testing.T) {
	hash := strings.Repeat("3", 40)
	root := "Pick.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Pick.Show.S01E01.1080p.WEB-GRP.mkv", gb),
		file("Pick.Show.S01E02.1080p.WEB-GRP.mkv", gb),
		file("Pick.Show.S01E03.1080p.WEB-GRP.mkv", gb),
	))
	showID, eps := e.show("Pick Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}, {1, 4, "Four"}, {0, 1, "Special"}})
	e.setAI(`{"auto_apply_high_confidence": false}`)
	id := e.assignShow(showID, hash, nil)
	d := e.detail(id)
	if d.Job.Status != library.JobReady {
		t.Fatalf("status = %s", d.Job.Status)
	}
	e1, e2, e3 := proposalByPath(d, "E01.1080p.WEB-GRP.mkv"), proposalByPath(d, "E02.1080p.WEB-GRP.mkv"), proposalByPath(d, "E03.1080p.WEB-GRP.mkv")
	suggestedReason := e3.Reason

	// Another episode: a manual pick; it stays accepted.
	d = e.patch(id, change(e3.ID, "", eps["S01E04"]))
	p := proposalByPath(d, "E03.1080p.WEB-GRP.mkv")
	if p.Source != "manual" || p.Confidence != nil || p.Reason != "You picked this episode" || !p.UserDecided ||
		p.Decision != "accepted" || len(p.Targets) != 1 || p.Targets[0].Episode != 4 ||
		len(p.SuggestedTargets) != 1 || p.SuggestedTargets[0].Episode != 3 || p.Group != "matched" {
		t.Errorf("manual pick = %+v", p)
	}
	if d.Job.Status != library.JobReady {
		t.Errorf("status = %s, want ready", d.Job.Status)
	}

	// The suggestion again (Undo): its confidence, source and reason are back.
	d = e.patch(id, change(e3.ID, "", eps["S01E03"]))
	p = proposalByPath(d, "E03.1080p.WEB-GRP.mkv")
	if p.Source != "rules" || p.Confidence == nil || *p.Confidence != "high" || p.Reason != suggestedReason || p.UserDecided {
		t.Errorf("restored = %+v", p)
	}

	// Un-accepting makes it pending: needs_review until it's accepted again.
	d = e.patch(id, change(e2.ID, library.DecisionPending))
	if p := proposalByPath(d, "E02.1080p.WEB-GRP.mkv"); !p.UserDecided || p.Decision != "pending" ||
		d.Job.Status != library.JobNeedsReview || d.Job.Counts.Pending != 1 {
		t.Errorf("pending = %+v, job %s %+v", p, d.Job.Status, d.Job.Counts)
	}
	d = e.patch(id, change(e2.ID, library.DecisionAccepted))
	if p := proposalByPath(d, "E02.1080p.WEB-GRP.mkv"); p.UserDecided || d.Job.Status != library.JobReady {
		t.Errorf("accepted again = %+v, job %s", p, d.Job.Status)
	}

	// Two files on E02 are a conflict; a multi-episode pick keeps its order
	// (repeats dropped) and reaches into Specials.
	d = e.patch(id, change(e1.ID, "", eps["S01E02"]))
	if d.Job.Status != library.JobNeedsReview || d.Job.Counts.Conflicts != 2 {
		t.Errorf("conflict: %s %+v", d.Job.Status, d.Job.Counts)
	}
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodeConflictingProposals {
		t.Errorf("apply with a conflict: %v", err)
	}
	d = e.patch(id, change(e1.ID, "", eps["S01E01"], eps["S00E01"], eps["S01E01"]))
	p = proposalByPath(d, "E01.1080p.WEB-GRP.mkv")
	if len(p.Targets) != 2 || p.Targets[0].Episode != 1 || p.Targets[1].Season != 0 || p.Source != "manual" ||
		d.Job.Status != library.JobReady || d.Job.Counts.Conflicts != 0 {
		t.Errorf("multi pick = %+v, job %s %+v", p.Targets, d.Job.Status, d.Job.Counts)
	}

	// Both fields at once, in one item, then undone in one batch.
	d = e.patch(id, change(e1.ID, library.DecisionIgnored, eps["S01E01"]), change(e2.ID, library.DecisionIgnored))
	if p := proposalByPath(d, "E01.1080p.WEB-GRP.mkv"); p.Decision != "ignored" || p.Source != "rules" || !p.UserDecided {
		t.Errorf("ignored = %+v", p)
	}
	d = e.patch(id, change(e1.ID, library.DecisionAccepted), change(e2.ID, library.DecisionAccepted))
	for _, p := range d.Proposals {
		if p.UserDecided || p.Decision != "accepted" {
			t.Errorf("after undo: %+v", p)
		}
	}

	// Apply writes the edits.
	e.patch(id, change(e3.ID, "", eps["S01E04"]))
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E04"]); a == nil || !strings.HasSuffix(a.FilePath, "E03.1080p.WEB-GRP.mkv") ||
		a.MatchSource != "manual" || a.Confidence != "" {
		t.Errorf("E04 = %+v", a)
	}
	if e.active(library.ItemTypeEpisode, eps["S01E03"]) != nil {
		t.Error("E03 got a file the user moved away")
	}
}

// A batch applies all or nothing: one bad item and nothing changes.
func TestPatch_AllOrNothing(t *testing.T) {
	hash := strings.Repeat("4", 40)
	e := newJobEnv(t, info(hash, "Atomic.Show.S01",
		file("Atomic.Show.S01E01.Pilot.mkv", gb), file("Atomic.Show.S01E01.Extended.Cut.mkv", gb),
		file("Atomic.Show.Mystery.mkv", gb)))
	showID, eps := e.show("Atomic Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}})
	otherShow, otherEps := e.show("Other Show", [][3]any{{1, 1, "Elsewhere"}})
	_ = otherShow
	id := e.assignShow(showID, hash, nil)
	before := e.detail(id)
	if before.Job.Status != library.JobNeedsReview {
		t.Fatalf("status = %s", before.Job.Status)
	}
	pilot := proposalByPath(before, "Pilot.mkv")
	unmatched := proposalByPath(before, "Mystery.mkv")
	if unmatched == nil || unmatched.Group != "unmatched" {
		t.Fatalf("unmatched = %+v", unmatched)
	}

	bad := map[string][]ProposalChange{
		"unknown proposal":       {change(pilot.ID, library.DecisionIgnored), change(999_999, library.DecisionAccepted)},
		"another show's episode": {change(pilot.ID, library.DecisionIgnored), change(unmatched.ID, "", otherEps["S01E01"])},
		"accept with no target":  {change(pilot.ID, library.DecisionIgnored), change(unmatched.ID, library.DecisionAccepted)},
		"unknown decision":       {change(pilot.ID, library.DecisionIgnored), change(unmatched.ID, "maybe")},
	}
	for name, changes := range bad {
		if _, err := e.svc.PatchProposals(e.ctx, id, changes); validationMessage(err) == "" {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}
	after := e.detail(id)
	if after.Job.Status != before.Job.Status || after.Job.Counts != before.Job.Counts {
		t.Errorf("job changed: %s %+v", after.Job.Status, after.Job.Counts)
	}
	for i := range before.Proposals {
		b, a := before.Proposals[i], after.Proposals[i]
		if a.Decision != b.Decision || a.UserDecided != b.UserDecided || len(a.Targets) != len(b.Targets) {
			t.Errorf("proposal %d changed: %+v → %+v", b.ID, b, a)
		}
	}

	// A file with no episode can take one and be accepted in one item.
	d := e.patch(id, change(unmatched.ID, library.DecisionAccepted, eps["S01E02"]))
	if p := proposalByPath(d, "Mystery.mkv"); p.Decision != "accepted" || p.Group != "unmatched" || p.Source != "manual" {
		t.Errorf("unmatched pick = %+v", p)
	}
}

// PATCH needs a classified, open job: job_working before proposals exist,
// job_closed once applied. During resolving it works, and the status is
// left for the AI run's end to settle.
func TestPatch_JobStates(t *testing.T) {
	hash := strings.Repeat("5", 40)
	e := newJobEnv(t, info(hash, "States.Show.S01",
		file("States.Show.S01E01.Pilot.mkv", gb), file("States.Show.S01E01.Extended.Cut.mkv", gb)))
	showID, _ := e.show("States Show", [][3]any{{1, 1, "One"}})

	id, _, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(hash)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PatchProposals(e.ctx, id, []ProposalChange{change(1, library.DecisionIgnored)}); conflictCode(err) != CodeJobWorking {
		t.Errorf("queued: %v", err)
	}
	e.run(id)
	extended := proposalByPath(e.detail(id), "Extended.Cut.mkv")
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'resolving' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	d := e.patch(id, change(extended.ID, library.DecisionIgnored))
	if d.Job.Status != library.JobResolving || proposalByPath(d, "Extended.Cut.mkv").Decision != "ignored" {
		t.Errorf("resolving: %s", d.Job.Status)
	}

	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PatchProposals(e.ctx, id, []ProposalChange{change(extended.ID, library.DecisionPending)}); conflictCode(err) != CodeJobClosed {
		t.Errorf("cancelled: %v", err)
	}
	if _, err := e.svc.PatchProposals(e.ctx, 999_999, []ProposalChange{change(1, library.DecisionPending)}); !errors.Is(err, library.ErrJobNotFound) {
		t.Errorf("unknown job: %v", err)
	}
}

// A movie job takes decisions only: accepting a candidate ignores the
// others, and Apply writes it.
func TestPatch_MovieCandidates(t *testing.T) {
	pack := strings.Repeat("6", 40)
	e := newJobEnv(t, info(pack, "Nolan Pack",
		file("Inception.2010.1080p.BluRay.x264.mkv", 14*gb),
		file("Interstellar.2014.1080p.BluRay.x264.mkv", 12*gb)))
	movieID := e.movie("Interstellar", 2014)
	showID, eps := e.show("Not A Movie", [][3]any{{1, 1, "One"}})
	_ = showID
	id, _, err := e.svc.Assign(e.ctx, library.JobMovie, movieID, AssignRequest{MagnetURI: magnet(pack)})
	if err != nil {
		t.Fatal(err)
	}
	e.run(id)
	d := e.detail(id)
	inception, interstellar := proposalByPath(d, "Inception.2010.1080p.BluRay.x264.mkv"), proposalByPath(d, "Interstellar.2014.1080p.BluRay.x264.mkv")
	if d.Job.Status != library.JobNeedsReview || inception.Group != "needs_review" || interstellar.Group != "needs_review" {
		t.Fatalf("job = %s", d.Job.Status)
	}

	if _, err := e.svc.PatchProposals(e.ctx, id, []ProposalChange{change(interstellar.ID, library.DecisionAccepted, eps["S01E01"])}); validationMessage(err) == "" {
		t.Errorf("episode ids on a movie job: %v", err)
	}

	d = e.patch(id, change(interstellar.ID, library.DecisionAccepted))
	if p := proposalByPath(d, "Inception.2010.1080p.BluRay.x264.mkv"); p.Decision != "ignored" || !p.UserDecided {
		t.Errorf("the other candidate = %+v", p)
	}
	if p := proposalByPath(d, "Interstellar.2014.1080p.BluRay.x264.mkv"); p.Decision != "accepted" || !p.UserDecided {
		t.Errorf("the pick = %+v", p)
	}
	if d.Job.Status != library.JobReady || d.Job.Counts.Matched != 1 || d.Job.Counts.ToApply != 1 {
		t.Errorf("job = %s %+v", d.Job.Status, d.Job.Counts)
	}

	// Changing one's mind: the other one wins now.
	d = e.patch(id, change(inception.ID, library.DecisionAccepted))
	if p := proposalByPath(d, "Interstellar.2014.1080p.BluRay.x264.mkv"); p.Decision != "ignored" {
		t.Errorf("the first pick = %+v", p)
	}
	d = e.patch(id, change(interstellar.ID, library.DecisionAccepted))
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeMovie, movieID); a == nil || !strings.HasSuffix(a.FilePath, "Interstellar.2014.1080p.BluRay.x264.mkv") {
		t.Errorf("movie = %+v", a)
	}
}

// An extra the user includes and gives an episode is applied as that
// episode's file.
func TestPatch_IncludedExtraApplies(t *testing.T) {
	hash := strings.Repeat("7", 40)
	e := newJobEnv(t, info(hash, "Extra.Show.S01",
		file("Extra.Show.S01E01.mkv", gb),
		file("Extra.Show.S01.NCOP.mkv", 100<<20)))
	showID, eps := e.show("Extra Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}})
	e.setAI(`{"auto_apply_high_confidence": false}`)
	id := e.assignShow(showID, hash, nil)
	d := e.detail(id)
	ncop := proposalByPath(d, "NCOP.mkv")
	if ncop == nil || ncop.Group != "extras" || ncop.Decision != "ignored" {
		t.Fatalf("NCOP = %+v", ncop)
	}
	d = e.patch(id, change(ncop.ID, library.DecisionAccepted, eps["S01E02"]))
	if p := proposalByPath(d, "NCOP.mkv"); p.Kind != "extra" || p.Group != "extras" || !p.UserDecided || d.Job.Counts.ToApply != 2 {
		t.Errorf("included = %+v, counts %+v", p, d.Job.Counts)
	}
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E02"]); a == nil || !strings.HasSuffix(a.FilePath, "NCOP.mkv") || a.MatchSource != "manual" {
		t.Errorf("E02 = %+v", a)
	}
}
