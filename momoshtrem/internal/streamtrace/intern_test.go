package streamtrace

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestIntern(t *testing.T) {
	r := New(8)
	if r.Intern("/a.mkv") != 0 {
		t.Fatal("off: Intern returned an ID")
	}
	if n := r.Stats().Strings; n != 0 {
		t.Fatalf("off: %d strings kept", n)
	}

	r.SetLevel(LevelSummary)
	a, b := r.Intern("/a.mkv"), r.Intern("/b.mkv")
	if a == 0 || b == 0 || a == b {
		t.Fatalf("IDs %d, %d", a, b)
	}
	buf := []byte("/a.mkv")
	if again := r.Intern(string(buf)); again != a {
		t.Fatalf("same string, new ID: %d, %d", again, a)
	}
	buf[1] = 'z' // the table kept its own copy
	if got := r.strs.snapshot()[a-1]; got != "/a.mkv" {
		t.Fatalf("interned string changed to %q", got)
	}
	if r.Intern("") != 0 {
		t.Fatal(`Intern("") isn't 0`)
	}
	if n := r.Stats().Strings; n != 2 {
		t.Fatalf("%d strings, want 2", n)
	}
}

func TestInternCutsLongStrings(t *testing.T) {
	r := New(8)
	r.SetLevel(LevelSummary)
	long := strings.Repeat("€", 200) // 600 bytes; byte 512 is mid-rune
	id := r.Intern(long)
	got := r.strs.snapshot()[id-1]
	if len(got) != 510 || !utf8.ValidString(got) || !strings.HasPrefix(long, got) {
		t.Fatalf("cut to %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
	if r.Intern(long[:510]+"tail") == id {
		t.Fatal("a different short string shares the cut string's ID")
	}
	if r.Intern(long[:513]+"different tail") != id {
		t.Fatal("strings equal up to the cut got different IDs")
	}
}

func TestInternIsBounded(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		r := New(8)
		r.SetLevel(LevelSummary)
		for i := range maxStrings {
			if r.Intern(strconv.Itoa(i)) == 0 {
				t.Fatalf("string %d refused", i)
			}
		}
		if r.Intern("one more") != 0 {
			t.Fatal("a full table took one more string")
		}
		if r.Intern("7") == 0 {
			t.Fatal("a full table forgot a known string")
		}
		if st := r.Stats(); st.Strings != maxStrings || st.StringsRefused != 1 {
			t.Fatalf("stats %+v", st)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		r := New(8)
		r.SetLevel(LevelSummary)
		pad := strings.Repeat("x", maxStringLen-8)
		n := 0
		for r.Intern(pad+strconv.Itoa(10_000_000+n)) != 0 {
			n++
		}
		if n != maxStringBytes/maxStringLen {
			t.Fatalf("took %d strings of %d bytes, want %d", n, maxStringLen, maxStringBytes/maxStringLen)
		}
	})
}
