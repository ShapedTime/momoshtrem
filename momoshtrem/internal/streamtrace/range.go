package streamtrace

import "strings"

// ParseRange encodes an HTTP Range header as HTTPStart's numbers:
// "bytes=100-199" is (100, 199), the open-ended "bytes=100-" is (100, -1),
// a suffix "bytes=-500" is (-1, 500), and no header, or one it can't read,
// is (-1, -1). Of several ranges, only the first counts. It doesn't
// allocate.
func ParseRange(h string) (start, end int64) {
	const unit = "bytes="
	if !strings.HasPrefix(h, unit) {
		return -1, -1
	}
	spec := h[len(unit):]
	if i := strings.IndexByte(spec, ','); i >= 0 {
		spec = spec[:i]
	}
	spec = strings.TrimSpace(spec)
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return -1, -1
	}
	first, last := strings.TrimSpace(spec[:dash]), strings.TrimSpace(spec[dash+1:])
	start, end = -1, -1
	ok := true
	if first != "" {
		start, ok = parseUint(first)
	}
	if ok && last != "" {
		end, ok = parseUint(last)
	}
	if !ok || (first == "" && last == "") {
		return -1, -1
	}
	return start, end
}

// parseUint reads a non-negative decimal int64, without strconv's error
// allocation.
func parseUint(s string) (int64, bool) {
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		d := int64(c - '0')
		if n > (1<<63-1-d)/10 {
			return 0, false
		}
		n = n*10 + d
	}
	return n, len(s) > 0
}
