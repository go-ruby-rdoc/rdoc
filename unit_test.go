package rdoc

// Deterministic, ruby-free unit tests. Together with the golden-oracle tests
// these exercise every branch so the 100% coverage gate holds without a Ruby
// toolchain present.

import (
	"strings"
	"testing"
)

func TestPipeAndMiscOracle(t *testing.T) {
	var c struct {
		Pipe map[string]struct{ In, HTML string }
		Misc map[string]struct{ In, HTML string }
	}
	readJSON(t, "pipe.json", &c)

	for name, tc := range c.Pipe {
		opts := &HTMLOptions{OutputDecoration: true, Pipe: true}
		doc := Parse(tc.In)
		f := NewToHtml(opts)
		doc.Accept(f)
		if got := f.endAccepting(); got != tc.HTML {
			t.Errorf("pipe[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.HTML, got)
		}
	}
	for name, tc := range c.Misc {
		if got := ToHTML(tc.In); got != tc.HTML {
			t.Errorf("misc[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.HTML, got)
		}
	}
}

func TestListTypeString(t *testing.T) {
	cases := map[ListType]string{
		ListBullet: "BULLET", ListLabel: "LABEL", ListLalpha: "LALPHA",
		ListNote: "NOTE", ListNumber: "NUMBER", ListUalpha: "UALPHA",
		ListType(99): "BULLET",
	}
	for lt, want := range cases {
		if got := lt.String(); got != want {
			t.Errorf("ListType(%d).String()=%q want %q", lt, got, want)
		}
	}
	// listType fallback for a non-list kind
	if tkText.listType() != ListBullet {
		t.Error("listType fallback")
	}
}

func TestDefaultHTMLOptions(t *testing.T) {
	o := DefaultHTMLOptions()
	if !o.OutputDecoration || o.Pipe {
		t.Errorf("defaults: %+v", o)
	}
	// NewToHtml with nil opts uses defaults
	f := NewToHtml(nil)
	if f.opts == nil || !f.opts.OutputDecoration {
		t.Error("NewToHtml(nil) should use defaults")
	}
}

func TestNoDecorationHeading(t *testing.T) {
	opts := &HTMLOptions{OutputDecoration: false}
	doc := Parse("= Title\n")
	f := NewToHtml(opts)
	doc.Accept(f)
	got := f.endAccepting()
	if strings.Contains(got, "id=") {
		t.Errorf("no-decoration heading should omit id: %q", got)
	}
	if !strings.Contains(got, `<a href="#label-Title">`) {
		t.Errorf("heading anchor missing: %q", got)
	}
}

func TestCrossRef(t *testing.T) {
	resolved := map[string]string{"Foo": "Foo.html"}
	opts := &HTMLOptions{
		OutputDecoration: true,
		CrossRef: func(name string) (string, bool) {
			h, ok := resolved[name]
			return h, ok
		},
	}
	doc := Parse("See Foo and Bar here.\n")
	f := NewToHtml(opts)
	doc.Accept(f)
	got := f.endAccepting()
	if !strings.Contains(got, `<a href="Foo.html">Foo</a>`) {
		t.Errorf("crossref Foo not linked: %q", got)
	}
	// suppressed crossref \Foo
	doc2 := Parse("See \\Foo suppressed.\n")
	f2 := NewToHtml(opts)
	doc2.Accept(f2)
	if strings.Contains(f2.endAccepting(), "<a ") {
		t.Errorf("suppressed crossref should not link: %q", f2.endAccepting())
	}
}

func TestRawElement(t *testing.T) {
	raw := &Raw{Parts: []string{"<custom>", "html"}}
	f := NewToHtml(nil)
	f.startAccepting()
	raw.accept(f)
	if got := f.endAccepting(); got != "<custom>\nhtml" {
		t.Errorf("raw: %q", got)
	}
	// markdown + rdoc raw
	m := NewToMarkdown()
	m.startAccepting()
	raw.accept(m)
	if m.endAccepting() != "<custom>\nhtml" {
		t.Errorf("md raw: %q", m.endAccepting())
	}
	r := NewToRdoc()
	r.startAccepting()
	raw.accept(r)
	if r.endAccepting() != "<custom>\nhtml" {
		t.Errorf("rdoc raw: %q", r.endAccepting())
	}
}

func TestDocumentAcceptNested(t *testing.T) {
	// a nested Document flattens through accept
	inner := &Document{Parts: []Element{&Paragraph{Parts: []string{"x"}}}}
	outer := &Document{Parts: []Element{inner}}
	f := NewToHtml(nil)
	outer.Accept(f)
	if got := f.endAccepting(); got != "\n<p>x</p>\n" {
		t.Errorf("nested doc: %q", got)
	}
}

func TestEmptyParagraphAndModel(t *testing.T) {
	// Paragraph.text with no parts
	p := &Paragraph{}
	if p.text() != "" {
		t.Error("empty paragraph text")
	}
	// Verbatim ruby flag + normalize trailing blanks
	v := &Verbatim{format: "ruby"}
	if !v.ruby() {
		t.Error("verbatim ruby flag")
	}
	v.Parts = []string{"a\n", "\n", "\n", "b\n"}
	v.normalize()
	if strings.Count(strings.Join(v.Parts, ""), "\n\n") != 0 && len(v.Parts) != 3 {
		t.Errorf("normalize: %v", v.Parts)
	}
}

func TestAliasExtraction(t *testing.T) {
	src := `class C
  def foo; end
  # an alias
  alias_method :bar, :foo
  alias baz foo
end
`
	top := Extract("t.rb", src)
	c := top.ClassesAndModule[0]
	if len(c.Aliases) != 2 {
		t.Fatalf("aliases: %d", len(c.Aliases))
	}
	if c.Aliases[0].NewName != "bar" || c.Aliases[0].OldName != "foo" || c.Aliases[0].Comment != "an alias" {
		t.Errorf("alias0: %+v", c.Aliases[0])
	}
	if c.Aliases[1].NewName != "baz" || c.Aliases[1].OldName != "foo" {
		t.Errorf("alias1: %+v", c.Aliases[1])
	}
}

func TestExtractTopLevelName(t *testing.T) {
	top := Extract("myfile.rb", "")
	if top.Name != "myfile.rb" || len(top.ClassesAndModule) != 0 {
		t.Errorf("empty extract: %+v", top)
	}
}

func TestCRLFSource(t *testing.T) {
	top := Extract("t.rb", "class C\r\n  def m; end\r\nend\r\n")
	if len(top.ClassesAndModule) != 1 || top.ClassesAndModule[0].Name != "C" {
		t.Errorf("crlf: %+v", top.ClassesAndModule)
	}
}

func TestCallSeqNoDirective(t *testing.T) {
	comment, seq := extractCallSeq("just a comment\nno directive")
	if comment != "just a comment\nno directive" || seq != "" {
		t.Errorf("no-directive: comment=%q seq=%q", comment, seq)
	}
	// directive followed by a non-indented line ends the sequence
	c2, s2 := extractCallSeq("call-seq:\n  f(x)\nback to text")
	if s2 != "f(x)\n" || !strings.Contains(c2, "back to text") {
		t.Errorf("ended seq: comment=%q seq=%q", c2, s2)
	}
}

func TestHighlightFallback(t *testing.T) {
	// unterminated string -> fallback
	if _, ok := HighlightRuby(`x = "unterminated`); ok {
		t.Error("expected fallback on unterminated string")
	}
	// unterminated regexp -> fallback
	if _, ok := HighlightRuby("x =~ /unterminated"); ok {
		t.Error("expected fallback on unterminated regexp")
	}
}

func TestHighlightGlobalsAndClassVars(t *testing.T) {
	got, ok := HighlightRuby("$g = @@c")
	if !ok {
		t.Fatal("highlight failed")
	}
	if strings.Contains(got, "ruby-ivar") {
		t.Errorf("$g/@@c should be identifiers: %q", got)
	}
}

func TestToLabelTidylink(t *testing.T) {
	// braced and single-word tidylinks reduce to their label in a heading id
	if got := toLabel("{the label}[http://x]"); got != "the+label" {
		t.Errorf("braced label: %q", got)
	}
	if got := toLabel("word[http://x]"); got != "word" {
		t.Errorf("word label: %q", got)
	}
}

func TestVerbatimExplicitRuby(t *testing.T) {
	// a Verbatim flagged ruby is highlighted even without Parseable
	v := &Verbatim{format: "ruby", Parts: []string{"x = 1\n"}}
	opts := &HTMLOptions{OutputDecoration: true, Highlighter: HighlightRuby}
	f := NewToHtml(opts)
	f.startAccepting()
	v.accept(f)
	got := f.endAccepting()
	if !strings.Contains(got, `<pre class="ruby">`) {
		t.Errorf("explicit ruby verbatim: %q", got)
	}
}

func TestHighlighterReturnsFalseFallback(t *testing.T) {
	// Highlighter present but returns ok=false -> plain escaped <pre>
	opts := &HTMLOptions{
		OutputDecoration: true,
		Parseable:        func(string) bool { return true },
		Highlighter:      func(string) (string, bool) { return "", false },
	}
	doc := Parse("  some code\n")
	f := NewToHtml(opts)
	doc.Accept(f)
	if got := f.endAccepting(); got != "\n<pre>some code</pre>\n" {
		t.Errorf("fallback verbatim: %q", got)
	}
}
