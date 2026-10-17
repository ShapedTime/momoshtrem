package swarm

// Pause stops the seeder sending piece data, as a peer that chokes us or
// stops serving does: it stays connected, keeps its pieces and answers
// nothing. Resume lets it serve again. A scenario that pauses a seeder
// mid-play makes the stall the lab's watchdog and findings must explain
// (observe, PLAN §5).
//
// It disallows the torrent's data upload (anacrolix chokes every peer)
// rather than throttling: anacrolix panics when the upload limiter's burst
// can't fit a chunk, and a limiter slowed to a trickle schedules the next
// send hours away, past the Resume.
func (s *Seeder) Pause() {
	s.paused.Store(true)
	s.t.DisallowDataUpload()
}

// Resume undoes Pause: the seeder unchokes its peers and serves again.
func (s *Seeder) Resume() {
	s.t.AllowDataUpload()
	s.paused.Store(false)
}

// Paused reports whether the seeder is paused: a leecher connected to it
// gets nothing, which no known issue explains.
func (s *Seeder) Paused() bool { return s.paused.Load() }
