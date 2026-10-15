package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/hdrezka"
)

// probeFlags are the parsed `hdrezka-probe` flags.
type probeFlags struct {
	query      string
	translator int    // 0: the page default
	quality    string // "": the best that isn't premium
	season     int    // 0: season 1 (series only)
	episode    int    // 0: episode 1 (series only)
	showURLs   bool
}

// parseProbeFlags parses the subcommand's flags, which may come before or
// after the query. ok is false when the process should exit with code (0
// after -h, 2 for bad flags).
func parseProbeFlags(args []string) (f probeFlags, code int, ok bool) {
	fs := flag.NewFlagSet(probeCommand, flag.ContinueOnError)
	translator := fs.Int("translator", 0, "translator `id` (default: the page's default, or the first that isn't premium)")
	quality := fs.String("quality", "", "quality `label` to check, e.g. 1080p (default: the best that isn't premium)")
	season := fs.Int("season", 0, "season `N` of a series (default 1)")
	episode := fs.Int("episode", 0, "episode `N` of a series (default 1)")
	showURLs := fs.Bool("show-urls", false, "print the CDN links in full (they're signed for this IP)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: momoshtrem %s [flags] <query>\n\n", probeCommand)
		fmt.Fprintln(fs.Output(), "Searches HDRezka and checks that the best match resolves to a seekable MP4.")
		fmt.Fprint(fs.Output(), "It needs no database and ignores HDREZKA_ENABLED.\n\nFlags:\n")
		fs.PrintDefaults()
	}

	var words []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return f, 0, false
			}
			return f, 2, false
		}
		if fs.NArg() == 0 {
			break
		}
		words = append(words, fs.Arg(0))
		args = fs.Args()[1:]
	}

	f.query = strings.TrimSpace(strings.Join(words, " "))
	if f.query == "" {
		fmt.Fprintln(os.Stderr, "a search query is required")
		fs.Usage()
		return f, 2, false
	}
	if *translator < 0 || *season < 0 || *episode < 0 {
		fmt.Fprintln(os.Stderr, "-translator, -season and -episode must not be negative")
		return f, 2, false
	}
	f.translator, f.quality, f.season, f.episode, f.showURLs = *translator, strings.TrimSpace(*quality), *season, *episode, *showURLs
	return f, 0, true
}

// runProbe runs `momoshtrem hdrezka-probe` and returns the exit code: 0 when
// every check passed, 1 otherwise.
func runProbe(cfg *config.Config, f probeFlags) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p := &prober{
		client: hdrezka.NewClient(hdrezka.WithBaseURL(cfg.HDRezka.URL)),
		http:   &http.Client{Timeout: 30 * time.Second},
		out:    os.Stdout,
		flags:  f,
	}
	if err := p.run(ctx); err != nil {
		fmt.Fprintf(p.out, "\nFAIL: %v\n", err)
		return 1
	}
	if p.failed > 0 {
		fmt.Fprintf(p.out, "\nFAIL: %d check(s) failed\n", p.failed)
		return 1
	}
	fmt.Fprintln(p.out, "\nPASS")
	return 0
}

// prober walks search → title → translators → streams → range checks and
// prints what it finds.
type prober struct {
	client *hdrezka.Client
	http   *http.Client // the range checks, made by hand to see raw statuses
	out    io.Writer
	flags  probeFlags
	failed int
}

// probeCandidates is how many search results the probe prints.
const probeCandidates = 5

func (p *prober) run(ctx context.Context) error {
	f := p.flags
	p.printf("HDRezka probe\n  mirror      %s\n  user agent  %s\n  query       %q\n",
		p.client.BaseURL(), p.client.UserAgent(), f.query)

	title, err := p.search(ctx)
	if err != nil {
		return err
	}
	tr, err := p.pickTranslator(title)
	if err != nil {
		return err
	}

	start := time.Now()
	var stream *hdrezka.Stream
	what := fmt.Sprintf("translator %d %q", tr.ID, tr.Name)
	if title.Kind == hdrezka.KindSeries {
		season, episode, err := p.pickEpisode(ctx, title, tr)
		if err != nil {
			return err
		}
		what += fmt.Sprintf(", S%02dE%02d", season, episode)
		start = time.Now()
		stream, err = p.client.Episode(ctx, title.Ref(), tr.ID, season, episode)
		if err != nil {
			return fmt.Errorf("streams for %s: %w", what, err)
		}
	} else {
		stream, err = p.client.Movie(ctx, title.Ref(), tr)
		if err != nil {
			return fmt.Errorf("streams for %s: %w", what, err)
		}
	}
	format := "plain links"
	if stream.Legacy {
		format = "legacy #h links"
	}
	p.printf("\nStreams for %s (%s, %s)\n", what, seconds(time.Since(start)), format)

	sizes := p.measure(ctx, stream)
	p.printSubtitles(stream)

	q, err := p.pickQuality(stream)
	if err != nil {
		return err
	}
	size := sizes[q.Label]
	if size <= 0 {
		return fmt.Errorf("%s has no measured size", q.Label)
	}
	return p.checkRanges(ctx, q, size)
}

// search finds the query, ranks the hits and fetches the best one's page.
func (p *prober) search(ctx context.Context) (*hdrezka.Title, error) {
	f := p.flags
	start := time.Now()
	how := "quick search"
	results, err := p.client.QuickSearch(ctx, f.query)
	if err == nil && len(results) == 0 {
		how = "full search"
		results, err = p.client.Search(ctx, f.query)
	}
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	p.printf("\nSearch: %d result(s) (%s, %s)\n", len(results), how, seconds(time.Since(start)))
	if len(results) == 0 {
		return nil, errors.New("search: no results")
	}

	want := hdrezka.Want{Title: f.query}
	if f.season > 0 || f.episode > 0 {
		want.Kind = hdrezka.KindSeries
	}
	ranked := hdrezka.Rank(results, want)
	for i, m := range ranked {
		if i == probeCandidates {
			break
		}
		p.printf("  %d. %-6s  %s  %s  %s\n", i+1, m.Grade, describe(m.Title, m.OriginalTitle, m.Year), m.Category, m.Path)
	}

	start = time.Now()
	title, err := p.client.Title(ctx, ranked[0].Path)
	if err != nil {
		return nil, fmt.Errorf("title page %s: %w", ranked[0].Path, err)
	}
	p.printf("\nTitle (%s)\n  post %d · %s · %s\n  %s\n", seconds(time.Since(start)),
		title.PostID, title.Kind, title.Category, describe(title.Title, title.OriginalTitle, title.Year))

	p.printf("\nTranslators (%d)\n", len(title.Translators))
	for _, tr := range title.Translators {
		name := tr.Name
		if name == "" {
			name = "(not named on the page)"
		}
		p.printf("  %-5d %s%s\n", tr.ID, name, translatorFlags(tr))
	}
	return title, nil
}

// pickTranslator takes -translator, else the page default unless it's
// premium, else the first one that isn't.
func (p *prober) pickTranslator(title *hdrezka.Title) (hdrezka.Translator, error) {
	if id := p.flags.translator; id != 0 {
		tr, ok := title.Translator(id)
		if !ok {
			return tr, fmt.Errorf("translator %d isn't on the page", id)
		}
		p.printf("Using translator %d (-translator)\n", id)
		return tr, nil
	}
	if tr, ok := title.DefaultTranslator(); ok && !tr.Premium {
		p.printf("Using translator %d (the page default)\n", tr.ID)
		return tr, nil
	}
	for _, tr := range title.Translators {
		if !tr.Premium {
			p.printf("Using translator %d (the default is premium)\n", tr.ID)
			return tr, nil
		}
	}
	return hdrezka.Translator{}, errors.New("every translator is premium")
}

// pickEpisode lists the translator's episodes and takes -season/-episode,
// default S01E01 (or the first episode when there's no S01E01).
func (p *prober) pickEpisode(ctx context.Context, title *hdrezka.Title, tr hdrezka.Translator) (season, episode int, err error) {
	start := time.Now()
	seasons, err := p.client.Episodes(ctx, title.Ref(), tr.ID)
	if err != nil {
		return 0, 0, fmt.Errorf("episodes of translator %d: %w", tr.ID, err)
	}
	counts := make([]string, len(seasons))
	for i, s := range seasons {
		counts[i] = fmt.Sprintf("S%d: %d", s.Number, len(s.Episodes))
	}
	p.printf("\nEpisodes of translator %d (%s): %s\n", tr.ID, seconds(time.Since(start)), strings.Join(counts, ", "))

	season, episode = p.flags.season, p.flags.episode
	if season == 0 && episode == 0 && !hasEpisode(seasons, 1, 1) && len(seasons[0].Episodes) > 0 {
		return seasons[0].Number, seasons[0].Episodes[0], nil
	}
	season, episode = max(season, 1), max(episode, 1)
	if !hasEpisode(seasons, season, episode) {
		return 0, 0, fmt.Errorf("translator %d has no S%02dE%02d", tr.ID, season, episode)
	}
	return season, episode, nil
}

func hasEpisode(seasons []hdrezka.Season, season, episode int) bool {
	for _, s := range seasons {
		if s.Number != season {
			continue
		}
		for _, e := range s.Episodes {
			if e == episode {
				return true
			}
		}
	}
	return false
}

// measure prints every quality with the size of its MP4. A quality that
// isn't premium and can't be measured fails the probe.
func (p *prober) measure(ctx context.Context, s *hdrezka.Stream) map[string]int64 {
	sizes := map[string]int64{}
	for _, q := range s.Qualities {
		marker := ""
		if q.Premium {
			marker = "  premium (a teaser without an account)"
		}
		if q.MP4 == "" {
			p.printf("  %-12s HLS only%s\n", q.Label, marker)
			continue
		}
		start := time.Now()
		size, err := p.client.ContentLength(ctx, q.MP4)
		if err != nil {
			if !q.Premium {
				p.failed++
			}
			p.printf("  %-12s size unknown: %v%s\n", q.Label, err, marker)
		} else {
			sizes[q.Label] = size
			p.printf("  %-12s %s  %s%s\n", q.Label, formatSize(size), seconds(time.Since(start)), marker)
		}
		p.printf("               %s\n", redact(q.MP4, p.flags.showURLs))
	}
	return sizes
}

func (p *prober) printSubtitles(s *hdrezka.Stream) {
	if len(s.Subtitles) == 0 {
		p.printf("  subtitles: none\n")
		return
	}
	names := make([]string, len(s.Subtitles))
	for i, sub := range s.Subtitles {
		code := sub.Code
		if code == "" {
			code = "?"
		}
		names[i] = fmt.Sprintf("%s (%s)", sub.Language, code)
	}
	p.printf("  subtitles: %s\n", strings.Join(names, ", "))
}

// pickQuality takes -quality, else the best that isn't premium.
func (p *prober) pickQuality(s *hdrezka.Stream) (hdrezka.Quality, error) {
	if label := p.flags.quality; label != "" {
		q, ok := s.Find(label)
		if !ok {
			return q, fmt.Errorf("no %s quality", label)
		}
		if q.MP4 == "" {
			return q, fmt.Errorf("%s: %w", label, hdrezka.ErrNoMP4)
		}
		return q, nil
	}
	q, ok := s.Best()
	if !ok {
		return q, errors.New("no quality that isn't premium has an MP4")
	}
	return q, nil
}

// rangeChunk is how much each range check reads.
const rangeChunk = 1024

// checkRanges reads the start and the middle of the file with Range
// requests, and reports whether moov comes before mdat.
func (p *prober) checkRanges(ctx context.Context, q hdrezka.Quality, size int64) error {
	p.printf("\nChecks on %s (%s)\n", q.Label, formatSize(size))

	start := time.Now()
	head, err := p.readRange(ctx, q.MP4, 0, rangeChunk, size)
	isMP4 := err == nil && len(head) >= 8 && bytes.Equal(head[4:8], []byte("ftyp"))
	switch {
	case err != nil:
		p.check(false, "bytes 0-%d: %v", rangeChunk-1, err)
	case !isMP4:
		p.check(false, "bytes 0-%d: 206, but no ftyp at byte 4: not an MP4", rangeChunk-1)
	default:
		p.check(true, "bytes 0-%d: 206, total %s, ftyp at byte 4 (%s)", rangeChunk-1, commas(size), seconds(time.Since(start)))
	}

	mid := size / 2
	start = time.Now()
	if _, err := p.readRange(ctx, q.MP4, mid, rangeChunk, size); err != nil {
		p.check(false, "bytes %d-%d: %v", mid, mid+rangeChunk-1, err)
	} else {
		p.check(true, "bytes %d-%d: 206 from the middle (%s)", mid, mid+rangeChunk-1, seconds(time.Since(start)))
	}

	if !isMP4 {
		return nil
	}
	start = time.Now()
	boxes, err := scanBoxes(func(off, n int64) ([]byte, error) {
		if off+n <= int64(len(head)) {
			return head[off : off+n], nil
		}
		return p.readRange(ctx, q.MP4, off, n, size)
	}, size)
	if err != nil {
		p.check(false, "box scan: %v", err)
		return nil
	}
	fast, desc := faststart(boxes)
	// Not failing: a file with moov at the end still plays, after one more
	// seek.
	verdict := "yes"
	if !fast {
		verdict = "NO"
	}
	p.printf("  INFO  faststart: %s (%s; %s)\n", verdict, desc, seconds(time.Since(start)))
	return nil
}

func (p *prober) check(ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		p.failed++
	}
	p.printf("  %s  %s\n", status, fmt.Sprintf(format, args...))
}

// readRange GETs n bytes at off and checks the answer: a 206 for exactly
// that span of a file of size total.
func (p *prober) readRange(ctx context.Context, link string, off, n, total int64) ([]byte, error) {
	if off+n > total {
		n = total - off
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, errors.New("invalid link")
	}
	req.Header.Set("User-Agent", p.client.UserAgent())
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+n-1))
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, withoutURL(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("HTTP %d, want 206", resp.StatusCode)
	}
	first, last, size, err := hdrezka.ParseContentRange(resp.Header.Get("Content-Range"))
	switch {
	case err != nil:
		return nil, err
	case first != off || last != off+n-1:
		return nil, fmt.Errorf("206 for bytes %d-%d, want %d-%d", first, last, off, off+n-1)
	case size != total:
		return nil, fmt.Errorf("206 says the total is %d, want %d", size, total)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, n))
	if err != nil {
		return nil, withoutURL(err)
	}
	if int64(len(data)) != n {
		return nil, fmt.Errorf("read %d bytes, want %d", len(data), n)
	}
	return data, nil
}

// mp4Box is a top-level MP4 box.
type mp4Box struct {
	typ    string
	offset int64
	size   int64
}

// maxBoxes bounds the box scan; moov or mdat come within the first few.
const maxBoxes = 16

// scanBoxes walks a file's top-level boxes from the start, reading each
// header with readAt, until moov or mdat.
func scanBoxes(readAt func(off, n int64) ([]byte, error), total int64) ([]mp4Box, error) {
	var boxes []mp4Box
	for off := int64(0); off < total && len(boxes) < maxBoxes; {
		hdr, err := readAt(off, min(16, total-off))
		if err != nil {
			return boxes, err
		}
		if len(hdr) < 8 {
			return boxes, fmt.Errorf("short box header at byte %d", off)
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		if !isBoxType(typ) {
			return boxes, fmt.Errorf("no box at byte %d: not an MP4", off)
		}
		switch size {
		case 0: // the box runs to the end of the file
			size = total - off
		case 1: // a 64-bit size follows the type
			if len(hdr) < 16 {
				return boxes, fmt.Errorf("short %s header at byte %d", typ, off)
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		}
		if size < 8 || size > total-off {
			return boxes, fmt.Errorf("%s box at byte %d has an invalid size %d", typ, off, size)
		}
		boxes = append(boxes, mp4Box{typ: typ, offset: off, size: size})
		if typ == "moov" || typ == "mdat" {
			return boxes, nil
		}
		off += size
	}
	return boxes, errors.New("no moov or mdat among the first boxes")
}

// isBoxType reports whether s looks like a top-level box type: four
// letters, digits or spaces ("ftyp", "moov", "mdat", "free").
func isBoxType(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == ' ') {
			return false
		}
	}
	return len(s) == 4
}

// faststart reports whether moov comes before mdat, with the boxes seen.
func faststart(boxes []mp4Box) (bool, string) {
	parts := make([]string, len(boxes))
	for i, b := range boxes {
		parts[i] = fmt.Sprintf("%s at %s", b.typ, commas(b.offset))
	}
	desc := strings.Join(parts, ", ")
	if len(boxes) > 0 && boxes[len(boxes)-1].typ == "moov" {
		return true, "moov before mdat: " + desc
	}
	return false, "mdat before moov: " + desc
}

func (p *prober) printf(format string, args ...any) {
	fmt.Fprintf(p.out, format, args...)
}

// redact hides a signed link's path: https://stream.voidboost.one/…/g3l26.mp4.
func redact(link string, show bool) string {
	if show {
		return link
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return "(link hidden)"
	}
	return u.Scheme + "://" + u.Host + "/…/" + path.Base(u.Path)
}

func describe(title, original string, year int) string {
	s := title
	if original != "" && original != title {
		s += " / " + original
	}
	if year != 0 {
		s += fmt.Sprintf(" (%d)", year)
	}
	return s
}

func translatorFlags(tr hdrezka.Translator) string {
	var flags []string
	for _, f := range []struct {
		on   bool
		name string
	}{{tr.Default, "default"}, {tr.Premium, "premium"}, {tr.Camrip, "camrip"}, {tr.Ads, "ads"}, {tr.Director, "director's cut"}} {
		if f.on {
			flags = append(flags, f.name)
		}
	}
	if len(flags) == 0 {
		return ""
	}
	return "  [" + strings.Join(flags, ", ") + "]"
}

// formatSize prints "3,322,740,623 bytes (3.09 GiB)".
func formatSize(n int64) string {
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	v, unit := float64(n), "B"
	for _, u := range units {
		if v < 1024 {
			break
		}
		v, unit = v/1024, u
	}
	return fmt.Sprintf("%s bytes (%.2f %s)", commas(n), v, unit)
}

// commas groups digits by thousands.
func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func seconds(d time.Duration) string {
	return fmt.Sprintf("%.2f s", d.Seconds())
}

// withoutURL strips the request URL from transport errors: CDN links are
// signed.
func withoutURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
