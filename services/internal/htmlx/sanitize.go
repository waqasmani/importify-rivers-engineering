// Package htmlx holds HTML helpers shared by capture storage and website analysis.
package htmlx

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// dropElements are removed entirely from stored snapshots.
var dropElements = map[atom.Atom]bool{
	atom.Script: true, atom.Noscript: true, atom.Iframe: true, atom.Object: true, atom.Embed: true, atom.Template: true,
	atom.Frame: true, atom.Frameset: true,
}

// SanitizeSnapshot removes scripts, embedded frames, event handlers, form values and inline
// credentials from a page snapshot. The result is stored for evidence only and is never served
// as HTML by REA.
func SanitizeSnapshot(src string) (string, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return "", err
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode && dropElements[c.DataAtom] {
				n.RemoveChild(c)
			} else if c.Type == html.CommentNode {
				n.RemoveChild(c)
			} else {
				if c.Type == html.ElementNode {
					cleanAttrs(c)
				}
				walk(c)
			}
			c = next
		}
	}
	walk(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func cleanAttrs(n *html.Node) {
	isInput := n.DataAtom == atom.Input || n.DataAtom == atom.Textarea || n.DataAtom == atom.Select || n.DataAtom == atom.Option
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		switch {
		case strings.HasPrefix(key, "on"):
			continue
		case isInput && (key == "value" || key == "checked" || key == "selected"):
			continue
		case key == "nonce" || key == "integrity" || strings.Contains(key, "token") || strings.Contains(key, "csrf"):
			continue
		case (key == "href" || key == "src" || key == "action" || key == "formaction" || key == "xlink:href") &&
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Val)), "javascript:"):
			continue
		}
		kept = append(kept, a)
	}
	n.Attr = kept
	if n.DataAtom == atom.Textarea {
		for c := n.FirstChild; c != nil; c = n.FirstChild {
			n.RemoveChild(c)
		}
	}
}

// Text extracts visible text from HTML, separating block elements with newlines.
func Text(src string, max int) string {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if b.Len() >= max {
			return
		}
		if n.Type == html.ElementNode && (dropElements[n.DataAtom] || n.DataAtom == atom.Style || n.DataAtom == atom.Head) {
			return
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				b.WriteString(t)
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.P, atom.Div, atom.Li, atom.Br, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Tr, atom.Section, atom.Article, atom.Td, atom.Th:
				b.WriteByte('\n')
			}
		}
	}
	walk(doc)
	out := b.String()
	lines := strings.Split(out, "\n")
	clean := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			clean = append(clean, l)
		}
	}
	res := strings.Join(clean, "\n")
	if len(res) > max {
		res = res[:max]
	}
	return res
}
