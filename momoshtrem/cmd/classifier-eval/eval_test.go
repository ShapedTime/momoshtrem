package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

const testdata = "../../internal/identify/testdata"

// fakeResolver answers without the network.
type fakeResolver func(identify.ResolveInput) []identify.AIFile

func (f fakeResolver) Resolve(_ context.Context, in identify.ResolveInput) (identify.ResolveOutput, error) {
	return identify.ResolveOutput{Files: f(in), Usage: identify.Usage{Model: "fake", InputTokens: 100, OutputTokens: 10, CostUSD: 0.001}}, nil
}

func intp(n int) *int { return &n }

func TestEvaluateRules(t *testing.T) {
	runs, err := loadGolden(testdata, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) < 20 {
		t.Errorf("%d golden torrents, want at least 20", len(runs))
	}
	rep := evaluate(context.Background(), runs, config{modes: []string{modeRules}, parallel: 1}, nil)
	var out bytes.Buffer
	rep.print(&out, true)
	if !rep.gate(&out) || rep.modes[0].correct != rep.files {
		t.Errorf("rules on the golden set:\n%s", out.String())
	}
}

// TestEvaluateModes runs auto and ai_always against fake resolvers: one
// that maps every file it's asked about to S01E01, and one that answers
// nothing.
func TestEvaluateModes(t *testing.T) {
	runs, err := loadGolden(testdata, []string{"aot-oad-collection", "frieren-erai-batch"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config{modes: []string{modeRules, modeAuto, modeAlways}, parallel: 2}

	wrong := fakeResolver(func(in identify.ResolveInput) []identify.AIFile {
		var out []identify.AIFile
		for _, p := range in.Scope {
			out = append(out, identify.AIFile{Path: p, Kind: identify.KindEpisode, Season: intp(1), Episodes: []int{1}, Confidence: identify.ConfidenceHigh, Reason: "wrong"})
		}
		return out
	})
	rep := evaluate(context.Background(), runs, cfg, wrong)
	var out bytes.Buffer
	rep.print(&out, true)
	if rep.gate(&out) {
		t.Errorf("a resolver that breaks files passed the gate:\n%s", out.String())
	}
	auto := rep.modes[1]
	// The rules leave AoT's two OADs at medium: the fake breaks both. (An
	// episode a high rules answer has, like S00E01, MergeAI would reject.)
	if auto.calls != 1 || auto.broke != 2 || auto.correct != rep.modes[0].correct-2 {
		t.Errorf("auto: %d calls, %d broke, %d correct\n%s", auto.calls, auto.broke, auto.correct, out.String())
	}
	always := rep.modes[2]
	// Alone, it's right once: Frieren's "01" is S01E01.
	if a := always.alone; always.calls != 2 || a.videos != 15 || a.right != 1 || a.wrong != 14 || a.wrongHigh != 14 {
		t.Errorf("ai_always: %d calls, alone %d videos, %d right, %d wrong (%d high)", always.calls, a.videos, a.right, a.wrong, a.wrongHigh)
	}
	if always.usage.CostUSD != 0.002 || !strings.Contains(out.String(), "BROKE") {
		t.Errorf("ai_always usage %+v; report:\n%s", always.usage, out.String())
	}
	// Every high rules answer other than S01E01 is disputed and goes to
	// review with its target kept; the rules were right on each. Frieren's
	// "01" (S01E01) agrees.
	if len(always.disputes) == 0 || always.rulesRight != len(always.disputes) || always.aiRight != 0 {
		t.Errorf("ai_always disputes: %d, rules right %d, AI right %d", len(always.disputes), always.rulesRight, always.aiRight)
	}
	for _, d := range always.disputes {
		if strings.Contains(d.where, "Sousou no Frieren - 01 ") {
			t.Errorf("an agreement was disputed: %s", d.where)
		}
	}
	if !strings.Contains(out.String(), "the AI disagreed with") || !strings.Contains(out.String(), "[rules right]") {
		t.Errorf("the report lacks the disagreements:\n%s", out.String())
	}

	silent := fakeResolver(func(identify.ResolveInput) []identify.AIFile { return nil })
	rep = evaluate(context.Background(), runs, cfg, silent)
	out.Reset()
	rep.print(&out, false)
	if !rep.gate(&out) || rep.modes[2].alone.unanswered != rep.modes[2].alone.videos {
		t.Errorf("a resolver that answers nothing:\n%s", out.String())
	}

	// ai_blind hides the rules' answers from the resolver.
	sawRules := false
	blind := fakeResolver(func(in identify.ResolveInput) []identify.AIFile {
		sawRules = sawRules || len(in.Rules) > 0
		return nil
	})
	evaluate(context.Background(), runs, config{modes: []string{modeBlind}, parallel: 1}, blind)
	if sawRules {
		t.Error("ai_blind passed the rules' answers")
	}
}

func TestDryRun(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), &out, config{testdata: testdata, modes: []string{modeRules, modeAuto, modeAlways}, parallel: 1},
		"claude-haiku-5-5", true, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Dry run: ", "auto:", "ai_always:", "aot-oad-collection", "request 1:", "max_tokens", "total: 30 requests"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry run lacks %q:\n%s", want, out.String())
		}
	}
	if err := run(context.Background(), &out, config{testdata: testdata, modes: []string{"magic"}}, "", true, false); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

func TestLoadKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("TMDB_API_KEY=x\nexport ANTHROPIC_API_KEY=\"sk-ant-test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadKey(path); err != nil || os.Getenv("ANTHROPIC_API_KEY") != "sk-ant-test" {
		t.Errorf("loadKey = %v", err)
	}
	if err := os.WriteFile(path, []byte("ANTHROPIC_API_KEY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadKey(path); err == nil {
		t.Error("an empty key was accepted")
	}
}
