package rdoc

// This file ports RDoc::Markup::ToLabel and RDoc::Text#to_html (the smart-quote
// / dash / entity pass) and the heading-anchor label generation.

import (
	"regexp"
	"strings"
)

// toLabel converts heading text into an HTML-safe anchor label, mirroring
// RDoc::Markup::ToLabel#convert: run the inline flow (stripping tidylinks and
// crossref markers to their text), then CGI.escape and map '%' -> '-' and drop
// a leading '-'.
func toLabel(text string) string {
	am := newAttributeManager()
	// ToLabel registers a TIDYLINK handler (and a CROSSREF handler). We only
	// need the textual reduction; register a tidylink handler so "{x}[y]" and
	// "w[y]" reduce to their label text.
	am.addRegexpHandling(reTidylinkLabel, attrUserBase, false)

	flow := am.flow(text)
	var b strings.Builder
	for _, f := range flow {
		switch v := f.(type) {
		case flowString:
			b.WriteString(string(v))
		case regexpHandling:
			b.WriteString(handleLabelTidylink(v.text))
		case attrChanger:
			// bold/em/tt contribute no characters in a label
		}
	}
	return cgiEscapeLabel(b.String())
}

var reTidylinkLabel = regexp.MustCompile(`(((\{.*?\})|\b\S+?)\[\S+?\])`)

func handleLabelTidylink(text string) string {
	if m := regexp.MustCompile(`\{(.*?)\}\[(.*?)\]`).FindStringSubmatch(text); m != nil {
		return m[1]
	}
	if m := regexp.MustCompile(`(\S+)\[(.*?)\]`).FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return text
}

// cgiEscapeLabel reproduces Ruby's CGI.escape(label).gsub('%','-').sub(/^-/,'').
func cgiEscapeLabel(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ':
			b.WriteByte('+')
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigit(c >> 4))
			b.WriteByte(hexDigit(c & 0xf))
		}
	}
	out := strings.ReplaceAll(b.String(), "%", "-")
	out = strings.TrimPrefix(out, "-")
	return out
}

func hexDigit(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + (n - 10)
}
