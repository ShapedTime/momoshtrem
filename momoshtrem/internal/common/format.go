package common

import (
	"path"
	"strconv"
	"strings"
)

// Itoa converts an int to a string using strconv.Itoa.
func Itoa(n int) string {
	return strconv.Itoa(n)
}

// Itoa64 converts an int64 to a string.
func Itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}

// PadZero pads an integer with leading zeros to reach the specified width.
func PadZero(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// CleanPath normalizes a path by cleaning it and ensuring it starts with /.
func CleanPath(p string) string {
	p = path.Clean(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
