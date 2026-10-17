package swarm

// HoldUploads stops the seeder sending data, as a peer that has gone
// quiet does: anacrolix chokes the leecher at the seeder's next write
// (DisallowDataUpload tickles every connection's writer), and requests
// wait. Reads of pieces the leecher lacks block until ReleaseUploads, or
// until their read timeout. The connections stay up.
//
// Hold a seeder only once the leecher is connected: a complete torrent
// that may not upload wants no connection, so anacrolix refuses the
// leecher's dial, and the leecher doesn't dial again after the release.
// It's the stress tier's minimal knob (S7); S6's proxy (#177) models
// richer faults.
func (s *Seeder) HoldUploads() { s.t.DisallowDataUpload() }

// ReleaseUploads lets the seeder send data again: anacrolix unchokes the
// leecher at the seeder's next write. Like any writer wakeup it can be
// lost (#195): the connection then recovers at its 1-minute keep-alive.
func (s *Seeder) ReleaseUploads() { s.t.AllowDataUpload() }
