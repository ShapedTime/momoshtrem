package runner

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/shapedtime/momoshtrem/internal/streamlab/observe"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// Sets is a repeatable --set k=v flag.
type Sets map[string]string

func (s Sets) String() string {
	var out []string
	for k, v := range s {
		out = append(out, k+"="+v)
	}
	return strings.Join(out, ",")
}

// Set parses one k=v.
func (s Sets) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("want k=v, got %q", v)
	}
	s[k] = val
	return nil
}

// ParentPipeEnv, set to 1, tells a child that its stdin is a pipe its
// parent holds open: EOF means the parent died, and the child quits with
// a goroutine dump rather than run on as an orphan.
const ParentPipeEnv = "STREAMLAB_PARENT_PIPE"

// RunChild runs one repetition in this process, as the runner asks with
// ChildCommand: it writes rep.json to --out and returns the exit code.
// Logs go to stderr, which the runner keeps as child.log.
func RunChild(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(ChildCommand, flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("scenario", "", "the scenario")
	rep := fs.Int("rep", 1, "the repetition's number (0 and below: warm-up)")
	out := fs.String("out", "", "the repetition's folder")
	configPath := fs.String("config", "", "a YAML config laid over the defaults")
	diagnose := fs.Bool("diagnose", false, "observe in depth: DEBUG logs of the streaming components, full-rate profiles, the flight recorder")
	sets := Sets{}
	fs.Var(sets, "set", "a param override, k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		return ChildToolError
	}
	// The capture records log records as lab events (observe); it must be
	// the default before the stack builds its loggers.
	logs := observe.NewCapture(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: logLevel()}), *diagnose)
	slog.SetDefault(slog.New(logs))
	if os.Getenv(ParentPipeEnv) == "1" {
		go watchParent()
	}

	sc, ok := scenario.Lookup(*name)
	if !ok || *out == "" {
		fmt.Fprintf(stderr, "%s: unknown scenario %q or no --out\n", ChildCommand, *name)
		return ChildToolError
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r, err := scenario.Run(ctx, sc, scenario.RunOptions{Rep: *rep, Sets: sets, ConfigPath: *configPath, Dir: *out,
		Logs: logs, Diagnose: *diagnose})
	code := ChildOK
	if err != nil {
		var te *scenario.ToolError
		if r == nil || errors.As(err, &te) {
			r = &report.Rep{Rep: *rep, Status: report.RepError, Error: err.Error()}
		} else {
			r.Status, r.Error = report.RepError, err.Error()
		}
		code = ChildToolError
		fmt.Fprintf(stderr, "%s: %v\n", ChildCommand, err)
	}
	env, tool := report.CollectEnv(), report.ToolInfo(".")
	r.Env, r.Tool = &env, &tool
	if err := report.WriteJSON(filepath.Join(*out, "rep.json"), r); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", ChildCommand, err)
		return ChildToolError
	}
	return code
}

// logLevel is STREAMLAB_LOG (debug, info, warn, error); info by default.
func logLevel() slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(os.Getenv("STREAMLAB_LOG"))); err != nil {
		return slog.LevelInfo
	}
	return l
}
