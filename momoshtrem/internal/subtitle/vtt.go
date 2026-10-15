package subtitle

import (
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrNotVTT: the file doesn't start with a WEBVTT header, so it isn't
	// WebVTT (an error page, say).
	ErrNotVTT = errors.New("not a WebVTT file")
	// ErrNoCues: the WebVTT file has no cue with any text.
	ErrNoCues = errors.New("the WebVTT file has no cues")
)

// VTTToSRT converts a WebVTT file to SRT, the format every stored subtitle
// has, so players needn't support WebVTT. Cues are renumbered from 1 as
//
//	N
//	HH:MM:SS,mmm --> HH:MM:SS,mmm
//	text
//
// with a blank line after each. It drops what SRT can't say: the header
// and its metadata, NOTE, STYLE and REGION blocks, cue identifiers and
// settings ("line:0 align:start"), and every tag but <i>, <b> and <u>
// (classes, voices and karaoke timestamps go; a voice's name with them).
// Entities are decoded, a no-break space becoming a space. Cues left with
// no text are dropped. It reads a UTF-8 BOM, CRLF or CR line ends, and
// timestamps without hours ("01:02.500").
func VTTToSRT(vtt []byte) ([]byte, error) {
	text := strings.TrimPrefix(string(vtt), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if !isBlockStart(lines[0], "WEBVTT") {
		return nil, ErrNotVTT
	}

	var out strings.Builder
	n := 0
	// The header's metadata ("Kind: captions") runs to the first blank
	// line; a cue right after it is read anyway.
	i := 1
	for i < len(lines) && !isBlank(lines[i]) && !strings.Contains(lines[i], "-->") {
		i++
	}
	for i < len(lines) {
		line := lines[i]
		switch {
		case isBlank(line):
			i++
			continue
		case isBlockStart(line, "NOTE"), isBlockStart(line, "STYLE"), isBlockStart(line, "REGION"):
			i = skipBlock(lines, i)
			continue
		}
		timing := line
		if !strings.Contains(timing, "-->") {
			// A cue identifier; the timing is on the next line.
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "-->") {
				i = skipBlock(lines, i)
				continue
			}
			i++
			timing = lines[i]
		}
		i++
		start := i
		for i < len(lines) && !isBlank(lines[i]) {
			i++
		}
		from, to, ok := parseTiming(timing)
		if !ok {
			continue
		}
		cue := cueText(lines[start:i])
		if cue == "" {
			continue
		}
		n++
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n", n, srtTimestamp(from), srtTimestamp(to), cue)
	}
	if n == 0 {
		return nil, ErrNoCues
	}
	return []byte(out.String()), nil
}

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

// isBlockStart reports whether line starts a block with the keyword:
// "NOTE", "NOTE a comment", but not "NOTES".
func isBlockStart(line, keyword string) bool {
	rest, ok := strings.CutPrefix(line, keyword)
	return ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t')
}

// skipBlock returns the index of the blank line ending the block at i.
func skipBlock(lines []string, i int) int {
	for i < len(lines) && !isBlank(lines[i]) {
		i++
	}
	return i
}

// parseTiming reads "00:01:02.500 --> 00:01:04.000 line:0 align:start".
func parseTiming(line string) (from, to time.Duration, ok bool) {
	left, right, found := strings.Cut(line, "-->")
	if !found {
		return 0, 0, false
	}
	end := strings.Fields(right)
	if len(end) == 0 {
		return 0, 0, false
	}
	from, okFrom := parseTimestamp(strings.TrimSpace(left))
	to, okTo := parseTimestamp(end[0])
	return from, to, okFrom && okTo
}

// parseTimestamp reads "HH:MM:SS.mmm" or "MM:SS.mmm". Hours may have more
// digits; a comma for the dot and fewer millisecond digits are accepted.
func parseTimestamp(s string) (time.Duration, bool) {
	clock, frac, found := strings.Cut(strings.Replace(s, ",", ".", 1), ".")
	if !found || frac == "" || len(frac) > 3 {
		return 0, false
	}
	parts := strings.Split(clock, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var total time.Duration
	for _, p := range parts {
		v, ok := digits(p)
		if !ok {
			return 0, false
		}
		total = total*60 + time.Duration(v)*time.Second
	}
	ms, ok := digits(frac + strings.Repeat("0", 3-len(frac)))
	if !ok {
		return 0, false
	}
	return total + time.Duration(ms)*time.Millisecond, true
}

// digits parses a non-empty run of ASCII digits.
func digits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v, err := strconv.Atoi(s)
	return v, err == nil
}

// srtTimestamp writes "HH:MM:SS,mmm".
func srtTimestamp(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

// cueText cleans a cue's lines for SRT and joins them; "" when no line has
// visible text.
func cueText(lines []string) string {
	var kept []string
	for _, line := range lines {
		formatted, visible := cleanCueLine(line)
		if strings.TrimSpace(visible) != "" {
			kept = append(kept, strings.TrimSpace(formatted))
		}
	}
	return strings.Join(kept, "\n")
}

// srtTags are the tags SRT shares with WebVTT.
var srtTags = map[string]bool{"i": true, "b": true, "u": true}

// cleanCueLine strips the tags SRT doesn't have (keeping <i>, <b> and <u>,
// without their classes) and decodes entities. visible is the line without
// any tag, to tell an empty line.
func cleanCueLine(line string) (formatted, visible string) {
	var f, v strings.Builder
	for {
		open := strings.IndexByte(line, '<')
		if open < 0 {
			break
		}
		closing := strings.IndexByte(line[open:], '>')
		if closing < 0 {
			break // A stray "<": text
		}
		f.WriteString(line[:open])
		v.WriteString(line[:open])
		tag := line[open+1 : open+closing]
		line = line[open+closing+1:]
		end := strings.HasPrefix(tag, "/")
		name := strings.TrimPrefix(tag, "/")
		if i := strings.IndexAny(name, ". \t"); i >= 0 {
			name = name[:i]
		}
		if srtTags[name] {
			if end {
				f.WriteString("</" + name + ">")
			} else {
				f.WriteString("<" + name + ">")
			}
		}
	}
	f.WriteString(line)
	v.WriteString(line)
	return decodeEntities(f.String()), decodeEntities(v.String())
}

// decodeEntities decodes &amp;, &lt;, &gt;, &nbsp; and the rest; a no-break
// space becomes a space.
func decodeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return strings.ReplaceAll(html.UnescapeString(s), "\u00a0", " ")
}
