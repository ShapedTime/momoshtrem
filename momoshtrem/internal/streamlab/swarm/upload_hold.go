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
// It's the stress tier's minimal knob (S7) and the observer's injected
// stall (S5); S6's proxy (#177) models richer faults. A scenario that
// holds a seeder on purpose declares it (scenario.Env.HoldSeeder): the
// watchdog then doesn't trip, and a stall then isn't #195's.
func (s *Seeder) HoldUploads() {
	s.held.Store(true)
	s.t.DisallowDataUpload()
}

// ReleaseUploads lets the seeder send data again: anacrolix unchokes the
// leecher at the seeder's next write. Like any writer wakeup it can be
// lost (#195): the connection then recovers at its 1-minute keep-alive.
func (s *Seeder) ReleaseUploads() {
	s.t.AllowDataUpload()
	s.held.Store(false)
}

// Held reports whether the seeder's uploads are held: a leecher connected
// to it gets nothing, which no known issue explains.
func (s *Seeder) Held() bool { return s.held.Load() }
