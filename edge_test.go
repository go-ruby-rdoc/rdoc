package rdoc

import "testing"

func TestEdgeOracle(t *testing.T) {
	var c struct{ HTML map[string]struct{ In, HTML string } }
	readJSON(t, "edge.json", &c)
	// cases whose golden output contains highlighting need the highlighter+parseable
	opts := &HTMLOptions{OutputDecoration: true, Highlighter: HighlightRuby}
	for name, tc := range c.HTML {
		var got string
		if containsRubyClass(tc.HTML) {
			opts.Parseable = func(string) bool { return true }
			doc := Parse(tc.In)
			f := NewToHtml(opts)
			doc.Accept(f)
			got = f.endAccepting()
		} else {
			got = ToHTML(tc.In)
		}
		if got != tc.HTML {
			t.Errorf("edge[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.HTML, got)
		}
	}
}

func TestParserOracle(t *testing.T) {
	var c struct{ HTML map[string]struct{ In, HTML string } }
	readJSON(t, "parser.json", &c)
	opts := &HTMLOptions{OutputDecoration: true, Highlighter: HighlightRuby}
	for name, tc := range c.HTML {
		var got string
		if containsRubyClass(tc.HTML) {
			opts.Parseable = func(string) bool { return true }
			doc := Parse(tc.In)
			f := NewToHtml(opts)
			doc.Accept(f)
			got = f.endAccepting()
		} else {
			got = ToHTML(tc.In)
		}
		if got != tc.HTML {
			t.Errorf("parser[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.HTML, got)
		}
	}
}
