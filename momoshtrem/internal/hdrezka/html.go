package hdrezka

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// parseHTML parses a page or a fragment. The parser repairs any markup, so
// it only fails on a read error, which a byte slice can't have.
func parseHTML(body []byte) (*html.Node, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, &ParseError{What: "HTML"}
	}
	return doc, nil
}

// findAll returns the elements under n (not n itself), in document order,
// that match.
func findAll(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && match(c) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// findFirst returns the first element under n (not n itself) that matches,
// or nil.
func findFirst(n *html.Node, match func(*html.Node) bool) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && match(c) {
			return c
		}
		if found := findFirst(c, match); found != nil {
			return found
		}
	}
	return nil
}

// textOf returns the text of the first element under n that matches, or "".
func textOf(n *html.Node, match func(*html.Node) bool) string {
	if found := findFirst(n, match); found != nil {
		return text(found)
	}
	return ""
}

// withClass matches elements that have class among their classes.
func withClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool { return hasClass(n, class) }
}

// withTag matches elements by tag name.
func withTag(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Data == tag }
}

// withAttr matches elements whose attribute key equals value.
func withAttr(key, value string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		v, ok := attr(n, key)
		return ok && v == value
	}
}

func hasClass(n *html.Node, class string) bool {
	v, _ := attr(n, "class")
	for _, c := range strings.Fields(v) {
		if c == class {
			return true
		}
	}
	return false
}

// attr returns an attribute's value and whether the element has it.
func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// text returns the text under n with runs of whitespace collapsed.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// ownText returns only n's direct text children, collapsed: the "(Original,
// 2014)" between a search hit's spans.
func ownText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
