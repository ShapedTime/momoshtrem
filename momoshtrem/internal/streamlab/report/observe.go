package report

import (
	"fmt"
	"slices"
	"strings"
)

// Observation is what the lab's observer (internal/streamlab/observe, PLAN
// §5 "observe/") saw of a repetition: its own cost, the pieces against the
// readers, lock contention, the logs and the watchdog. Times are from the
// observer's start, just after the stack started.
type Observation struct {
	// Diagnose: --diagnose, DEBUG logs of the streaming components and the
	// flight recorder on all along; timings aren't faithful then.
	Diagnose bool `json:"diagnose"`
	// Samplers are each sampler's cost. OverheadPct is their busy time
	// over the process's CPU time, in percent: the budget is 2%.
	Samplers     []SamplerStat `json:"samplers"`
	OverheadPct  float64       `json:"overhead_pct"`
	ProcessCPUMS float64       `json:"process_cpu_ms"`
	WallMS       float64       `json:"wall_ms"`
	// Timeline sums up each title's piece arrival against its readers.
	Timeline []TitleTimeline `json:"timeline,omitempty"`
	// Mutex and Block are the most contended sites of the mutex and block
	// profiles; ClientLockMS and SetPriorityMS the mutex delay under the
	// anacrolix client lock and under piece SetPriority calls (F7).
	Mutex         []ContentionSite `json:"mutex,omitempty"`
	Block         []ContentionSite `json:"block,omitempty"`
	ClientLockMS  float64          `json:"client_lock_ms"`
	SetPriorityMS float64          `json:"set_priority_ms"`
	Logs          LogSummary       `json:"logs"`
	Trips         []Trip           `json:"watchdog_trips,omitempty"`
	Faults        []Fault          `json:"faults,omitempty"`
}

// SamplerStat is one sampler's cost.
type SamplerStat struct {
	Name       string  `json:"name"`
	EveryMS    float64 `json:"every_ms"`
	Samples    int     `json:"samples"`
	BusyMS     float64 `json:"busy_ms"`
	MaxMS      float64 `json:"max_ms"`
	CPUPct     float64 `json:"cpu_pct"`                     // BusyMS over the process's CPU time
	ClientLock bool    `json:"takes_client_lock,omitempty"` // it takes the anacrolix client lock
}

// TitleTimeline is a title's pieces against its server-side readers.
type TitleTimeline struct {
	Title  string `json:"title"`
	Pieces int    `json:"pieces"`
	// Completed pieces arrived during the run: Ahead before any reader
	// needed them (never at Now while incomplete), Waited after a reader
	// had raised them to Now; WaitP50MS and WaitP95MS are those from Now
	// to complete.
	Completed int     `json:"completed"`
	Ahead     int     `json:"ahead"`
	Waited    int     `json:"waited"`
	WaitP50MS float64 `json:"wait_p50_ms"`
	WaitP95MS float64 `json:"wait_p95_ms"`
	// ReaderSamples are the readers' positions sampled; AtIncomplete those
	// where the reader's piece was incomplete (it waited, or will). Lead
	// is the complete data ahead of a reader, from its position.
	ReaderSamples int   `json:"reader_samples"`
	AtIncomplete  int   `json:"at_incomplete"`
	LeadP50       int64 `json:"lead_p50"`
	LeadMin       int64 `json:"lead_min"`
	// Plot is the timeline's SVG, relative to the repetition's folder.
	Plot string `json:"plot,omitempty"`
}

// ContentionSite is where a profile's contention or blocking was: the
// first frame past the lock, its callers, and what it cost.
type ContentionSite struct {
	Site    string   `json:"site"`
	Stack   []string `json:"stack"`
	Count   int64    `json:"count"`
	DelayMS float64  `json:"delay_ms"`
	// Note flags the anacrolix client lock and SetPriority (F7).
	Note string `json:"note,omitempty"`
}

// LogSummary counts what the capture recorded.
type LogSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Debug    int `json:"debug,omitempty"` // below WARN: --diagnose's records
	Dropped  int `json:"dropped,omitempty"`
	// BenignStorageErrors are anacrolix reads that logged at ERROR and
	// then succeeded on a retry (#219); ReadFailures those that gave up
	// after the completion resync: data the storage lost (F11).
	BenignStorageErrors int        `json:"benign_storage_errors"`
	ReadFailures        int        `json:"read_failures"`
	Top                 []LogCount `json:"top,omitempty"`
}

// LogCount is how often one message was logged.
type LogCount struct {
	Level     string `json:"level"`
	Component string `json:"component,omitempty"`
	Msg       string `json:"msg"`
	Count     int    `json:"count"`
}

// Trip is a watchdog trip: requests active, a read blocked, no fault, and
// no progress for ForMS.
type Trip struct {
	AtMS  float64 `json:"at_ms"`
	ForMS float64 `json:"for_ms"`
	// Readers are the server readers at the trip, the stuck ones first.
	Readers []TripReader `json:"readers"`
	// Known is "#195" when the trip carries its signature; Why says why
	// it does or doesn't.
	Known     string   `json:"known,omitempty"`
	Why       string   `json:"why"`
	Artifacts []string `json:"artifacts,omitempty"`
}

// TripReader is a server reader at a watchdog trip.
type TripReader struct {
	Title    string `json:"title"`
	Position int64  `json:"position"`
	Piece    int    `json:"piece"`
	// State is the piece's: "complete", or its effective priority and
	// "partial" when some of it arrived.
	State string `json:"state"`
}

// Fault is a fault a scenario declared: the watchdog doesn't trip during
// one.
type Fault struct {
	Name string `json:"name"`
	// Torrent is the torrent it starves; "" is every torrent.
	Torrent string  `json:"torrent,omitempty"`
	StartMS float64 `json:"start_ms"`
	EndMS   float64 `json:"end_ms"`
}

// Finding is a hint the observer drew from the data, pointing at its
// evidence. Findings never change a verdict.
type Finding struct {
	// Rule names the rule that found it (stall-below-now, starved, ...).
	Rule string `json:"rule"`
	// Known names the known issue it points at, if any.
	Known    string  `json:"known,omitempty"`
	Hint     string  `json:"hint"`
	Evidence string  `json:"evidence"`
	AtMS     float64 `json:"at_ms"`
	// Refs are artifacts (relative to the repetition's folder) and request
	// IDs that hold the evidence.
	Refs []string `json:"refs,omitempty"`
}

// FindingLines sums up the repetitions' findings for the report: one line
// per rule and hint, with the repetitions that found it.
func FindingLines(reps []Rep) []string {
	type key struct{ rule, hint string }
	var order []key
	seen := map[key][]string{}
	for _, rep := range reps {
		for _, f := range rep.Findings {
			k := key{f.Rule, f.Hint}
			if _, ok := seen[k]; !ok {
				order = append(order, k)
			}
			name := fmt.Sprint(rep.Rep)
			if !slices.Contains(seen[k], name) {
				seen[k] = append(seen[k], name)
			}
		}
	}
	out := []string{}
	for _, k := range order {
		out = append(out, fmt.Sprintf("%s: %s (rep %s)", k.rule, k.hint, strings.Join(seen[k], ", ")))
	}
	return out
}

// findingsMarkdown lists the findings over the repetitions.
func (r *Report) findingsMarkdown() string {
	if len(r.Findings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Findings\n\nHints drawn from the observation; they never change the verdict. Each repetition's evidence is in its rep.json and artifacts.\n\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	return b.String()
}

// observationMarkdown is a repetition's observation: the samplers' cost,
// the piece timeline, contention, logs, watchdog trips and findings.
func observationMarkdown(rep *Rep) string {
	o := rep.Observe
	if o == nil {
		return ""
	}
	var b strings.Builder
	mode := "measure mode"
	if o.Diagnose {
		mode = "--diagnose: DEBUG logs and the flight recorder on, timings not faithful"
	}
	fmt.Fprintf(&b, "\n## Observation (rep %d, %s)\n\n", rep.Rep, mode)
	fmt.Fprintf(&b, "**Cost:** the samplers took %.2f%% of the process's CPU time (%s ms of %s ms; budget 2%%). "+
		"That's their own busy time: the mutex and block profiles' cost inside the runtime isn't in it, and only an A/B against "+
		"`--set observe=false` measures it.\n\n",
		o.OverheadPct, Num(sumBusy(o.Samplers)), Num(o.ProcessCPUMS))
	b.WriteString("| Sampler | Every ms | Samples | Busy ms | Max ms | CPU % | Client lock |\n|---|---|---|---|---|---|---|\n")
	for _, s := range o.Samplers {
		every := Num(s.EveryMS)
		if s.EveryMS == 0 {
			every = "events" // the piece mirror and the log capture
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %.3f | %v |\n", s.Name, every, s.Samples, Num(s.BusyMS), Num(s.MaxMS), s.CPUPct, s.ClientLock)
	}

	if len(o.Timeline) > 0 {
		b.WriteString("\n### Piece timeline\n\nPieces against the server's readers. Ahead: complete before a reader needed it; waited: raised to Now incomplete, with the time from Now to complete. A reader at an incomplete piece is waiting for it, or about to.\n\n")
		b.WriteString("| Title | Pieces | Completed | Ahead | Waited | Now→complete p50 / p95 ms | Reader samples | At an incomplete piece | Lead p50 / min | Plot |\n|---|---|---|---|---|---|---|---|---|---|\n")
		for _, t := range o.Timeline {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %s / %s | %d | %d | %s / %s | %s |\n", cell(t.Title), t.Pieces, t.Completed, t.Ahead, t.Waited,
				Num(t.WaitP50MS), Num(t.WaitP95MS), t.ReaderSamples, t.AtIncomplete, Bytes(t.LeadP50), Bytes(t.LeadMin), orDash(t.Plot))
		}
	}

	if len(o.Mutex) > 0 || len(o.Block) > 0 {
		fmt.Fprintf(&b, "\n### Contention\n\nMutex delay under the anacrolix client lock: %s ms; under piece SetPriority calls (F7): %s ms.\n",
			Num(o.ClientLockMS), Num(o.SetPriorityMS))
		for _, p := range []struct {
			name  string
			sites []ContentionSite
		}{{"Mutex", o.Mutex}, {"Block", o.Block}} {
			if len(p.sites) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n| %s site | Count | Delay ms | Note | Callers |\n|---|---|---|---|---|\n", p.name)
			for _, s := range p.sites {
				fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n", cell(s.Site), s.Count, Num(s.DelayMS), orDash(s.Note), cell(strings.Join(s.Stack, " ← ")))
			}
		}
	}

	l := o.Logs
	fmt.Fprintf(&b, "\n### Logs\n\n%d errors, %d warnings captured", l.Errors, l.Warnings)
	if l.Debug > 0 {
		fmt.Fprintf(&b, ", %d DEBUG/INFO records (--diagnose)", l.Debug)
	}
	if l.Dropped > 0 {
		fmt.Fprintf(&b, ", %d dropped past the cap", l.Dropped)
	}
	fmt.Fprintf(&b, "; %d benign storage errors (reads that logged ERROR and recovered, #219), %d reads that failed after the completion resync (F11).\n",
		l.BenignStorageErrors, l.ReadFailures)
	if len(l.Top) > 0 {
		b.WriteString("\n| Level | Component | Message | Count |\n|---|---|---|---|\n")
		for _, c := range l.Top {
			fmt.Fprintf(&b, "| %s | %s | %s | %d |\n", c.Level, orDash(c.Component), cell(c.Msg), c.Count)
		}
	}

	if len(o.Faults) > 0 {
		b.WriteString("\n**Faults:**")
		for _, f := range o.Faults {
			fmt.Fprintf(&b, " %s %s-%s ms;", f.Name, Num(f.StartMS), Num(f.EndMS))
		}
		b.WriteString("\n")
	}
	if len(o.Trips) > 0 {
		b.WriteString("\n### Watchdog\n\n")
		for _, t := range o.Trips {
			var rs []string
			for _, r := range t.Readers {
				rs = append(rs, fmt.Sprintf("%s at %d (piece %d, %s)", r.Title, r.Position, r.Piece, r.State))
			}
			fmt.Fprintf(&b, "- at %s ms, no progress for %s ms: %s. %s. Dumps: %s\n", Num(t.AtMS), Num(t.ForMS),
				orDash(strings.Join(rs, "; ")), t.Why, orDash(strings.Join(t.Artifacts, ", ")))
		}
	}
	if len(rep.Findings) > 0 {
		b.WriteString("\n### Findings\n\n")
		for _, f := range rep.Findings {
			known := ""
			if f.Known != "" {
				known = " (" + f.Known + ")"
			}
			refs := ""
			if len(f.Refs) > 0 {
				refs = " See " + strings.Join(f.Refs, ", ") + "."
			}
			fmt.Fprintf(&b, "- **%s**%s at %s ms: %s. Evidence: %s.%s\n", f.Rule, known, Num(f.AtMS), f.Hint, f.Evidence, refs)
		}
	}
	return b.String()
}

func sumBusy(ss []SamplerStat) float64 {
	var n float64
	for _, s := range ss {
		n += s.BusyMS
	}
	return n
}
