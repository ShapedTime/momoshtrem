// Command classifier-eval measures episode mapping on the corpus's golden
// torrents (internal/identify/testdata, the cases marked golden: true) in
// three modes:
//   - rules: the rules alone;
//   - auto: the AI on the files the rules leave unsure (classifier_mode
//     auto);
//   - ai_always: the AI on every video (classifier_mode ai_always);
//   - ai_blind (only with -modes): ai_always without the rules' answers in
//     the prompt, a diagnostic of what the AI does where the rules have
//     nothing.
//
// For each mode it reports file-level accuracy against the corpus's
// expectations, after identify.MergeAI as the job service merges the
// answers; the files the AI changed and whether each change was right;
// tokens; cost per torrent; and latency. For ai_always it also scores the
// AI's own answers, before MergeAI keeps the rules' high ones.
//
// The AI modes call the Anthropic API and cost money, so this never runs in
// tests or CI. They need ANTHROPIC_API_KEY, from the environment or read
// from a .env file with -env-file; -dry-run prints the requests' sizes (and
// with -prompt their text) without calling the API.
//
//	go run ./cmd/classifier-eval -env-file ../.env [-modes rules,auto,ai_always] [-dry-run] [-v]
//
// The gate (PLAN B3) is at least 95% file-level accuracy in ai_always mode,
// with auto no worse than rules. It exits 1 when the gate fails.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
)

func main() {
	var (
		testdata = flag.String("testdata", "internal/identify/testdata", "the corpus's testdata directory")
		modes    = flag.String("modes", "rules,auto,ai_always", "modes to run, comma-separated")
		model    = flag.String("model", ai.DefaultModel, "the model the AI modes use")
		cases    = flag.String("cases", "", "only these golden cases, comma-separated ids")
		parallel = flag.Int("parallel", 4, "torrents resolved at once")
		dryRun   = flag.Bool("dry-run", false, "print request sizes instead of calling the API")
		prompt   = flag.Bool("prompt", false, "with -dry-run, print each request's user message too")
		verbose  = flag.Bool("v", false, "list every AI change and every miss")
		envFile  = flag.String("env-file", "", "read ANTHROPIC_API_KEY from this .env file when the environment doesn't set it")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg := config{
		testdata: *testdata, modes: splitList(*modes), cases: splitList(*cases),
		parallel: max(*parallel, 1), verbose: *verbose,
	}
	if *envFile != "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		if err := loadKey(*envFile); err != nil {
			fmt.Fprintln(os.Stderr, "classifier-eval:", err)
			os.Exit(2)
		}
	}
	err := run(ctx, os.Stdout, cfg, *model, *dryRun, *prompt)
	if errors.Is(err, errGate) {
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "classifier-eval:", err)
		os.Exit(2)
	}
}

var errGate = errors.New("the gate failed")

func run(ctx context.Context, w io.Writer, cfg config, model string, dryRun, prompt bool) error {
	for _, m := range cfg.modes {
		if m != modeRules && m != modeAuto && m != modeAlways && m != modeBlind {
			return fmt.Errorf("unknown mode %q (rules, auto, ai_always, ai_blind)", m)
		}
	}
	runs, err := loadGolden(cfg.testdata, cfg.cases)
	if err != nil {
		return err
	}
	if dryRun {
		printDryRun(w, runs, cfg.modes, prompt)
		return nil
	}

	var resolver identify.Resolver
	if cfg.needsAI() {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return errors.New("ANTHROPIC_API_KEY is not set: the AI modes need it (or use -modes rules, or -dry-run)")
		}
		r, err := ai.New(ai.Config{APIKey: key, Model: model})
		if err != nil {
			return err
		}
		resolver = r
	}

	rep := evaluate(ctx, runs, cfg, resolver)
	rep.model = model
	rep.print(w, cfg.verbose)
	if !rep.gate(w) {
		return errGate
	}
	return nil
}

// loadKey sets ANTHROPIC_API_KEY from a .env file's ANTHROPIC_API_KEY=
// line (quotes optional). Nothing else is read, and the key is never
// printed.
func loadKey(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		value, ok := strings.CutPrefix(line, "ANTHROPIC_API_KEY=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if value == "" {
			break
		}
		return os.Setenv("ANTHROPIC_API_KEY", value)
	}
	return fmt.Errorf("%s has no ANTHROPIC_API_KEY", path)
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
