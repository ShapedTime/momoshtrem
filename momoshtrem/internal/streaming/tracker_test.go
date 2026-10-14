package streaming

import "testing"

func TestReaderTracker_Count(t *testing.T) {
	rt := NewReaderTracker()
	a, b, c := &PriorityReader{}, &PriorityReader{}, &PriorityReader{}

	if got := rt.Count(); got != 0 {
		t.Fatalf("empty tracker Count = %d, want 0", got)
	}

	rt.Register("hash1", a)
	rt.Register("hash1", b)
	rt.Register("hash2", c)
	if got := rt.Count(); got != 3 {
		t.Errorf("Count = %d, want 3", got)
	}

	rt.Unregister("hash1", a)
	rt.Unregister("hash2", c)
	if got := rt.Count(); got != 1 {
		t.Errorf("Count after unregister = %d, want 1", got)
	}
}
