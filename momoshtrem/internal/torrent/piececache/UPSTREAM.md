# piececache: upstream and changes

`piececache` is a copy of `github.com/anacrolix/missinggo/v2@v2.10.0/filecache` (MIT, Matt Joiner; see `LICENSE`). It is the store behind anacrolix's resource-pieces storage (`storage.NewResourcePieces`), which momoshtrem's `torrent.InitStorage` builds. The copy adds pins, a streaming reserve, deferred trimming, a directory index and two piece queries, for kept and smart downloads (#77, #201).

Each change is marked `piececache:` in the code. To see them all, diff this folder against upstream (only the package clause differs in the unchanged files):

```sh
diff -r "$(go env GOMODCACHE)/github.com/anacrolix/missinggo/v2@v2.10.0/filecache" internal/torrent/piececache
```

## Files

| File | State |
|---|---|
| `file.go`, `itemstate.go`, `key.go`, `lru.go`, `policy.go` | Upstream's, package renamed. |
| `cache.go` | Changed: see below. |
| `uniform.go` | Changed: `Readdirnames` reads the directory index. |
| `dirindex.go`, `pins.go` | New. |
| `cache_test.go`, `lru_test.go` | Upstream's tests, ported from testify to the standard library. `lru_test.go` also holds `policy_test.go` and `lruitems_test.go`. |
| `differential_test.go`, `pins_test.go`, `dirindex_test.go`, `restart_test.go`, `bench_test.go` | New tests. |

`lru.go` still imports missinggo v1's `orderedmap`, as upstream does. `go.mod` already lists that module, as indirect.

## Changes

### Pins (`pins.go`, `cache.go`)
- `Pin(owner, hexes)` replaces the piece hexes `owner` pins, and `Unpin(owner)` drops them. Hexes are refcounted across owners. Strings that aren't hex are ignored, and upper case is folded.
- A pin covers a piece's items in anacrolix's layout: `completed/<hex>`, and `incompleted/<hex>/<offset>` for each chunk. It also covers items created after the pin.
- A pinned item stays in `items` and in `filled`, but leaves the LRU policy, so the trim can't pick it. Its accesses still update its access time. On unpin it goes back to the policy with that time.
- `CacheInfo` gains `Pinned`, the pinned bytes (part of `Filled`).
- `Remove` is unchanged and unconditional: a piece that fails its hash goes through `MarkNotComplete`, which must delete it. The pin itself stays, so a new download of that piece is pinned as it's written.
- Pin and Unpin don't trim, as `SetCapacity` doesn't.

### The trim (`cache.go`)
`updateItem` is split into `applyUpdate` (upstream's update, plus the pinned bytes and the directory index) and `trimToCapacity(protect)`:
- Upstream's trim calls `remove`, whose own `updateItem` trims again, recursively. Here the trim is a loop over `removeFile` + `applyUpdate`. It makes the same evictions in the same order.
- It evicts the least recently used unpinned item while `filled > capacity && filled - pinned > reserve`. `SetReserve(bytes)` sets the reserve (default 0; negative is 0) and, like `SetCapacity`, doesn't trim.
- When nothing unpinned is left, it stops. Upstream's `Choose` panics with "cache empty" there. Without pins, upstream can't get there, since a fill over capacity means an item is in the policy.
- **The key being updated.** When the victim is the key whose update runs the trim, and it is the last unpinned item, it is kept if it fits the capacity on its own. Then only pinned bytes hold the cache over its capacity, and evicting the chunk being written would only make anacrolix fetch it again. Otherwise it goes, as upstream's does.
  - Without pins this never applies. The last item is then all of `filled`, which is over the capacity, so it goes exactly as upstream evicts it.
  - It protects only that key. With reserve 0 and pins at or over the capacity, the next write still evicts the previous chunk. **Set a reserve whenever pins can fill the capacity.**
- `TrimToCapacity` trims with no protected key.

### Deferred trimming (`pins.go`)
- `DeferTrim()` skips every trim, `TrimToCapacity`'s included, until `Ready()`. A fallback timer ends the deferral after `ReadyFallback` (2 minutes) and logs a warning. Each `DeferTrim` has its own generation, so a timer that fired as `Ready` stopped it can't end a later `DeferTrim`.
- `Ready()` doesn't trim; the next update or `TrimToCapacity` does.
- `InitStorage` doesn't call `DeferTrim`, so production trims from the first write as it did with `filecache`.

### Directory index (`dirindex.go`, `uniform.go`, `cache.go`)
- Upstream's `Readdirnames` walks every item under the cache's lock, on every 32 KiB read of a piece being hashed. That's ~38k items at a 150 GiB cap. The index maps each directory to its entries, and `Readdirnames` walks only its subtree.
- It returns the same names as upstream: every item under `Location + "/"`, at any depth, relative to `Location`, and nothing for an empty or unclean `Location`. Order is unspecified in both.
- `applyUpdate` maintains it whenever an item is added or forgotten, which covers create, remove, rename, eviction and the rescan. The rescan rebuilds it from scratch.

### Queries (`pins.go`)
- `HavePieces([]PieceRef) []bool`: whether `completed/<hex>` exists with the piece's length, as anacrolix's completion check stats it. It reads the cache's own record of its files, not the disk.
- `PieceRefFor(metainfo.Piece)` names a piece as the storage does (v1 hash in hex, and its length). It returns false for a pure v2 torrent's piece.
- `Demote(hexes)` makes those pieces' items the least recently used, so the trim takes them first. Their order among themselves is kept. A pinned item keeps the demoted time for when it's unpinned, and a later access makes an item recent again. Demotion is in memory only: after a restart the rescan stamps items with their files' times again, so callers demote again after `Ready`.

## Behaviour differences from upstream

**With no pins and no reserve: none.** The differential test (`differential_test.go`) runs random sequences of creates, writes, truncating Puts, reads, files held open across steps, removes, renames, `SetCapacity`, `TrimToCapacity`, `Readdirnames`, `Get` and `Delete` on both caches. After every step it checks:
- the same files and folders with the same contents;
- the same items and sizes, and the same LRU order;
- the same `Info`, with `Pinned` 0;
- the same answers.

Renames set the source's file times first, the same in both folders, and half of them follow a capacity change. So they often run over capacity with the renamed item the least recently used. That is the one case without pins where the trim's victim is the key being updated while other items remain. `TestRenameOverCapacityMatchesUpstream` pins that case down deterministically.

Four mutations of the trim are each caught:
- protecting the updated key always;
- dropping the "last unpinned item" test;
- dropping the "fits on its own" test;
- trimming at `filled >= capacity`.

Only differences by design remain:
- a reserve above 0 stops the trim at `max(capacity, reserve)` unpinned bytes;
- `DeferTrim` holds trims;
- `Readdirnames` costs the directory's entries, not the whole cache.

## Kept as upstream (worth knowing)
- `SetCapacity` doesn't trim; the next update does, all at once.
- `NewCache` rescans in a goroutine that holds the lock, so the first call (for example `SetCapacity`, `DeferTrim` or `Pin`) waits for the rescan.
- Opening an existing item with `O_TRUNC` keeps its recorded size until writes pass it, so `Filled` overcounts meanwhile. `HavePieces` inherits that for a piece whose `completed/<hex>` is being rewritten.
- `Rename` stamps the new item with its file's access and modification times.
- `MarkComplete` briefly holds a piece twice, its chunks and its completed copy, so a cache with `n` pieces of room keeps `n - 1` pieces complete.
