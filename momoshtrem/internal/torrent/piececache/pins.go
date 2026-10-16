package piececache

import (
	"encoding/hex"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

// piececache: pins, the streaming reserve, deferred trimming, and the piece
// queries. Everything here is new; nothing upstream calls it.
//
// Pieces are named as anacrolix's resource-pieces storage names them: by
// the v1 piece hash in lower-case hex, as completed/<hex> once the piece
// passed its hash, and incompleted/<hex>/<offset> for each chunk before.

// ReadyFallback is how long DeferTrim holds trims when Ready isn't called.
const ReadyFallback = 2 * time.Minute

// PieceRef names a piece for HavePieces: its hex and its length.
type PieceRef struct {
	Hex    string
	Length int64
}

// PieceRefFor is p as the storage names it. ok is false for a pure v2
// torrent's piece, which has no v1 hash.
func PieceRefFor(p metainfo.Piece) (ref PieceRef, ok bool) {
	h := p.V1Hash()
	if !h.Ok {
		return PieceRef{}, false
	}
	return PieceRef{Hex: hex.EncodeToString(h.Value[:]), Length: p.Length()}, true
}

// pieceHex is the piece an item belongs to: completed/<hex> or
// incompleted/<hex>/<offset>.
func pieceHex(k key) (string, bool) {
	s := string(k)
	if rest, ok := strings.CutPrefix(s, "completed/"); ok {
		return rest, rest != "" && !strings.Contains(rest, "/")
	}
	if rest, ok := strings.CutPrefix(s, "incompleted/"); ok {
		h, off, ok := strings.Cut(rest, "/")
		return h, ok && h != "" && off != "" && !strings.Contains(off, "/")
	}
	return "", false
}

// normalHex is h in lower case, or false when it isn't a hex string.
func normalHex(h string) (string, bool) {
	h = strings.ToLower(h)
	if h == "" {
		return "", false
	}
	for i := 0; i < len(h); i++ {
		if c := h[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return h, true
}

func (me *Cache) isPinned(k key) bool {
	if len(me.pinCount) == 0 {
		return false
	}
	h, ok := pieceHex(k)
	return ok && me.pinCount[h] > 0
}

// pieceItems calls f for each item of the piece h.
func (me *Cache) pieceItems(h string, f func(key, itemState)) {
	k := key("completed/" + h)
	if ii, ok := me.items[k]; ok {
		f(k, ii)
	}
	dir := key("incompleted/" + h)
	for name := range me.dirs.files(dir) {
		k := dir + "/" + key(name)
		if ii, ok := me.items[k]; ok {
			f(k, ii)
		}
	}
}

// Pin replaces the pieces owner pins with hexes. A pinned piece's items,
// complete or not and including ones written later, are never evicted; the
// trim takes other items, and lets the cache overflow its capacity rather
// than evict them. Remove still deletes them, as a failed hash must. Pieces
// are counted per owner: one stays pinned while any owner pins it. Strings
// that aren't hex are ignored. Pin doesn't trim.
func (me *Cache) Pin(owner string, hexes []string) {
	set := make(map[string]struct{}, len(hexes))
	for _, h := range hexes {
		if h, ok := normalHex(h); ok {
			set[h] = struct{}{}
		}
	}
	me.mu.Lock()
	defer me.mu.Unlock()
	old := me.owners[owner]
	for h := range set {
		if _, ok := old[h]; !ok {
			me.pinHex(h)
		}
	}
	for h := range old {
		if _, ok := set[h]; !ok {
			me.unpinHex(h)
		}
	}
	if len(set) == 0 {
		delete(me.owners, owner)
	} else {
		me.owners[owner] = set
	}
}

// Unpin drops every pin owner holds. The pieces no other owner pins go back
// to the LRU with their last access times; the next trim may take them.
func (me *Cache) Unpin(owner string) {
	me.Pin(owner, nil)
}

func (me *Cache) pinHex(h string) {
	me.pinCount[h]++
	if me.pinCount[h] > 1 {
		return
	}
	me.pieceItems(h, func(k key, ii itemState) {
		me.policy.Forget(k)
		me.pinned += ii.Size
	})
}

func (me *Cache) unpinHex(h string) {
	me.pinCount[h]--
	if me.pinCount[h] > 0 {
		return
	}
	delete(me.pinCount, h)
	me.pieceItems(h, func(k key, ii itemState) {
		me.pinned -= ii.Size
		me.policy.Used(k, ii.Accessed)
	})
}

// SetReserve sets the unpinned bytes the trim never goes below, so that
// pins can't squeeze out the streaming cache: while pins hold the cache
// over its capacity, the newest unpinned bytes stay. The default is 0.
// Like SetCapacity, it doesn't trim.
func (me *Cache) SetReserve(bytes int64) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.reserve = max(bytes, 0)
}

// Demote makes the pieces' items the least recently used, keeping their
// order among themselves, so the trim takes them before anything else.
// A pinned piece keeps the demoted times for when it's unpinned. A later
// read or write of an item makes it recent again.
//
// Demotion is in memory only: after a restart the rescan stamps items with
// their files' times again, so callers demote again after Ready.
func (me *Cache) Demote(hexes []string) {
	me.mu.Lock()
	defer me.mu.Unlock()
	type item struct {
		k  key
		ii itemState
	}
	var items []item
	seen := make(map[string]struct{}, len(hexes))
	for _, h := range hexes {
		h, ok := normalHex(h)
		if !ok {
			continue
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		me.pieceItems(h, func(k key, ii itemState) {
			items = append(items, item{k, ii})
		})
	}
	if len(items) == 0 {
		return
	}
	// The oldest unpinned item's time, or now when there's none.
	base := time.Now()
	if me.policy.NumItems() > 0 {
		base = me.items[me.policy.Choose().(key)].Accessed
	}
	slices.SortFunc(items, func(a, b item) int {
		if c := a.ii.Accessed.Compare(b.ii.Accessed); c != 0 {
			return c
		}
		return strings.Compare(string(a.k), string(b.k))
	})
	for i, it := range items {
		it.ii.Accessed = base.Add(-time.Duration(len(items) - i))
		me.items[it.k] = it.ii
		if !me.isPinned(it.k) {
			me.policy.Used(it.k, it.ii.Accessed)
		}
	}
}

// HavePieces says, for each piece, whether the cache holds it complete:
// completed/<hex> with the piece's length, as anacrolix's completion check
// reads it. It reads the cache's own record of its files (the rescan's,
// then every write and removal), not the disk.
func (me *Cache) HavePieces(pieces []PieceRef) []bool {
	have := make([]bool, len(pieces))
	me.mu.Lock()
	defer me.mu.Unlock()
	for i, p := range pieces {
		h, ok := normalHex(p.Hex)
		if !ok {
			continue
		}
		ii, ok := me.items[key("completed/"+h)]
		have[i] = ok && ii.Size == p.Length
	}
	return have
}

// DeferTrim holds every trim, TrimToCapacity's too, until Ready or, at the
// latest, ReadyFallback from now. Call it before the first write, so that
// after a restart nothing is evicted before the pins are restored. Without
// it, the cache trims from the first update, as upstream's does.
func (me *Cache) DeferTrim() {
	me.deferTrimFor(ReadyFallback)
}

func (me *Cache) deferTrimFor(fallback time.Duration) {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.deferring {
		return
	}
	me.deferring = true
	// The generation keeps a stale timer, one that fired as Ready stopped
	// it, from ending a later DeferTrim.
	me.deferGen++
	gen := me.deferGen
	me.readyTimer = time.AfterFunc(fallback, func() {
		if me.fallback(gen) {
			slog.Warn("piece cache: pins weren't restored in time; trimming resumes",
				"component", "piececache", "after", fallback)
		}
	})
}

// Ready ends DeferTrim. It doesn't trim: the next update or TrimToCapacity
// does. Without DeferTrim it does nothing.
func (me *Cache) Ready() {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.endDeferring()
}

// fallback is the timer of DeferTrim's generation gen: it ends deferring if
// that DeferTrim still holds, and says whether it did.
func (me *Cache) fallback(gen uint64) bool {
	me.mu.Lock()
	defer me.mu.Unlock()
	if gen != me.deferGen {
		return false
	}
	return me.endDeferring()
}

// endDeferring ends deferring, and says whether it was on.
func (me *Cache) endDeferring() bool {
	if !me.deferring {
		return false
	}
	me.deferring = false
	if me.readyTimer != nil {
		me.readyTimer.Stop()
		me.readyTimer = nil
	}
	return true
}
