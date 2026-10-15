package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/library"
)

func tgt(id int64, season, episode int) identify.Target {
	return identify.Target{EpisodeID: id, Season: season, Episode: episode}
}

// The classifier's kinds land in the contract's kind, group, extra_type and
// starting decision.
func TestShowDrafts(t *testing.T) {
	const root = "Show.S01/"
	props := []identify.Proposal{
		{Path: root + "E01.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(101, 1, 1)}, Confidence: identify.ConfidenceHigh, Reason: `"S01E01" in the name`},
		{Path: root + "E02E03.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(102, 1, 2), tgt(103, 1, 3)}, Confidence: identify.ConfidenceHigh},
		{Path: root + "E04.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(104, 1, 4)}, Confidence: identify.ConfidenceMedium},
		{Path: root + "1080p/E05.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(105, 1, 5)}, Confidence: identify.ConfidenceHigh},
		{Path: root + "720p/E05.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(105, 1, 5)}, Confidence: identify.ConfidenceHigh},
		{Path: root + "Special.mkv", Kind: identify.KindSpecial, Targets: []identify.Target{tgt(1, 0, 1)}, Confidence: identify.ConfidenceHigh},
		{Path: root + "OVA.mkv", Kind: identify.KindSpecial, Targets: []identify.Target{tgt(2, 0, 2)}, Confidence: identify.ConfidenceLow},
		{Path: root + "Specials off.mkv", Kind: identify.KindSpecial, Reason: "Specials are off for this show"},
		{Path: root + "Mystery.mkv", Kind: identify.KindUnknown, Reason: "No episode number or title in the name"},
		{Path: root + "NCOP.mkv", Kind: identify.KindExtra, ExtraType: identify.ExtraOpening},
		{Path: root + "Sample/E01.sample.mkv", Kind: identify.KindSample, ExtraType: identify.ExtraSample},
		{Path: root + "E06.mkv", Kind: identify.KindExtra, ExtraType: identify.ExtraSuperseded, SupersededBy: root + "E06v2.mkv"},
		{Path: root + "E06v2.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(106, 1, 6)}, Confidence: identify.ConfidenceHigh, Source: identify.SourceAI},
		{Path: root + "E01.en.srt", Kind: identify.KindSubtitle, LinkedVideo: root + "E01.mkv", Targets: []identify.Target{tgt(101, 1, 1)}, Confidence: identify.ConfidenceHigh},
		{Path: root + "E09.en.srt", Kind: identify.KindSubtitle, Targets: []identify.Target{tgt(109, 1, 9)}, Confidence: identify.ConfidenceMedium},
		{Path: root + "Fonts/a.ttf", Kind: identify.KindExtra, ExtraType: identify.ExtraFont},
		{Path: root + "info.nfo", Kind: identify.KindExtra},
	}
	type want struct {
		kind       library.ProposalKind
		group      library.ProposalGroup
		extra      string
		decision   library.Decision
		ids        []int64
		confidence string
		linked     string
		superseded string
		source     string
	}
	wants := []want{
		{library.KindEpisode, library.GroupMatched, "", library.DecisionAccepted, []int64{101}, "high", "", "", "rules"},
		{library.KindEpisode, library.GroupMatched, "", library.DecisionAccepted, []int64{102, 103}, "high", "", "", "rules"},
		{library.KindEpisode, library.GroupNeedsReview, "", library.DecisionPending, []int64{104}, "medium", "", "", "rules"},
		{library.KindEpisode, library.GroupNeedsReview, "", library.DecisionPending, []int64{105}, "high", "", "", "rules"},
		{library.KindEpisode, library.GroupNeedsReview, "", library.DecisionPending, []int64{105}, "high", "", "", "rules"},
		{library.KindEpisode, library.GroupSpecials, "", library.DecisionAccepted, []int64{1}, "high", "", "", "rules"},
		{library.KindEpisode, library.GroupNeedsReview, "", library.DecisionPending, []int64{2}, "low", "", "", "rules"},
		{library.KindEpisode, library.GroupUnmatched, "", library.DecisionPending, nil, "", "", "", "rules"},
		{library.KindEpisode, library.GroupUnmatched, "", library.DecisionPending, nil, "", "", "", "rules"},
		{library.KindExtra, library.GroupExtras, "opening", library.DecisionIgnored, nil, "", "", "", "rules"},
		{library.KindExtra, library.GroupExtras, "sample", library.DecisionIgnored, nil, "", "", "", "rules"},
		{library.KindExtra, library.GroupExtras, "superseded", library.DecisionIgnored, nil, "", "", root + "E06v2.mkv", "rules"},
		{library.KindEpisode, library.GroupMatched, "", library.DecisionAccepted, []int64{106}, "high", "", "", "ai"},
		{library.KindSubtitle, library.GroupExtras, "", library.DecisionIgnored, []int64{101}, "high", root + "E01.mkv", "", "rules"},
		{library.KindSubtitle, library.GroupExtras, "", library.DecisionIgnored, []int64{109}, "medium", "", "", "rules"},
		{library.KindExtra, library.GroupExtras, "font", library.DecisionIgnored, nil, "", "", "", "rules"},
		{library.KindExtra, library.GroupExtras, "other", library.DecisionIgnored, nil, "", "", "", "rules"},
	}

	drafts := showDrafts(props)
	if len(drafts) != len(wants) {
		t.Fatalf("drafts = %d, want %d", len(drafts), len(wants))
	}
	for i, d := range drafts {
		w := wants[i]
		got := want{d.Kind, d.Group, d.ExtraType, d.Decision, d.EpisodeIDs, d.Confidence, d.LinkedPath, d.SupersededByPath, d.Source}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("%s:\n got  %+v\n want %+v", props[i].Path, got, w)
		}
	}
	if drafts[0].Reason != `"S01E01" in the name` || drafts[7].Reason != "Specials are off for this show" {
		t.Errorf("reasons = %q, %q", drafts[0].Reason, drafts[7].Reason)
	}
	if n := videoCount(drafts); n != 10 {
		t.Errorf("videoCount = %d, want 10 (main videos, matched or not)", n)
	}
}

func TestMovieDrafts(t *testing.T) {
	files := []identify.Proposal{
		{Path: "Pack/Film.2014.mkv", Size: 12 << 30, Kind: identify.KindUnknown},
		{Path: "Pack/Film.2014.Extended.mkv", Size: 14 << 30, Kind: identify.KindUnknown},
		{Path: "Pack/Bonus.mkv", Size: 1 << 30, Kind: identify.KindUnknown},
		{Path: "Pack/Sample/film.sample.mkv", Size: 50 << 20, Kind: identify.KindSample, ExtraType: identify.ExtraSample, Reason: "Sample clip"},
		{Path: "Pack/Film.2014.en.srt", Kind: identify.KindSubtitle, LinkedVideo: "Pack/Film.2014.mkv"},
		{Path: "Pack/Film.nfo", Kind: identify.KindExtra, ExtraType: identify.ExtraInfo},
		{Path: "Pack/Behind.mkv", Size: 10 << 20, Kind: identify.KindUnknown},
	}
	collection := []identify.MovieCandidate{
		{Path: "Pack/Film.2014.Extended.mkv", Confidence: identify.ConfidenceMedium, Reason: "Best of 2 large videos"},
		{Path: "Pack/Film.2014.mkv", Confidence: identify.ConfidenceMedium, Reason: "Another large video"},
		{Path: "Pack/Bonus.mkv", Confidence: identify.ConfidenceLow, Reason: "Smaller video"},
	}
	type row struct {
		kind     library.ProposalKind
		group    library.ProposalGroup
		decision library.Decision
		extra    string
	}
	got := func(ds []library.ProposalDraft) []row {
		var out []row
		for _, d := range ds {
			out = append(out, row{d.Kind, d.Group, d.Decision, d.ExtraType})
		}
		return out
	}

	drafts := movieDrafts(files, collection)
	want := []row{
		{library.KindMovie, library.GroupNeedsReview, library.DecisionPending, ""},
		{library.KindMovie, library.GroupNeedsReview, library.DecisionPending, ""},
		{library.KindMovie, library.GroupExtras, library.DecisionIgnored, ""},
		{library.KindExtra, library.GroupExtras, library.DecisionIgnored, "sample"},
		{library.KindSubtitle, library.GroupExtras, library.DecisionIgnored, ""},
		{library.KindExtra, library.GroupExtras, library.DecisionIgnored, "info"},
		{library.KindExtra, library.GroupExtras, library.DecisionIgnored, "other"},
	}
	if !reflect.DeepEqual(got(drafts), want) {
		t.Errorf("collection:\n got  %+v\n want %+v", got(drafts), want)
	}
	if drafts[4].LinkedPath != "Pack/Film.2014.mkv" || drafts[6].Reason != "Too small to be the film" {
		t.Errorf("subtitle link %q, small video reason %q", drafts[4].LinkedPath, drafts[6].Reason)
	}
	if videoCount(drafts) != 3 {
		t.Errorf("videoCount = %d, want 3 candidates", videoCount(drafts))
	}

	single := []identify.MovieCandidate{
		{Path: "Pack/Film.2014.mkv", Confidence: identify.ConfidenceHigh, Reason: "Largest video"},
		{Path: "Pack/Bonus.mkv", Confidence: identify.ConfidenceLow, Reason: "Smaller video"},
	}
	drafts = movieDrafts(files[:3], single)
	want = []row{
		{library.KindMovie, library.GroupMatched, library.DecisionAccepted, ""},
		{library.KindExtra, library.GroupExtras, library.DecisionIgnored, "other"},
		{library.KindMovie, library.GroupExtras, library.DecisionIgnored, ""},
	}
	if !reflect.DeepEqual(got(drafts), want) {
		t.Errorf("one film:\n got  %+v\n want %+v", got(drafts), want)
	}
}

// prop is a stored proposal for the rule tests.
func prop(id int64, kind library.ProposalKind, group library.ProposalGroup, decision library.Decision, confidence string, ids ...int64) *library.AssignmentProposal {
	p := &library.AssignmentProposal{ID: id, Kind: kind, Group: group, Decision: decision, Confidence: confidence, EpisodeIDs: ids, Source: "rules"}
	if len(ids) > 0 {
		season := 1
		if ids[0] < 100 {
			season = 0
		}
		p.FirstSeason = &season
	}
	return p
}

func TestCountsAndStatus(t *testing.T) {
	const (
		ep  = library.KindEpisode
		mov = library.KindMovie
		ext = library.KindExtra
		sub = library.KindSubtitle
	)
	acc, pen, ign := library.DecisionAccepted, library.DecisionPending, library.DecisionIgnored
	tests := []struct {
		name   string
		ps     []*library.AssignmentProposal
		want   JobCounts
		status library.JobStatus
	}{
		{
			name: "all matched",
			ps: []*library.AssignmentProposal{
				prop(1, ep, library.GroupMatched, acc, "high", 101),
				prop(2, ep, library.GroupMatched, acc, "high", 102, 103),
				prop(3, ep, library.GroupSpecials, acc, "high", 1),
				prop(4, ext, library.GroupExtras, ign, ""),
			},
			want:   JobCounts{Files: 4, ToApply: 3, Matched: 2, Specials: 1, Ignored: 1},
			status: library.JobReady,
		},
		{
			name: "a pending mapping blocks; an unmatched video doesn't",
			ps: []*library.AssignmentProposal{
				prop(1, ep, library.GroupNeedsReview, pen, "medium", 101),
				prop(2, ep, library.GroupUnmatched, pen, ""),
			},
			want:   JobCounts{Files: 2, ToApply: 1, Pending: 1, NeedsReview: 1, Unmatched: 1},
			status: library.JobNeedsReview,
		},
		{
			name: "unmatched only is ready",
			ps: []*library.AssignmentProposal{
				prop(1, ep, library.GroupMatched, acc, "high", 101),
				prop(2, ep, library.GroupUnmatched, pen, ""),
			},
			want:   JobCounts{Files: 2, ToApply: 1, Matched: 1, Unmatched: 1},
			status: library.JobReady,
		},
		{
			name: "two accepted files on one episode conflict",
			ps: []*library.AssignmentProposal{
				prop(1, ep, library.GroupNeedsReview, acc, "high", 105),
				prop(2, ep, library.GroupNeedsReview, acc, "high", 105, 106),
				prop(3, ep, library.GroupMatched, acc, "high", 107),
			},
			want:   JobCounts{Files: 3, ToApply: 3, Conflicts: 2, NeedsReview: 2, Matched: 3},
			status: library.JobNeedsReview,
		},
		{
			name: "an ignored file never conflicts, nor does a subtitle",
			ps: []*library.AssignmentProposal{
				prop(1, ep, library.GroupNeedsReview, acc, "high", 105),
				prop(2, ep, library.GroupNeedsReview, ign, "high", 105),
				prop(3, sub, library.GroupExtras, acc, "medium", 105),
			},
			want:   JobCounts{Files: 3, ToApply: 2, Matched: 2, Ignored: 1},
			status: library.JobReady,
		},
		{
			name: "pending and conflicting count once in needs_review",
			ps: []*library.AssignmentProposal{
				prop(1, ep, library.GroupNeedsReview, pen, "medium", 105),
				prop(2, ep, library.GroupNeedsReview, pen, "low", 105),
			},
			want:   JobCounts{Files: 2, ToApply: 2, Pending: 2, Conflicts: 2, NeedsReview: 2},
			status: library.JobNeedsReview,
		},
		{
			name: "movie candidates map and never conflict",
			ps: []*library.AssignmentProposal{
				prop(1, mov, library.GroupNeedsReview, pen, "medium"),
				prop(2, mov, library.GroupNeedsReview, pen, "medium"),
			},
			want:   JobCounts{Files: 2, ToApply: 2, Pending: 2, NeedsReview: 2},
			status: library.JobNeedsReview,
		},
		{
			name: "the accepted film is matched",
			ps: []*library.AssignmentProposal{
				prop(1, mov, library.GroupNeedsReview, acc, "medium"),
				prop(2, mov, library.GroupNeedsReview, ign, "medium"),
			},
			want:   JobCounts{Files: 2, ToApply: 1, Matched: 1, Ignored: 1},
			status: library.JobReady,
		},
		{
			name:   "nothing",
			want:   JobCounts{},
			status: library.JobReady,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countsOf(tt.ps); got != tt.want {
				t.Errorf("counts = %+v\nwant     %+v", got, tt.want)
			}
			if got := settledStatus(tt.ps); got != tt.status {
				t.Errorf("status = %s, want %s", got, tt.status)
			}
		})
	}
}

func TestAutoApplies(t *testing.T) {
	acc, pen, ign := library.DecisionAccepted, library.DecisionPending, library.DecisionIgnored
	ep := library.KindEpisode
	matched := prop(1, ep, library.GroupMatched, acc, "high", 101)
	extra := prop(9, library.KindExtra, library.GroupExtras, ign, "")
	tests := []struct {
		name   string
		dryRun bool
		ps     []*library.AssignmentProposal
		want   bool
	}{
		{"everything high", false, []*library.AssignmentProposal{matched, extra}, true},
		{"nothing to apply (openings only)", false, []*library.AssignmentProposal{extra}, false},
		{"nothing at all", false, nil, false},
		{"a dry run never", true, []*library.AssignmentProposal{matched}, false},
		{"pending", false, []*library.AssignmentProposal{matched, prop(2, ep, library.GroupNeedsReview, pen, "medium", 102)}, false},
		{"accepted medium (an AI answer)", false, []*library.AssignmentProposal{matched, prop(2, ep, library.GroupMatched, acc, "medium", 102)}, false},
		{"an unmatched video", false, []*library.AssignmentProposal{matched, prop(2, ep, library.GroupUnmatched, pen, "")}, false},
		{"an ignored unmatched video", false, []*library.AssignmentProposal{matched, prop(2, ep, library.GroupUnmatched, ign, "")}, true},
		{"a conflict", false, []*library.AssignmentProposal{matched, prop(2, ep, library.GroupNeedsReview, acc, "high", 101)}, false},
		{"one plausible film", false, []*library.AssignmentProposal{prop(1, library.KindMovie, library.GroupMatched, acc, "high"), prop(2, library.KindMovie, library.GroupExtras, ign, "low")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &library.AssignmentJob{DryRun: tt.dryRun}
			if got := autoApplies(job, tt.ps); got != tt.want {
				t.Errorf("autoApplies = %v, want %v", got, tt.want)
			}
		})
	}
}

// Apply's 409s come in the contract's order.
func TestApplyChecks(t *testing.T) {
	conflicting := []*library.AssignmentProposal{
		prop(1, library.KindEpisode, library.GroupNeedsReview, library.DecisionPending, "high", 101),
		prop(2, library.KindEpisode, library.GroupNeedsReview, library.DecisionAccepted, "high", 101),
	}
	pending := []*library.AssignmentProposal{prop(1, library.KindEpisode, library.GroupNeedsReview, library.DecisionPending, "low", 101)}
	tests := []struct {
		name   string
		job    library.AssignmentJob
		ps     []*library.AssignmentProposal
		mode   applyMode
		want   string
		wantOK bool
	}{
		{"dry run first", library.AssignmentJob{DryRun: true, Status: library.JobApplied}, conflicting, applyReview, CodeDryRun, false},
		{"applied", library.AssignmentJob{Status: library.JobApplied}, conflicting, applyReview, CodeJobClosed, false},
		{"failed", library.AssignmentJob{Status: library.JobFailed}, nil, applyReview, CodeJobClosed, false},
		{"cancelled", library.AssignmentJob{Status: library.JobCancelled}, nil, applyReview, CodeJobClosed, false},
		{"queued", library.AssignmentJob{Status: library.JobQueued}, nil, applyReview, CodeJobWorking, false},
		{"resolving", library.AssignmentJob{Status: library.JobResolving}, conflicting, applyReview, CodeJobWorking, false},
		{"conflict before pending", library.AssignmentJob{Status: library.JobNeedsReview}, conflicting, applyReview, CodeConflictingProposals, false},
		{"pending", library.AssignmentJob{Status: library.JobNeedsReview}, pending, applyReview, CodePendingProposals, false},
		{"ready", library.AssignmentJob{Status: library.JobReady}, nil, applyReview, "", true},
		{"legacy leaves conflicts out", library.AssignmentJob{Status: library.JobNeedsReview}, conflicting, applyLegacy, "", true},
		{"legacy still needs a classified job", library.AssignmentJob{Status: library.JobClassifying}, nil, applyLegacy, CodeJobWorking, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := applyChecks(&tt.job, tt.ps, tt.mode)
			if tt.wantOK {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			var c *JobConflictError
			if !errors.As(err, &c) || c.Code != tt.want {
				t.Errorf("err = %v, want code %s", err, tt.want)
			}
		})
	}
}

func TestSortProposals(t *testing.T) {
	conf := func(s string) *string { return &s }
	target := func(season, episode int) []ProposalTarget {
		return []ProposalTarget{{Season: season, Episode: episode}}
	}
	ps := []ProposalView{
		{ID: 1, Group: "extras", FilePath: "a"},
		{ID: 2, Group: "matched", Targets: target(1, 2), Confidence: conf("high")},
		{ID: 3, Group: "matched", Targets: target(1, 1), Confidence: conf("high")},
		{ID: 4, Group: "specials", Targets: target(0, 1), Confidence: conf("high")},
		{ID: 5, Group: "needs_review", Targets: target(0, 3), Confidence: conf("low")},
		{ID: 6, Group: "needs_review", Targets: target(2, 1), Confidence: conf("medium")},
		{ID: 7, Group: "unmatched", FilePath: "b"},
		{ID: 8, Group: "unmatched", FilePath: "a"},
		{ID: 9, Group: "needs_review", Confidence: conf("medium"), FilePath: "film2"},
		{ID: 10, Group: "needs_review", Confidence: conf("high"), FilePath: "film1"},
	}
	sortProposals(ps)
	var got []int64
	for _, p := range ps {
		got = append(got, p.ID)
	}
	want := []int64{6, 5, 10, 9, 8, 7, 4, 3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// replaces is computed when read: the episode's file from elsewhere, never
// the proposal's own file once applied.
func TestReplacesOf(t *testing.T) {
	job := &library.AssignmentJob{InfoHash: "new"}
	p := &library.AssignmentProposal{FilePath: "Pack/E01.mkv", EpisodeIDs: []int64{101}}
	other := &library.EpisodeRef{ID: 101, Active: &library.TorrentAssignment{
		InfoHash: "old", MagnetURI: "magnet:?xt=urn:btih:old&dn=Show.S01.720p", FilePath: "Show.S01.720p/E01.mkv"}}
	own := &library.EpisodeRef{ID: 101, Active: &library.TorrentAssignment{InfoHash: "new", FilePath: "Pack/E01.mkv"}}
	sameHashOtherFile := &library.EpisodeRef{ID: 101, Active: &library.TorrentAssignment{InfoHash: "new", MagnetURI: "magnet:?xt=urn:btih:new", FilePath: "Pack/E01.v1.mkv"}}

	if got := replacesOf(job, p, other); got == nil || *got != (ProposalReplaces{ReleaseName: "Show.S01.720p", FilePath: "Show.S01.720p/E01.mkv"}) {
		t.Errorf("another torrent's file: %+v", got)
	}
	if got := replacesOf(job, p, own); got != nil {
		t.Errorf("its own file: %+v", got)
	}
	if got := replacesOf(job, p, sameHashOtherFile); got == nil || got.FilePath != "Pack/E01.v1.mkv" {
		t.Errorf("another file of the torrent: %+v", got)
	}
	if got := replacesOf(job, p, &library.EpisodeRef{ID: 101}); got != nil {
		t.Errorf("no file: %+v", got)
	}
	stream := &library.EpisodeRef{ID: 101, Active: &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka,
		FilePath: "hdrezka/646/t56/s01e01/1080p.mp4", Stream: &library.StreamRef{TranslatorName: "LostFilm", Quality: "1080p"}}}
	if got := replacesOf(job, p, stream); got == nil || *got != (ProposalReplaces{ReleaseName: "HDRezka · LostFilm · 1080p"}) {
		t.Errorf("an HDRezka stream: %+v", got)
	}
}

// The first AI run only touches pending proposals the user hasn't, and
// sets their group from its answer.
func TestMergeFirstRun(t *testing.T) {
	current := []*library.AssignmentProposal{
		{ID: 1, FilePath: "a.mkv", Kind: library.KindEpisode, Group: library.GroupMatched, Decision: library.DecisionAccepted, EpisodeIDs: []int64{101}, Confidence: "high", Source: "rules"},
		{ID: 2, FilePath: "b.mkv", Kind: library.KindEpisode, Group: library.GroupUnmatched, Decision: library.DecisionPending, Source: "rules", Reason: "no number"},
		{ID: 3, FilePath: "c.mkv", Kind: library.KindEpisode, Group: library.GroupNeedsReview, Decision: library.DecisionPending, EpisodeIDs: []int64{103}, Confidence: "low", Source: "rules", UserDecided: true},
		{ID: 4, FilePath: "d.mkv", Kind: library.KindEpisode, Group: library.GroupNeedsReview, Decision: library.DecisionPending, EpisodeIDs: []int64{104}, Confidence: "low", Source: "rules", Reason: "r"},
	}
	merged := []library.ProposalDraft{
		{FilePath: "a.mkv", Kind: library.KindEpisode, Group: library.GroupMatched, Decision: library.DecisionAccepted, EpisodeIDs: []int64{101}, Confidence: "high", Source: "rules"},
		{FilePath: "b.mkv", Kind: library.KindEpisode, Group: library.GroupSpecials, Decision: library.DecisionAccepted, EpisodeIDs: []int64{5}, Confidence: "high", Source: "ai", Reason: `"Mini Anime 05" → S00E06 by title`},
		{FilePath: "c.mkv", Kind: library.KindEpisode, Group: library.GroupMatched, Decision: library.DecisionAccepted, EpisodeIDs: []int64{105}, Confidence: "high", Source: "ai"},
		{FilePath: "d.mkv", Kind: library.KindEpisode, Group: library.GroupNeedsReview, Decision: library.DecisionPending, EpisodeIDs: []int64{104}, Confidence: "low", Source: "rules", Reason: "r"},
	}
	changed := mergeFirstRun(current, merged)
	if len(changed) != 1 || changed[0].ID != 2 {
		t.Fatalf("changed = %+v, want only proposal 2", changed)
	}
	p := changed[0]
	if p.Group != library.GroupSpecials || p.Decision != library.DecisionAccepted || p.Source != "ai" ||
		!reflect.DeepEqual(p.SuggestedEpisodeIDs, []int64{5}) || p.SuggestedSource != "ai" || p.Updated {
		t.Errorf("merged proposal = %+v", p)
	}
	if current[1].Group != library.GroupUnmatched {
		t.Error("mergeFirstRun changed its input")
	}
}

func TestPickMovie(t *testing.T) {
	ps := []*library.AssignmentProposal{
		{ID: 1, FilePath: "Pack/Film.mkv", Kind: library.KindMovie, Group: library.GroupNeedsReview, Decision: library.DecisionPending},
		{ID: 2, FilePath: "Pack/Film.Extended.mkv", Kind: library.KindMovie, Group: library.GroupNeedsReview, Decision: library.DecisionPending},
		{ID: 3, FilePath: "Pack/Bonus.mkv", Kind: library.KindMovie, Group: library.GroupExtras, Decision: library.DecisionIgnored},
		{ID: 4, FilePath: "Pack/Film.Extended.en.srt", Kind: library.KindSubtitle, Group: library.GroupExtras, Decision: library.DecisionIgnored},
	}
	changed := pickMovie(ps, "Film.Extended.mkv") // without the torrent's folder
	got := map[int64]library.Decision{}
	for _, p := range changed {
		got[p.ID] = p.Decision
		if !p.UserDecided {
			t.Errorf("proposal %d isn't user_decided", p.ID)
		}
	}
	want := map[int64]library.Decision{1: library.DecisionIgnored, 2: library.DecisionAccepted}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changed = %v, want %v", got, want)
	}
	if pickMovie(ps, "Other.mkv") != nil || pickMovie(ps, "lm.Extended.mkv") != nil {
		t.Error("a path no candidate has picked one")
	}
}

// The worker asks the AI about ai.Scope's files: the unsure main videos,
// or every one in ai_always.
func TestAIScope(t *testing.T) {
	props := []identify.Proposal{
		{Path: "a.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(1, 1, 1)}, Confidence: identify.ConfidenceHigh},
		{Path: "b.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(2, 1, 2)}, Confidence: identify.ConfidenceMedium},
		{Path: "c.mkv", Kind: identify.KindUnknown},
		{Path: "d.mkv", Kind: identify.KindExtra, ExtraType: identify.ExtraOpening},
		{Path: "e.srt", Kind: identify.KindSubtitle},
		{Path: "f.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(3, 1, 3)}, Confidence: identify.ConfidenceHigh},
		{Path: "g.mkv", Kind: identify.KindEpisode, Targets: []identify.Target{tgt(3, 1, 3)}, Confidence: identify.ConfidenceHigh},
	}
	if got := ai.Scope(props, false); !reflect.DeepEqual(got, []string{"b.mkv", "c.mkv", "f.mkv", "g.mkv"}) {
		t.Errorf("auto = %v", got)
	}
	if got := ai.Scope(props, true); len(got) != 5 {
		t.Errorf("ai_always = %v, want every main video", got)
	}
}

func TestNormalizeInfoHash(t *testing.T) {
	tests := []struct{ in, want string }{
		{"B7A3F1E9C2D4058A6E1F3C5B7D9A2E4F6081C3D5", "b7a3f1e9c2d4058a6e1f3c5b7d9a2e4f6081c3d5"},
		{"wor2d2ouauyxi3ljpmjmmp6wngqaliem", "b3a3a1e9d40531746d697b12c63fd669a005a08c"}, // base32
		{"not-a-hash", ""},
		{"", ""},
		{strings.Repeat("z", 40), ""},
	}
	for _, tt := range tests {
		if got := normalizeInfoHash(tt.in); got != tt.want {
			t.Errorf("normalizeInfoHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseStatuses(t *testing.T) {
	if got, err := ParseStatuses(""); err != nil || len(got) != len(library.OpenJobStatuses) {
		t.Errorf(`"" = %v, %v`, got, err)
	}
	if got, err := ParseStatuses("open"); err != nil || len(got) != 6 {
		t.Errorf("open = %v, %v", got, err)
	}
	if got, err := ParseStatuses("applied, failed"); err != nil || !reflect.DeepEqual(got, []library.JobStatus{library.JobApplied, library.JobFailed}) {
		t.Errorf("list = %v, %v", got, err)
	}
	if _, err := ParseStatuses("applied,done"); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("unknown = %v", err)
	}
}

func TestValidateHint(t *testing.T) {
	n := func(v int) *int { return &v }
	ok := []*library.JobHint{nil, {Season: n(0)}, {Season: n(2), Episode: n(1), Text: "absolute numbers"}}
	for _, h := range ok {
		if err := validateHint(h); err != nil {
			t.Errorf("%+v: %v", h, err)
		}
	}
	bad := []*library.JobHint{{Season: n(-1)}, {Episode: n(0)}, {Text: strings.Repeat("x", maxHintText+1)}}
	for _, h := range bad {
		if err := validateHint(h); !errors.Is(err, ErrInvalidHint) {
			t.Errorf("%+v: %v, want ErrInvalidHint", h, err)
		}
	}
	if normalizeHint(&library.JobHint{Text: "  "}) != nil {
		t.Error("an empty hint is kept")
	}
}

func TestClipReason(t *testing.T) {
	long := strings.Repeat("é", 130)
	if got := clipReason(long); len([]rune(got)) != maxReason || !strings.HasSuffix(got, "…") {
		t.Errorf("clipReason = %d runes", len([]rune(got)))
	}
	if clipReason("short") != "short" {
		t.Error("a short reason changed")
	}
}

func TestKeyedMutex(t *testing.T) {
	var k keyedMutex
	unlockA := k.Lock("a")
	done := make(chan struct{})
	go func() {
		unlock := k.Lock("a")
		unlock()
		close(done)
	}()
	unlockB := k.Lock("b") // another key doesn't wait
	unlockB()
	select {
	case <-done:
		t.Fatal("the second lock of a key didn't wait")
	default:
	}
	unlockA()
	<-done
	if len(k.locks) != 0 {
		t.Errorf("locks left = %d", len(k.locks))
	}
}
