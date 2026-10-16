package checks

import (
	"context"
	"fmt"
	"mime"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// The probes record what today's code does about a suspected bug (PLAN
// §3). They pass or fail nothing yet: S4's known-issue registry (#175)
// turns their results into expected failures.

// F3Result is what one PROPFIND did to the library's torrents (F3, #83).
type F3Result struct {
	Dir     string
	Entries int // resources the PROPFIND listed, the folder included
	// Before are the torrents loaded before the PROPFIND, and Loaded the
	// ones it loaded.
	Before []string
	Loaded []string
}

// LoadedByPropfind reports whether the PROPFIND loaded a torrent. Today it
// does: x/net's PROPFIND opens every file it lists, and opening a
// torrent's file adds the torrent (vfs.LibraryFS.openTorrentFile).
func (r F3Result) LoadedByPropfind() bool { return len(r.Loaded) > 0 }

func (r F3Result) String() string {
	if r.LoadedByPropfind() {
		return fmt.Sprintf("torrent loaded by PROPFIND (Depth 1 of %s, %d entries): %s",
			r.Dir, r.Entries, strings.Join(r.Loaded, ", "))
	}
	return fmt.Sprintf("no torrent loaded by PROPFIND (Depth 1 of %s, %d entries)", r.Dir, r.Entries)
}

// F3 lists dir (Depth 1) through conn, as a player browsing a movie or a
// season folder does, and reports which of the library's torrents that
// loaded.
func F3(ctx context.Context, s *stack.Stack, conn *client.Conn, dir string) (F3Result, error) {
	r := F3Result{Dir: dir, Before: loaded(s)}
	entries, err := conn.Propfind(ctx, dir, 1)
	if err != nil {
		return r, err
	}
	r.Entries = len(entries)
	for _, h := range loaded(s) {
		if !slices.Contains(r.Before, h) {
			r.Loaded = append(r.Loaded, h)
		}
	}
	return r, nil
}

// loaded returns the library's torrents the torrent service has loaded.
func loaded(s *stack.Stack) []string {
	var out []string
	for _, h := range s.Torrents() {
		if _, err := s.Service.GetTorrent(h); err == nil {
			out = append(out, h)
		}
	}
	return out
}

// MIMEResult is what this machine's MIME table says of the lab's video
// extensions (F1).
type MIMEResult struct {
	MKV, MP4 string // "" when the extension doesn't resolve
}

// MIME looks the video extensions up in the local MIME table. When .mkv
// doesn't resolve, http.ServeContent sniffs 512 bytes at offset 0 of every
// GET and HEAD, and PROPFIND's getcontenttype does too, per file. macOS
// and most Linux hosts resolve it (their mime.types lists it); production's
// alpine:3 image has no /etc/mime.types, and Go's built-in table lacks
// .mkv. So where .mkv resolves, the reads at offset 0 seen here are fewer
// than production's.
func MIME() MIMEResult {
	return MIMEResult{MKV: mime.TypeByExtension(".mkv"), MP4: mime.TypeByExtension(".mp4")}
}

// ResolvesMKV reports whether .mkv resolves here, unlike in production.
func (r MIMEResult) ResolvesMKV() bool { return r.MKV != "" }

func (r MIMEResult) String() string {
	or := func(t string) string {
		if t == "" {
			return "unresolved"
		}
		return t
	}
	note := "sniffed at offset 0, as in production"
	if r.ResolvesMKV() {
		note = "not sniffed here; production's Alpine image sniffs it at offset 0"
	}
	return fmt.Sprintf(".mkv: %s (%s); .mp4: %s", or(r.MKV), note, or(r.MP4))
}
