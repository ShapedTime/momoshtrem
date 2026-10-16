package streamtrace

import (
	"strconv"
	"time"
)

// Kind is what an event records. Its name (String) is the NDJSON "kind".
type Kind uint8

// The kinds, by hook site (PLAN §7 "Hook sites"). Each records at
// LevelSummary unless its constructor says otherwise.
const (
	_ Kind = iota

	KindHTTPStart // http.start: a WebDAV request arrives
	KindHTTPEnd   // http.end: its response is done
	KindHTTPSeek  // http.seek: webdavFile.Seek

	KindVFSOpen // vfs.open: LibraryFS.OpenContext

	KindTFRead       // tf.read: a TorrentFile read
	KindTFActivation // tf.activation: waitForFirstAccess
	KindTFClose      // tf.close: TorrentFile.Close

	KindReaderCreate // reader.create: a PriorityReader is created
	KindReaderClose  // reader.close: and closed
	KindFormatStart  // format.start: detectFormat starts
	KindFormatEnd    // format.end: and ends

	KindPrioUpdate // prio.update: the Prioritizer sets priorities

	KindTorrentAdd    // torrent.add: the torrent service adds a torrent
	KindTorrentRemove // torrent.remove: and removes one
	KindTorrentState  // torrent.state: the ActivityManager changes a torrent's state

	KindSessionBegin // session.begin: a streaming session starts
	KindSessionEnd   // session.end: and ends

	KindRemoteOpen    // remote.open: a RemoteFile opens a body
	KindRemoteReuse   // remote.reuse: reads on from the open body
	KindRemoteSkip    // remote.skip: discards a short gap
	KindRemoteReopen  // remote.reopen: reopens after a failed read
	KindRemoteTimeout // remote.timeout: its idle watchdog fires

	KindCDNOpen    // cdn.open: the HDRezka streamer gets a response
	KindCDNRefresh // cdn.refresh: resolves a refused link again
	KindCDNRetry   // cdn.retry: retries a transient failure
	KindCDNSlot    // cdn.slot: waited for a free body slot

	kindCount
)

// maxKinds is what the 6 header bits hold.
const maxKinds = 1 << 6

var kindNames = [kindCount]string{
	KindHTTPStart:     "http.start",
	KindHTTPEnd:       "http.end",
	KindHTTPSeek:      "http.seek",
	KindVFSOpen:       "vfs.open",
	KindTFRead:        "tf.read",
	KindTFActivation:  "tf.activation",
	KindTFClose:       "tf.close",
	KindReaderCreate:  "reader.create",
	KindReaderClose:   "reader.close",
	KindFormatStart:   "format.start",
	KindFormatEnd:     "format.end",
	KindPrioUpdate:    "prio.update",
	KindTorrentAdd:    "torrent.add",
	KindTorrentRemove: "torrent.remove",
	KindTorrentState:  "torrent.state",
	KindSessionBegin:  "session.begin",
	KindSessionEnd:    "session.end",
	KindRemoteOpen:    "remote.open",
	KindRemoteReuse:   "remote.reuse",
	KindRemoteSkip:    "remote.skip",
	KindRemoteReopen:  "remote.reopen",
	KindRemoteTimeout: "remote.timeout",
	KindCDNOpen:       "cdn.open",
	KindCDNRefresh:    "cdn.refresh",
	KindCDNRetry:      "cdn.retry",
	KindCDNSlot:       "cdn.slot",
}

// kindWords is how many payload words each kind stores (the layouts below).
var kindWords = [maxKinds]uint8{
	KindHTTPStart:     6,
	KindHTTPEnd:       5,
	KindHTTPSeek:      4,
	KindVFSOpen:       6,
	KindTFRead:        5,
	KindTFActivation:  3,
	KindTFClose:       3,
	KindReaderCreate:  6,
	KindReaderClose:   4,
	KindFormatStart:   1,
	KindFormatEnd:     5,
	KindPrioUpdate:    6,
	KindTorrentAdd:    3,
	KindTorrentRemove: 1,
	KindTorrentState:  1,
	KindSessionBegin:  3,
	KindSessionEnd:    3,
	KindRemoteOpen:    5,
	KindRemoteReuse:   5,
	KindRemoteSkip:    5,
	KindRemoteReopen:  5,
	KindRemoteTimeout: 5,
	KindCDNOpen:       4,
	KindCDNRefresh:    4,
	KindCDNRetry:      4,
	KindCDNSlot:       4,
}

func (k Kind) String() string {
	if k < kindCount && kindNames[k] != "" {
		return kindNames[k]
	}
	return "kind(" + strconv.Itoa(int(k)) + ")"
}

// FileType is what a VFS open returned.
type FileType uint8

const (
	FileOther           FileType = iota // not found, or anything else
	FileDir                             // a directory
	FileTorrent                         // a TorrentFile
	FileRemote                          // a RemoteFile (HDRezka)
	FileSubtitle                        // a subtitle on disk
	FileTorrentSubtitle                 // a subtitle inside a torrent
	FilePlaceholder                     // a placeholder with no source
)

var fileTypeNames = [...]string{"other", "dir", "torrent", "remote", "subtitle", "torrent_subtitle", "placeholder"}

// ReadFlags describe a tf.read.
type ReadFlags uint8

const (
	ReadSlow      ReadFlags = 1 << iota // took over 500 ms (StreamingSlowReads)
	ReadTimeout                         // timed out (StreamingReadTimeouts)
	ReadDrain                           // drained a timed-out read first
	ReadFirstByte                       // the file's first byte
	ReadSeek                            // moved the reader (StreamingSeekActual)
	ReadError                           // failed

	// notableReads are kept at LevelSummary; other reads only at LevelDetailed.
	notableReads = ReadSlow | ReadTimeout | ReadDrain | ReadFirstByte | ReadSeek | ReadError
)

var readFlagNames = [...]string{"slow", "timeout", "drain", "first_byte", "seek", "error"}

// PrioType is what a prio.update did. The names are the
// StreamingPriorityUpdates "type" label values, plus reset.
type PrioType uint8

const (
	PrioInitial   PrioType = iota // InitialPrioritize: now = header, ra = footer (High)
	PrioSeek                      // UpdateForSeek: now = [off, urgent end), ra = readahead
	PrioDebounced                 // UpdateForSeek skipped (detailed only): off
	PrioFormat                    // SetFormatInfo: now = the moov atom
	PrioReset                     // ResetPriorities: now = the whole file
)

var prioTypeNames = [...]string{"initial", "seek", "seek_debounced", "format", "reset"}

// AddFlags describe a torrent.add.
type AddFlags uint8

const (
	AddFromStore AddFlags = 1 << iota // the info dict came from the MetainfoStore
	AddTimeout                        // the metadata didn't come in time
	AddFailed                         // the client refused it
)

var addFlagNames = [...]string{"from_store", "timeout", "failed"}

// State is a torrent's state in the ActivityManager.
type State uint8

const (
	StateNone   State = iota // not registered
	StateActive              // network on
	StateIdle                // network off after the idle timeout
	StatePaused              // paused by the user
)

var stateNames = [...]string{"none", "active", "idle", "paused"}

// RemoteOp is what a RemoteFile did: the kinds remote.open … remote.timeout.
type RemoteOp uint8

const (
	RemoteOpen    RemoteOp = iota // opened a body at off: dur to open
	RemoteReuse                   // read n bytes at off from the open body (detailed only)
	RemoteSkip                    // discarded n bytes up to off
	RemoteReopen                  // a read at off failed midway: opening again
	RemoteTimeout                 // nothing for dur (the read timeout) at off
)

// CDNOp is what the HDRezka streamer did: the kinds cdn.open … cdn.slot.
type CDNOp uint8

const (
	CDNOpen    CDNOp = iota // a response: status, dur to headers
	CDNRefresh              // the link was refused (status): resolving it again
	CDNRetry                // a 429, 5xx (status) or transport error (0): retrying
	CDNSlot                 // waited dur for one of the body slots
)

// Payload packing. Each unexported twin below lists its kind's payload
// words ("Words:"), in order, separated by |. "a, b" is one word with a in
// its low 32 bits and b in its high 32; "x + y<<n" is bit fields of one
// word.
// The NDJSON encoder (ndjson.go) reads the same layouts back.

func pack(lo, hi uint32) uint64 { return uint64(lo) | uint64(hi)<<32 }

func lo32(v uint64) uint32 { return uint32(v) }
func hi32(v uint64) uint32 { return uint32(v >> 32) }

func bit(b bool, mask uint64) uint64 {
	if b {
		return mask
	}
	return 0
}

// clamp32 fits a non-negative count into 32 bits.
func clamp32(n int64) uint32 {
	if n < 0 {
		return 0
	}
	return uint32(min(n, 1<<32-1))
}

// clampBits fits a non-negative value into bits bits.
func clampBits(n int64, bits uint) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(min(n, 1<<bits-1))
}

// The constructors. Each exported one is the level check alone, which
// inlines into the hook: off, a hook costs one atomic load. The work is in
// its unexported twin, kept out of line (go:noinline) so that the inliner
// prices the call at its flat call cost instead of the twin's size, which
// would push the check over the inlining budget.

// HTTPStart records a WebDAV request arriving. req is its ID (NewID).
// method, path, ua and client are interned; client is the remote host
// without its port (ports would fill the string table). rangeStart and
// rangeEnd are the Range header as ParseRange encodes it.
func (r *Recorder) HTTPStart(req uint64, method, path, ua, client ID, rangeStart, rangeEnd int64, ifRange bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.httpStart(req, method, path, ua, client, rangeStart, rangeEnd, ifRange)
	}
}

// Words: req | path, ua | method, client | rangeStart | rangeEnd | if-range.
//
//go:noinline
func (r *Recorder) httpStart(req uint64, method, path, ua, client ID, rangeStart, rangeEnd int64, ifRange bool) {
	r.put(KindHTTPStart, LevelSummary, req, pack(uint32(path), uint32(ua)), pack(uint32(method), uint32(client)),
		uint64(rangeStart), uint64(rangeEnd), bit(ifRange, 1))
}

// HTTPEnd records a request's response: its status, the body bytes
// written, the time to its first byte and its whole duration. aborted
// means the client went away first.
func (r *Recorder) HTTPEnd(req uint64, status int, bytes int64, ttfb, dur time.Duration, aborted bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.httpEnd(req, status, bytes, ttfb, dur, aborted)
	}
}

// Words: req | status + aborted<<32 | bytes | ttfb | dur.
//
//go:noinline
func (r *Recorder) httpEnd(req uint64, status int, bytes int64, ttfb, dur time.Duration, aborted bool) {
	r.put(KindHTTPEnd, LevelSummary, req, uint64(clamp32(int64(status)))|bit(aborted, 1<<32),
		uint64(bytes), uint64(ttfb), uint64(dur), 0)
}

// HTTPSeek records webdavFile.Seek: offset and whence asked, pos reached.
func (r *Recorder) HTTPSeek(req uint64, offset int64, whence int, pos int64) {
	if r.level.Load() >= int32(LevelSummary) {
		r.httpSeek(req, offset, whence, pos)
	}
}

// Words: req | offset | whence | pos.
//
//go:noinline
func (r *Recorder) httpSeek(req uint64, offset int64, whence int, pos int64) {
	r.put(KindHTTPSeek, LevelSummary, req, uint64(offset), uint64(whence), uint64(pos), 0, 0)
}

// VFSOpen records LibraryFS.OpenContext: the request opening, the ID the
// opened file's events carry, its path and torrent hash (interned; 0 for
// none), what it opened, its size, how long the open took (it may add the
// torrent) and whether it failed.
func (r *Recorder) VFSOpen(req, file uint64, path, hash ID, typ FileType, size int64, dur time.Duration, failed bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.vfsOpen(req, file, path, hash, typ, size, dur, failed)
	}
}

// Words: file | req | path, hash | type + failed<<8 | size | dur.
//
//go:noinline
func (r *Recorder) vfsOpen(req, file uint64, path, hash ID, typ FileType, size int64, dur time.Duration, failed bool) {
	r.put(KindVFSOpen, LevelSummary, file, req, pack(uint32(path), uint32(hash)), uint64(typ)|bit(failed, 1<<8),
		uint64(size), uint64(dur))
}

// TFRead records a TorrentFile read: n bytes at off in dur, after drain
// waiting for a timed-out read. Notable reads (any flag) record at
// LevelSummary, the others at LevelDetailed only, so at detailed there is
// exactly one tf.read per read.
func (r *Recorder) TFRead(file uint64, off int64, n int, dur, drain time.Duration, flags ReadFlags) {
	if r.level.Load() != int32(LevelOff) {
		r.tfRead(file, off, n, dur, drain, flags)
	}
}

// Words: file | off | n + flags<<32 | dur | drain.
//
//go:noinline
func (r *Recorder) tfRead(file uint64, off int64, n int, dur, drain time.Duration, flags ReadFlags) {
	lvl := LevelDetailed
	if flags&notableReads != 0 {
		lvl = LevelSummary
	}
	if !r.On(lvl) {
		return
	}
	r.put(KindTFRead, lvl, file, uint64(off), uint64(clamp32(int64(n)))|uint64(flags)<<32, uint64(dur), uint64(drain), 0)
}

// TFActivation records a TorrentFile's first-access wait for its torrent
// to activate: how long it waited, and whether it gave up.
func (r *Recorder) TFActivation(file uint64, hash ID, wait time.Duration, timedOut bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.tfActivation(file, hash, wait, timedOut)
	}
}

// Words: file | hash + timedOut<<32 | wait.
//
//go:noinline
func (r *Recorder) tfActivation(file uint64, hash ID, wait time.Duration, timedOut bool) {
	r.put(KindTFActivation, LevelSummary, file, uint64(hash)|bit(timedOut, 1<<32), uint64(wait), 0, 0, 0)
}

// TFClose records a TorrentFile closing after open, at reader position pos.
func (r *Recorder) TFClose(file uint64, open time.Duration, pos int64) {
	if r.level.Load() >= int32(LevelSummary) {
		r.tfClose(file, open, pos)
	}
}

// Words: file | open | pos.
//
//go:noinline
func (r *Recorder) tfClose(file uint64, open time.Duration, pos int64) {
	r.put(KindTFClose, LevelSummary, file, uint64(open), uint64(pos), 0, 0, 0)
}

// ReaderCreate records a PriorityReader created for a file: its ID, the
// file's ID, torrent hash and path in the torrent (interned), size, offset
// in the torrent and piece length, which map prio.update's byte ranges to
// pieces.
func (r *Recorder) ReaderCreate(reader, file uint64, hash, path ID, size, offset, pieceLen int64) {
	if r.level.Load() >= int32(LevelSummary) {
		r.readerCreate(reader, file, hash, path, size, offset, pieceLen)
	}
}

// Words: reader | file | hash, path | size | offset | pieceLen.
//
//go:noinline
func (r *Recorder) readerCreate(reader, file uint64, hash, path ID, size, offset, pieceLen int64) {
	r.put(KindReaderCreate, LevelSummary, reader, file, pack(uint32(hash), uint32(path)), uint64(size), uint64(offset), uint64(pieceLen))
}

// ReaderClose records a PriorityReader closing after open, at pos; reset
// means it reset the file's priorities (open ≥ 30 s).
func (r *Recorder) ReaderClose(reader uint64, open time.Duration, pos int64, reset bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.readerClose(reader, open, pos, reset)
	}
}

// Words: reader | open | pos | reset.
//
//go:noinline
func (r *Recorder) readerClose(reader uint64, open time.Duration, pos int64, reset bool) {
	r.put(KindReaderClose, LevelSummary, reader, uint64(open), uint64(pos), bit(reset, 1), 0, 0)
}

// FormatStart records a reader's format detection starting.
func (r *Recorder) FormatStart(reader uint64) {
	if r.level.Load() >= int32(LevelSummary) {
		r.formatStart(reader)
	}
}

// Words: reader.
//
//go:noinline
func (r *Recorder) formatStart(reader uint64) {
	r.put(KindFormatStart, LevelSummary, reader, 0, 0, 0, 0, 0)
}

// FormatEnd records a reader's format detection ending after dur: the
// format's name (interned) and the moov atom (0, 0 when none).
func (r *Recorder) FormatEnd(reader uint64, format ID, moovOff, moovSize int64, dur time.Duration) {
	if r.level.Load() >= int32(LevelSummary) {
		r.formatEnd(reader, format, moovOff, moovSize, dur)
	}
}

// Words: reader | format | moovOff | moovSize | dur.
//
//go:noinline
func (r *Recorder) formatEnd(reader uint64, format ID, moovOff, moovSize int64, dur time.Duration) {
	r.put(KindFormatEnd, LevelSummary, reader, uint64(format), uint64(moovOff), uint64(moovSize), uint64(dur), 0)
}

// PrioUpdate records a Prioritizer update of type typ (PrioType says what
// the ranges hold): the offset, the now range [off, nowEnd), the readahead
// range [raStart, raEnd), all file-relative bytes, forward for a seek's
// direction (StreamingSeeks), how many pieces it downgraded, and how long
// its SetPriority loops took. Debounced updates record at LevelDetailed
// only.
func (r *Recorder) PrioUpdate(reader uint64, typ PrioType, forward bool, off, nowEnd, raStart, raEnd int64, downgraded int, loop time.Duration) {
	if r.level.Load() != int32(LevelOff) {
		r.prioUpdate(reader, typ, forward, off, nowEnd, raStart, raEnd, downgraded, loop)
	}
}

// Words: reader | type + forward<<4 + downgraded<<8 (16 bits) + loop<<24
// (40 bits of ns) | off | nowEnd | raStart | raEnd.
//
//go:noinline
func (r *Recorder) prioUpdate(reader uint64, typ PrioType, forward bool, off, nowEnd, raStart, raEnd int64, downgraded int, loop time.Duration) {
	lvl := LevelSummary
	if typ == PrioDebounced {
		lvl = LevelDetailed
	}
	if !r.On(lvl) {
		return
	}
	meta := uint64(typ&0xf) | bit(forward, 1<<4) | clampBits(int64(downgraded), 16)<<8 | clampBits(int64(loop), 40)<<24
	r.put(KindPrioUpdate, lvl, reader, meta, uint64(off), uint64(nowEnd), uint64(raStart), uint64(raEnd))
}

// TorrentAdd records the torrent service adding a torrent: how long it
// took, how it went, and its piece and file counts.
func (r *Recorder) TorrentAdd(hash ID, dur time.Duration, pieces, files int, flags AddFlags) {
	if r.level.Load() >= int32(LevelSummary) {
		r.torrentAdd(hash, dur, pieces, files, flags)
	}
}

// Words: hash + flags<<32 | dur | pieces, files.
//
//go:noinline
func (r *Recorder) torrentAdd(hash ID, dur time.Duration, pieces, files int, flags AddFlags) {
	r.put(KindTorrentAdd, LevelSummary, pack(uint32(hash), uint32(flags)), uint64(dur),
		pack(clamp32(int64(pieces)), clamp32(int64(files))), 0, 0, 0)
}

// TorrentRemove records the torrent service removing a torrent: whether
// it was loaded, and whether its data goes too.
func (r *Recorder) TorrentRemove(hash ID, loaded, deleteData bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.torrentRemove(hash, loaded, deleteData)
	}
}

// Words: hash + loaded<<32 + deleteData<<33.
//
//go:noinline
func (r *Recorder) torrentRemove(hash ID, loaded, deleteData bool) {
	r.put(KindTorrentRemove, LevelSummary, uint64(hash)|bit(loaded, 1<<32)|bit(deleteData, 1<<33), 0, 0, 0, 0, 0)
}

// TorrentState records the ActivityManager moving a torrent from one state
// to another (activation is a move to StateActive).
func (r *Recorder) TorrentState(hash ID, from, to State) {
	if r.level.Load() >= int32(LevelSummary) {
		r.torrentState(hash, from, to)
	}
}

// Words: hash + from<<32 + to<<40.
//
//go:noinline
func (r *Recorder) torrentState(hash ID, from, to State) {
	r.put(KindTorrentState, LevelSummary, uint64(hash)|uint64(from)<<32|uint64(to)<<40, 0, 0, 0, 0, 0)
}

// SessionBegin records a streaming session starting: its ID, and the path,
// User-Agent and client host it's keyed by (interned).
func (r *Recorder) SessionBegin(session uint64, path, ua, client ID) {
	if r.level.Load() >= int32(LevelSummary) {
		r.sessionBegin(session, path, ua, client)
	}
}

// Words: session | path, ua | client.
//
//go:noinline
func (r *Recorder) sessionBegin(session uint64, path, ua, client ID) {
	r.put(KindSessionBegin, LevelSummary, session, pack(uint32(path), uint32(ua)), uint64(client), 0, 0, 0)
}

// SessionEnd records a streaming session ending after dur, having read
// bytes.
func (r *Recorder) SessionEnd(session uint64, bytes int64, dur time.Duration) {
	if r.level.Load() >= int32(LevelSummary) {
		r.sessionEnd(session, bytes, dur)
	}
}

// Words: session | bytes | dur.
//
//go:noinline
func (r *Recorder) sessionEnd(session uint64, bytes int64, dur time.Duration) {
	r.put(KindSessionEnd, LevelSummary, session, uint64(bytes), uint64(dur), 0, 0, 0)
}

// Remote records what a RemoteFile (an HDRezka stream) did: op says what
// off, n and dur mean, and failed that it didn't work. RemoteReuse records
// at LevelDetailed only.
func (r *Recorder) Remote(op RemoteOp, file uint64, off, n int64, dur time.Duration, failed bool) {
	if r.level.Load() != int32(LevelOff) {
		r.remote(op, file, off, n, dur, failed)
	}
}

// Words: file | off | n | dur | failed.
//
//go:noinline
func (r *Recorder) remote(op RemoteOp, file uint64, off, n int64, dur time.Duration, failed bool) {
	if op > RemoteTimeout {
		return
	}
	lvl := LevelSummary
	if op == RemoteReuse {
		lvl = LevelDetailed
	}
	if !r.On(lvl) {
		return
	}
	r.put(KindRemoteOpen+Kind(op), lvl, file, uint64(off), uint64(n), uint64(dur), bit(failed, 1), 0)
}

// CDN records what the HDRezka streamer did for a stream (interned): op
// says what status and dur mean; off is the body's start.
func (r *Recorder) CDN(op CDNOp, stream ID, off int64, status int, dur time.Duration, failed bool) {
	if r.level.Load() >= int32(LevelSummary) {
		r.cdn(op, stream, off, status, dur, failed)
	}
}

// Words: stream + failed<<32 | off | status | dur.
//
//go:noinline
func (r *Recorder) cdn(op CDNOp, stream ID, off int64, status int, dur time.Duration, failed bool) {
	if op > CDNSlot {
		return
	}
	r.put(KindCDNOpen+Kind(op), LevelSummary, uint64(stream)|bit(failed, 1<<32), uint64(off), uint64(status), uint64(dur), 0, 0)
}
