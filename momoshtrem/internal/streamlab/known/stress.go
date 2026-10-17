package known

// DeleteDataDeadlock is #230: a RemoveTorrent with its data, during a
// piece hash, deadlocks with anacrolix's hasher and wedges the torrent's
// hash lock for good. The stress tier (S7) found it.
const DeleteDataDeadlock = "#230"

// The stress tier's known issues, registered from this file so the
// registry's list stays the gate's.
func init() {
	Issues = append(Issues, Issue{
		ID:      DeleteDataDeadlock,
		Title:   "Removing a torrent with its data while a piece hashes deadlocks it until restart",
		Ref:     "#230",
		Checks:  []string{"ops-bounded", "quiescence"},
		MaxRate: Always,
		Doc: "RemoveTorrent(hash, true) runs deletePieces right after t.Drop(), and MarkNotComplete takes the piece's RWMutex " +
			"for writing. anacrolix v1.60.0's piecePerResourcePiece.WriteTo, the piece hasher's read, holds the same lock for " +
			"reading and takes it again for each chunk (ReadAt, NewReader): once the writer waits, neither goes on. The remove " +
			"keeps the torrent's hash lock, so every add, open or remove of it waits too. Its ops-bounded signature: every " +
			"stuck call is a RemoveTorrent or an Open, and its goroutine dump shows a deletePieces MarkNotComplete blocked in " +
			"RWMutex.Lock and a WriteTo NewReader blocked in RWMutex.RLock of the same mutex. Its quiescence signature: that " +
			"cycle was seen, and every goroutine left is the remove or an add, open or remove queued behind its hash lock " +
			"(#225's orphaned reads aside). remove-while-hashing guards it.",
	})
}
