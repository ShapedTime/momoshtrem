package piececache

import (
	"testing"
	"time"
)

// Upstream's lru_test.go, policy_test.go and lruitems_test.go, ported from
// testify to the standard library.

func TestLRU(t *testing.T) {
	testPolicy(t, &lru{})
}

func TestLruDuplicateAccessTimes(t *testing.T) {
	var li Policy = new(lru)
	now := time.Now()
	li.Used(key("a"), now)
	li.Used(key("b"), now)
	if n := li.NumItems(); n != 2 {
		t.Errorf("NumItems = %d, want 2", n)
	}
}

func panics(f func()) (did bool) {
	defer func() {
		if recover() != nil {
			did = true
		}
	}()
	f()
	return
}

func testChooseForgottenKey(t *testing.T, p Policy) {
	t.Helper()
	wantItems := func(want int) {
		t.Helper()
		if n := p.NumItems(); n != want {
			t.Errorf("NumItems = %d, want %d", n, want)
		}
	}
	wantItems(0)
	if !panics(func() { p.Choose() }) {
		t.Error("Choose on an empty policy didn't panic")
	}
	p.Used(key("a"), time.Now())
	wantItems(1)
	p.Used(key("a"), time.Now().Add(1))
	wantItems(1)
	p.Forget(key("a"))
	wantItems(0)
	if !panics(func() { p.Choose() }) {
		t.Error("Choose on an emptied policy didn't panic")
	}
}

func testPolicy(t *testing.T, p Policy) {
	testChooseForgottenKey(t, p)
}
