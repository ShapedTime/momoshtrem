package report

import (
	"bufio"
	"mime"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// CollectEnv describes this process: the machine, the Go build (cgo,
// race, tags, from the build settings) and the MIME table.
func CollectEnv() Env {
	e := Env{
		Go:         runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CPU:        cpuModel(),
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		MIME:       map[string]string{".mkv": mime.TypeByExtension(".mkv"), ".mp4": mime.TypeByExtension(".mp4")},
		Load:       loadAvg(),
	}
	e.Host, _ = os.Hostname()
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "CGO_ENABLED":
				e.CGO = s.Value == "1"
			case "-race":
				e.Race = s.Value == "true"
			case "-tags":
				e.Tags = s.Value
			}
		}
	}
	// The race build tag holds where the build settings may not (a test
	// binary): compare (S8) refuses timing verdicts for race builds.
	e.Race = e.Race || raceBuild
	e.UTP = utpBuild(e.CGO, e.Tags)
	return e
}

// utpBuild names the uTP implementation the build links: anacrolix's pure
// Go one without cgo or with disable_libutp (as production, PLAN D3),
// libutp otherwise.
func utpBuild(cgo bool, tags string) string {
	if !cgo || strings.Contains(","+tags+",", ",disable_libutp,") {
		return "go (unused: the lab runs TCP)"
	}
	return "libutp (unused: the lab runs TCP)"
}

// cpuModel is the CPU's name, best effort.
func cpuModel() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "linux":
		f, err := os.Open("/proc/cpuinfo")
		if err != nil {
			return ""
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if k, v, ok := strings.Cut(sc.Text(), ":"); ok && strings.TrimSpace(k) == "model name" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// loadAvg is the 1, 5 and 15 minute load averages, best effort.
func loadAvg() []float64 {
	var fields []string
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
		if err != nil {
			return nil
		}
		fields = strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{}"))
	case "linux":
		b, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			return nil
		}
		fields = strings.Fields(string(b))
	}
	if len(fields) < 3 {
		return nil
	}
	var out []float64
	for _, f := range fields[:3] {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// ToolInfo is the tool's name, version and the tree's git state: from git
// in dir, else from the build's stamp. git comes first because go build
// stamps the wrong tree for a git worktree nested in its main checkout (it
// looks for a .git folder, and a worktree has a .git file), and go run
// stamps nothing.
func ToolInfo(dir string) Tool {
	t := Tool{Name: "streamlab", Version: "dev"}
	t.Git.Commit = git(dir, "rev-parse", "HEAD")
	if t.Git.Commit != "" {
		t.Git.Dirty = git(dir, "status", "--porcelain") != ""
		t.Git.Branch = git(dir, "rev-parse", "--abbrev-ref", "HEAD")
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			t.Version = v
		}
		for _, s := range bi.Settings {
			switch {
			case t.Git.Commit != "":
			case s.Key == "vcs.revision":
				t.Git.Commit = s.Value
			case s.Key == "vcs.modified":
				t.Git.Dirty = s.Value == "true"
			}
		}
	}
	return t
}

func git(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
