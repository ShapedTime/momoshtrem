package downloads

import (
	"slices"
	"testing"
)

// The downloading set is the wanted files that still want bytes: not the
// ones on disk, nor a paused download's.
func TestSnapshot_DownloadingFiles(t *testing.T) {
	var none *Snapshot
	if got := none.DownloadingFiles(); got != nil {
		t.Errorf("nil snapshot = %v, want nil", got)
	}

	s := &Snapshot{files: map[FileKey]FileStatus{
		{"bb", "Show/E02.mkv"}:  {Bytes: 10, DoneBytes: 3, State: StateDownloading},
		{"bb", "Show/E01.mkv"}:  {Bytes: 10, DoneBytes: 0, State: StateQueued},
		{"bb", "Show/E03.mkv"}:  {Bytes: 10, DoneBytes: 10, Complete: true, State: StateDone},
		{"aa", "Movie.mkv"}:     {Bytes: 10, DoneBytes: 2, State: StateWaitingForWindow},
		{"cc", "Paused.mkv"}:    {Bytes: 10, DoneBytes: 2, State: StatePaused},
		{"dd", "DiskFull.mkv"}:  {Bytes: 10, State: StateDiskFull},
		{"ee", "Stalled.mkv"}:   {Bytes: 10, State: StateStalled},
		{"ff", "Streamed.mkv"}:  {Bytes: 10, State: StateWaitingForStream},
		{"00", "Over-cap.mkv"}:  {Bytes: 10, State: StateStorageFull},
		{"01", "Nothing.mkv"}:   {Bytes: 10, State: StateNoFile},
		{"02", "Elsewhere.mkv"}: {Bytes: 10, State: StateUnavailable},
	}}
	want := []FileKey{
		{"aa", "Movie.mkv"}, {"bb", "Show/E01.mkv"}, {"bb", "Show/E02.mkv"},
		{"dd", "DiskFull.mkv"}, {"ee", "Stalled.mkv"}, {"ff", "Streamed.mkv"},
	}
	if got := s.DownloadingFiles(); !slices.Equal(got, want) {
		t.Errorf("downloading files =\n%v\nwant\n%v", got, want)
	}
}
