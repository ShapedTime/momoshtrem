package swarm

import atorrent "github.com/anacrolix/torrent"

// StallIssue tracks reads that stall with one connected seeder: anacrolix
// v1.60.0's connection writer loses a wakeup (anacrolix/torrent#1070), and
// the read waits up to the 1-minute keep-alive.
const StallIssue = "#195"

// Stalled reports whether a read that timed out on piece of t is the known
// stall (StallIssue): the leecher is connected to a seeder, and the piece
// is wanted now (anacrolix's effective priority Now, which counts its own
// readers and is None while downloads are disallowed) but incomplete. A
// torrent left idle or paused, or with no seeder connected, isn't the
// known stall.
//
// It's an expected failure (PLAN D6) until the issue is fixed; S4's
// known-issue registry (#175) replaces this.
func Stalled(t *atorrent.Torrent, piece int) bool {
	if t == nil || t.Info() == nil || piece < 0 || piece >= t.NumPieces() || t.Stats().ConnectedSeeders < 1 {
		return false
	}
	ps := t.PieceState(piece)
	return ps.Priority == atorrent.PiecePriorityNow && !ps.Complete
}
