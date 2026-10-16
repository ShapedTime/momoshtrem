package streaming

import "github.com/anacrolix/torrent"

// BufferedAhead returns how many bytes of f, from off on, are complete and
// contiguous: what a reader at off can read without waiting for peers. 0
// when the piece at off isn't complete, or before the torrent has its info.
func BufferedAhead(t *torrent.Torrent, f *torrent.File, off int64) int64 {
	info := t.Info()
	if info == nil || off < 0 || off >= f.Length() {
		return 0
	}
	runs := t.PieceStateRuns()
	complete := make([]pieceRun, len(runs))
	for i, r := range runs {
		complete[i] = pieceRun{complete: r.Complete, length: r.Length}
	}
	return bufferedAhead(complete, info.PieceLength, f.Offset()+off, f.Offset()+f.Length())
}

// pieceRun is consecutive pieces that are all complete, or all not.
type pieceRun struct {
	complete bool
	length   int
}

// bufferedAhead is the complete bytes contiguous from the torrent offset
// from up to end (the file's end), over runs in piece order.
func bufferedAhead(runs []pieceRun, pieceLength, from, end int64) int64 {
	if pieceLength <= 0 || from >= end {
		return 0
	}
	first := from / pieceLength // the piece at from
	var piece int64             // the first piece of the run
	for i, r := range runs {
		next := piece + int64(r.length)
		if first >= next {
			piece = next
			continue
		}
		if !r.complete {
			return 0
		}
		// Complete from here: add the complete runs after it.
		last := next
		for _, more := range runs[i+1:] {
			if !more.complete {
				break
			}
			last += int64(more.length)
		}
		return min(last*pieceLength, end) - from
	}
	return 0
}
