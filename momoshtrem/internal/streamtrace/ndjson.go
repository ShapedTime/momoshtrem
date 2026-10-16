package streamtrace

import (
	"io"
	"strconv"
	"unicode/utf8"
)

// WriteNDJSON writes events as newline-delimited JSON, one object per line,
// with interned IDs resolved back to their strings:
//
//	{"seq":42,"t_ns":1250000000,"kind":"tf.read","level":"summary","file":7,"off":1048576,"n":32768,"dur_ns":612000000,"drain_ns":0,"slow":true}
//
// Every line has seq, t_ns (since Stats.Start), kind and level; the rest
// depends on the kind (see each constructor). Durations are integer
// nanoseconds named *_ns. The byte ranges (now, ra) are [start, end)
// pairs. http.start's range is the Range header as ParseRange gives it:
// [first, last] inclusive, with -1 for a missing end ([-1, n] is the last
// n bytes). Flags appear only when set, and a string whose ID is 0 is
// left out.
func (r *Recorder) WriteNDJSON(w io.Writer, events []Event) error {
	enc := encoder{strs: r.strs.snapshot(), b: make([]byte, 0, 32<<10)}
	for i := range events {
		enc.event(&events[i])
		if len(enc.b) >= 28<<10 {
			if _, err := w.Write(enc.b); err != nil {
				return err
			}
			enc.b = enc.b[:0]
		}
	}
	if len(enc.b) > 0 {
		if _, err := w.Write(enc.b); err != nil {
			return err
		}
	}
	return nil
}

type encoder struct {
	strs []string
	b    []byte
}

func (e *encoder) event(ev *Event) {
	e.b = append(e.b, `{"seq":`...)
	e.b = strconv.AppendUint(e.b, ev.Seq, 10)
	e.int("t_ns", int64(ev.Time))
	e.name("kind", ev.Kind.String())
	e.name("level", ev.Level.String())

	p := &ev.p
	switch ev.Kind {
	case KindHTTPStart:
		e.uint("req", p[0])
		e.str("method", lo32(p[2]))
		e.str("path", lo32(p[1]))
		e.str("ua", hi32(p[1]))
		e.str("client", hi32(p[2]))
		if start, end := int64(p[3]), int64(p[4]); start != -1 || end != -1 {
			e.pair("range", start, end)
		}
		e.flag("if_range", p[5]&1 != 0)
	case KindHTTPEnd:
		e.uint("req", p[0])
		e.int("status", int64(lo32(p[1])))
		e.int("bytes", int64(p[2]))
		e.int("ttfb_ns", int64(p[3]))
		e.int("dur_ns", int64(p[4]))
		e.flag("aborted", hi32(p[1])&1 != 0)
	case KindHTTPSeek:
		e.uint("req", p[0])
		e.int("offset", int64(p[1]))
		e.int("whence", int64(p[2]))
		e.int("pos", int64(p[3]))
	case KindVFSOpen:
		e.uint("req", p[1])
		e.uint("file", p[0])
		e.str("path", lo32(p[2]))
		e.str("hash", hi32(p[2]))
		e.enum("type", fileTypeNames[:], uint64(uint8(p[3])))
		e.int("size", int64(p[4]))
		e.int("dur_ns", int64(p[5]))
		e.flag("failed", p[3]&(1<<8) != 0)
	case KindTFRead:
		e.uint("file", p[0])
		e.int("off", int64(p[1]))
		e.int("n", int64(lo32(p[2])))
		e.int("dur_ns", int64(p[3]))
		e.int("drain_ns", int64(p[4]))
		e.flags(readFlagNames[:], uint64(hi32(p[2])))
	case KindTFActivation:
		e.uint("file", p[0])
		e.str("hash", lo32(p[1]))
		e.int("wait_ns", int64(p[2]))
		e.flag("timed_out", hi32(p[1])&1 != 0)
	case KindTFClose:
		e.uint("file", p[0])
		e.int("open_ns", int64(p[1]))
		e.int("pos", int64(p[2]))
	case KindReaderCreate:
		e.uint("reader", p[0])
		e.uint("file", p[1])
		e.str("hash", lo32(p[2]))
		e.str("path", hi32(p[2]))
		e.int("size", int64(p[3]))
		e.int("offset", int64(p[4]))
		e.int("piece_len", int64(p[5]))
	case KindReaderClose:
		e.uint("reader", p[0])
		e.int("open_ns", int64(p[1]))
		e.int("pos", int64(p[2]))
		e.flag("reset", p[3]&1 != 0)
	case KindFormatStart:
		e.uint("reader", p[0])
	case KindFormatEnd:
		e.uint("reader", p[0])
		e.str("format", uint32(p[1]))
		e.int("moov_off", int64(p[2]))
		e.int("moov_size", int64(p[3]))
		e.int("dur_ns", int64(p[4]))
	case KindPrioUpdate:
		meta := p[1]
		e.uint("reader", p[0])
		e.enum("type", prioTypeNames[:], meta&0xf)
		e.int("off", int64(p[2]))
		if PrioType(meta&0xf) == PrioDebounced {
			break // it set nothing
		}
		e.pair("now", int64(p[2]), int64(p[3]))
		e.pair("ra", int64(p[4]), int64(p[5]))
		e.int("downgraded", int64(meta>>8&(1<<16-1)))
		e.int("loop_ns", int64(meta>>24))
		e.flag("forward", meta&(1<<4) != 0)
	case KindTorrentAdd:
		e.str("hash", lo32(p[0]))
		e.int("dur_ns", int64(p[1]))
		e.int("pieces", int64(lo32(p[2])))
		e.int("files", int64(hi32(p[2])))
		e.flags(addFlagNames[:], uint64(hi32(p[0])))
	case KindTorrentRemove:
		e.str("hash", lo32(p[0]))
		e.flag("loaded", p[0]&(1<<32) != 0)
		e.flag("delete_data", p[0]&(1<<33) != 0)
	case KindTorrentState:
		e.str("hash", lo32(p[0]))
		e.enum("from", stateNames[:], p[0]>>32&0xff)
		e.enum("to", stateNames[:], p[0]>>40&0xff)
	case KindSessionBegin:
		e.uint("session", p[0])
		e.str("path", lo32(p[1]))
		e.str("ua", hi32(p[1]))
		e.str("client", uint32(p[2]))
	case KindSessionEnd:
		e.uint("session", p[0])
		e.int("bytes", int64(p[1]))
		e.int("dur_ns", int64(p[2]))
	case KindRemoteOpen, KindRemoteReuse, KindRemoteSkip, KindRemoteReopen, KindRemoteTimeout:
		e.uint("file", p[0])
		e.int("off", int64(p[1]))
		e.int("n", int64(p[2]))
		e.int("dur_ns", int64(p[3]))
		e.flag("failed", p[4]&1 != 0)
	case KindCDNOpen, KindCDNRefresh, KindCDNRetry, KindCDNSlot:
		e.str("stream", lo32(p[0]))
		e.int("off", int64(p[1]))
		e.int("status", int64(p[2]))
		e.int("dur_ns", int64(p[3]))
		e.flag("failed", p[0]&(1<<32) != 0)
	}
	e.b = append(e.b, '}', '\n')
}

func (e *encoder) key(k string) {
	e.b = append(e.b, ',', '"')
	e.b = append(e.b, k...)
	e.b = append(e.b, '"', ':')
}

func (e *encoder) int(k string, v int64) {
	e.key(k)
	e.b = strconv.AppendInt(e.b, v, 10)
}

func (e *encoder) uint(k string, v uint64) {
	e.key(k)
	e.b = strconv.AppendUint(e.b, v, 10)
}

func (e *encoder) pair(k string, a, b int64) {
	e.key(k)
	e.b = append(e.b, '[')
	e.b = strconv.AppendInt(e.b, a, 10)
	e.b = append(e.b, ',')
	e.b = strconv.AppendInt(e.b, b, 10)
	e.b = append(e.b, ']')
}

func (e *encoder) flag(k string, set bool) {
	if set {
		e.key(k)
		e.b = append(e.b, "true"...)
	}
}

// flags writes a flag per set bit, bit i named names[i].
func (e *encoder) flags(names []string, bits uint64) {
	for i, n := range names {
		e.flag(n, bits&(1<<i) != 0)
	}
}

// name writes a known identifier: no escaping needed.
func (e *encoder) name(k, v string) {
	e.key(k)
	e.b = append(e.b, '"')
	e.b = append(e.b, v...)
	e.b = append(e.b, '"')
}

// enum writes names[v], or the number when it's out of range.
func (e *encoder) enum(k string, names []string, v uint64) {
	if v < uint64(len(names)) {
		e.name(k, names[v])
		return
	}
	e.uint(k, v)
}

// str writes an interned string, left out when its ID is 0 or unknown.
func (e *encoder) str(k string, id uint32) {
	if id == 0 || int(id) > len(e.strs) {
		return
	}
	e.key(k)
	e.b = appendJSONString(e.b, e.strs[id-1])
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON string, invalid UTF-8 as U+FFFD.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '"', '\\':
				b = append(b, '\\', c)
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, `�`...)
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}
