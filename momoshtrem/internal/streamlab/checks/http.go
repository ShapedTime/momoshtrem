package checks

import (
	"fmt"
	"net/http"
	"strings"
)

// RangeResponse checks the answer to a Range GET of [start, end] of a file
// of size bytes: 206 Partial Content, Content-Range "bytes start-end/size"
// and Content-Length end-start+1. For an open-ended range ("bytes=start-")
// end is size-1.
func RangeResponse(resp *http.Response, start, end, size int64) error {
	if resp == nil {
		return fmt.Errorf("range %d-%d: no response", start, end)
	}
	var problems []string
	if resp.StatusCode != http.StatusPartialContent {
		problems = append(problems, fmt.Sprintf("status %d, want 206", resp.StatusCode))
	}
	if got, want := resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-%d/%d", start, end, size); got != want {
		problems = append(problems, fmt.Sprintf("Content-Range %q, want %q", got, want))
	}
	if got, want := resp.Header.Get("Content-Length"), fmt.Sprint(end-start+1); got != want {
		problems = append(problems, fmt.Sprintf("Content-Length %q, want %q", got, want))
	}
	if len(problems) > 0 {
		return fmt.Errorf("range %d-%d of %d bytes: %s", start, end, size, strings.Join(problems, "; "))
	}
	return nil
}

// Panics returns the server error log entries (stack.ServerErrors) that
// report a handler panic net/http recovered.
func Panics(serverErrors []string) []string {
	var out []string
	for _, e := range serverErrors {
		if strings.Contains(e, "http: panic serving") {
			out = append(out, e)
		}
	}
	return out
}
