package rdoc

// White-box unit tests targeting individual branches and helpers so the suite
// reaches full statement coverage deterministically (no Ruby required).

import (
	"strings"
	"testing"
)

func TestFlowItemMarkers(t *testing.T) {
	// exercise the isFlow marker methods
	var f flowItem
	f = flowString("x")
	f.isFlow()
	f = attrChanger{}
	f.isFlow()
	f = regexpHandling{}
	f.isFlow()
}

func TestNonMatchingWordPair(t *testing.T) {
	am := newAttributeManager()
	// register a non-matching pair "{...}" -> a fresh bit, exercising
	// addWordPair's else-branch and convertWordPairMap.
	am.addWordPair("{", "}", attrUserBase<<8, false)
	flow := am.flow("a {word} b")
	var got strings.Builder
	for _, fi := range flow {
		if s, ok := fi.(flowString); ok {
			got.WriteString(string(s))
		}
	}
	if got.String() != "a word b" {
		t.Errorf("non-matching pair flow strings: %q", got.String())
	}
	// the bit must have been set on "word"
	sawOn := false
	for _, fi := range flow {
		if c, ok := fi.(attrChanger); ok && c.turnOn == attrUserBase<<8 {
			sawOn = true
		}
	}
	if !sawOn {
		t.Error("non-matching pair did not turn on its attribute")
	}
}

func TestNonMatchingWordPairNoMatch(t *testing.T) {
	am := newAttributeManager()
	am.addWordPair("{", "}", attrUserBase<<8, false)
	// no delimiters -> convertWordPairMap finds nothing
	flow := am.flow("plain text")
	if len(flow) != 1 {
		t.Errorf("plain text flow: %+v", flow)
	}
}

func TestAppendUniq(t *testing.T) {
	s := []string{"a", "b"}
	if got := appendUniq(s, "a"); len(got) != 2 {
		t.Errorf("dup not skipped: %v", got)
	}
	if got := appendUniq(s, "c"); len(got) != 3 {
		t.Errorf("new not added: %v", got)
	}
}

func TestAttrSpanBounds(t *testing.T) {
	a := newAttrSpan(3)
	if a.at(-1) != 0 || a.at(5) != 0 {
		t.Error("out-of-range attr should be 0")
	}
	// setAttrs past end is clamped
	a.setAttrs(1, 100, attrBold)
	if a.at(2) == 0 {
		t.Error("setAttrs should have set in-range bits")
	}
}

func TestFindNested(t *testing.T) {
	parent := &ClassModule{Name: "P"}
	child := &ClassModule{Name: "C"}
	parent.Nested = []*ClassModule{child}
	if parent.findNested("C") != child {
		t.Error("findNested existing")
	}
	if parent.findNested("X") != nil {
		t.Error("findNested missing")
	}
}

func TestReopenClass(t *testing.T) {
	// reopening a class merges into the existing CM (lookupCM hit at top level)
	src := "class C\n  def a; end\nend\nclass C\n  def b; end\nend\n"
	top := Extract("t.rb", src)
	if len(top.ClassesAndModule) != 1 {
		t.Fatalf("reopen should not duplicate: %d", len(top.ClassesAndModule))
	}
	if len(top.ClassesAndModule[0].Methods) != 2 {
		t.Errorf("reopen methods: %d", len(top.ClassesAndModule[0].Methods))
	}
}

func TestQualifiedNested(t *testing.T) {
	top := Extract("t.rb", "class A::B::C\n  def m; end\nend\n")
	if len(top.ClassesAndModule) != 1 || top.ClassesAndModule[0].Name != "A" {
		t.Fatalf("top should be A: %+v", top.ClassesAndModule)
	}
	a := top.ClassesAndModule[0]
	if !a.IsModule || len(a.Nested) != 1 || a.Nested[0].Name != "B" {
		t.Errorf("A nesting: %+v", a)
	}
	b := a.Nested[0]
	if len(b.Nested) != 1 || b.Nested[0].Name != "C" || b.Nested[0].FullName != "A::B::C" {
		t.Errorf("B nesting: %+v", b)
	}
}

func TestCommentResetOnBlank(t *testing.T) {
	// a blank line between comment and def detaches the comment
	src := "# stray\n\nclass C\n  def m; end\nend\n"
	top := Extract("t.rb", src)
	if top.ClassesAndModule[0].Comment != "" {
		t.Errorf("comment should detach across blank line: %q", top.ClassesAndModule[0].Comment)
	}
}

func TestDoubleHashBlockComment(t *testing.T) {
	// "## text" drops the extra hash on the first line
	src := "## A class.\nclass C\nend\n"
	top := Extract("t.rb", src)
	if got := top.ClassesAndModule[0].Comment; got != "A class." {
		t.Errorf("## comment: %q", got)
	}
}

func TestTakeCommentTrimsBlankEdges(t *testing.T) {
	e := &extractor{}
	e.haveComment = true
	e.comment = []string{"", "body", ""}
	if got := e.takeComment(); got != "body" {
		t.Errorf("trim: %q", got)
	}
	// no comment -> empty
	if e.takeComment() != "" {
		t.Error("empty take")
	}
}

func TestParserHelpers(t *testing.T) {
	// lineRest with no newline
	if lineRest("abc") != "abc" {
		t.Error("lineRest no-newline")
	}
	// labelConsumed without trailing newline
	if labelConsumed("[x] ") != 4 {
		t.Errorf("labelConsumed: %d", labelConsumed("[x] "))
	}
	// blockquoteConsumed with no word, no newline
	if blockquoteConsumed(">>>", "") != 3 {
		t.Errorf("blockquoteConsumed: %d", blockquoteConsumed(">>>", ""))
	}
	// listMarker variants
	if listMarker(token{kind: tkNote, str: "n"}) != "n::" {
		t.Error("listMarker note")
	}
	if listMarker(token{kind: tkNumber, str: "1"}) != "1." {
		t.Error("listMarker number")
	}
}

func TestSkipNoToken(t *testing.T) {
	p := &parser{}
	if p.skip(tkNewline) {
		t.Error("skip on empty stream should be false")
	}
}

func TestBuildHeadingNoText(t *testing.T) {
	// "=\n" -> heading level 1 with empty text (build_heading unget path)
	doc := Parse("=\n")
	h, ok := doc.Parts[0].(*Heading)
	if !ok || h.Level != 1 || h.Text != "" {
		t.Errorf("empty heading: %+v", doc.Parts[0])
	}
}

func TestVerbatimWithListMarkers(t *testing.T) {
	// a verbatim block that contains list/header/rule/blockquote markers
	// exercises the build_verbatim marker reconstruction.
	in := "  text\n    * a\n    1. b\n    [x] c\n    n:: d\n    = h\n    --- r\n    >>> q\n"
	got := ToHTML(in)
	if !strings.Contains(got, "<pre>") {
		t.Errorf("verbatim markers: %q", got)
	}
	for _, want := range []string{"* a", "1. b", "[x] c", "n:: d", "= h", "--- r", "&gt;&gt;&gt; q"} {
		if !strings.Contains(got, want) {
			t.Errorf("verbatim missing %q in %q", want, got)
		}
	}
}

func TestTextToHTMLBranches(t *testing.T) {
	cases := map[string]string{
		"x``y":  "x“y",   // backtick double quote
		"plain": "plain", // no significant chars (advance-to-end branch)
		"a...b": "…",     // ellipsis
		"--":    "–",     // en dash
	}
	for in, want := range cases {
		if got := textToHTML(in); !strings.Contains(got, want) {
			t.Errorf("textToHTML(%q)=%q want contains %q", in, got, want)
		}
	}
}

func TestHandleRDocLinkVariants(t *testing.T) {
	h := NewToHtml(nil)
	if got := h.handleRDocLink("rdoc-ref:Foo"); got != "Foo" {
		t.Errorf("rdoc-ref: %q", got)
	}
	if got := h.handleRDocLink("rdoc-image:pic.png"); got != `<img src="pic.png">` {
		t.Errorf("rdoc-image: %q", got)
	}
	if got := h.handleRDocLink("rdoc-image:pic.png:Alt"); got != `<img src="pic.png" alt="Alt">` {
		t.Errorf("rdoc-image alt: %q", got)
	}
	if got := h.handleRDocLink("rdoc-other:thing"); got != "thing" {
		t.Errorf("rdoc-other: %q", got)
	}
	// rdoc-label with footmark/foottext prefixes
	if got := h.handleRDocLink("rdoc-label:footmark-1"); !strings.Contains(got, "href=\"#footmark-1\"") {
		t.Errorf("rdoc-label footmark: %q", got)
	}
	if got := h.handleRDocLink("rdoc-label:foottext-1"); !strings.Contains(got, "1") {
		t.Errorf("rdoc-label foottext: %q", got)
	}
	// unknown scheme returns escaped text
	if got := h.handleRDocLink("notrdoc"); got != "notrdoc" {
		t.Errorf("non-rdoc: %q", got)
	}
}

func TestSplitImageAltNoColon(t *testing.T) {
	u, alt := splitImageAlt("pic.png")
	if u != "pic.png" || alt != "" {
		t.Errorf("no-colon: %q %q", u, alt)
	}
	// colon followed by slash (URL) is not a split point
	u2, alt2 := splitImageAlt("http://x/y.png")
	if alt2 != "" || u2 != "http://x/y.png" {
		t.Errorf("url-colon: %q %q", u2, alt2)
	}
}

func TestGenURLImageAndFile(t *testing.T) {
	h := NewToHtml(nil)
	// image link
	if got := h.genURL("http://x.com/a.png", "x"); got != `<img src="http://x.com/a.png" />` {
		t.Errorf("img: %q", got)
	}
	// .rb file link rewrite
	if got := h.genURL("path/to/file.rb", "file"); !strings.Contains(got, "file_rb.html") {
		t.Errorf("file rewrite: %q", got)
	}
	// link: scheme is not rewritten
	if got := h.genURL("link:foo.rb", "f"); !strings.Contains(got, "foo.rb") {
		t.Errorf("link scheme: %q", got)
	}
}

func TestHandleTidyLinkNoMatch(t *testing.T) {
	h := NewToHtml(nil)
	if got := h.handleTidyLink("not a link"); got != "not a link" {
		t.Errorf("tidy no-match: %q", got)
	}
}

func TestConvertRegexpHandlingDefault(t *testing.T) {
	// a regexpHandling with no recognised bit returns escaped text
	h := NewToHtml(nil)
	got := h.convertRegexpHandling(regexpHandling{bit: attrRegexp, text: "<x>"})
	if got != "&lt;x&gt;" {
		t.Errorf("default regexp handling: %q", got)
	}
}

func TestLabelTidylinkNoMatch(t *testing.T) {
	if got := handleLabelTidylink("plain"); got != "plain" {
		t.Errorf("label tidylink no-match: %q", got)
	}
}

func TestAcceptBlankLineHTML(t *testing.T) {
	f := NewToHtml(nil)
	f.startAccepting()
	(&BlankLine{}).accept(f)
	if f.endAccepting() != "" {
		t.Errorf("blank line html should be empty: %q", f.endAccepting())
	}
}

func TestListItemStartNoteLabel(t *testing.T) {
	// note + label list item start HTML via direct render
	got := ToHTML("term:: definition\n")
	if !strings.Contains(got, "<dt>term</dt>") || !strings.Contains(got, "<dd>") {
		t.Errorf("note list item: %q", got)
	}
}

func TestHighlightSymbolAndRegexpFlags(t *testing.T) {
	got, ok := HighlightRuby("x = :sym; y =~ /ab/i")
	if !ok {
		t.Fatal("highlight failed")
	}
	if !strings.Contains(got, `ruby-value">:sym`) {
		t.Errorf("symbol: %q", got)
	}
	if !strings.Contains(got, `ruby-regexp">/ab/i`) {
		t.Errorf("regexp flags: %q", got)
	}
}

func TestHighlightLoneColon(t *testing.T) {
	// a ':' that is neither '::' nor a symbol-start passes through
	got, ok := HighlightRuby("h[1] = 2")
	if !ok {
		t.Fatal("highlight failed")
	}
	if strings.Contains(got, "ruby-value\">:") {
		t.Errorf("unexpected symbol: %q", got)
	}
}

func TestHighlightDivisionVsRegexp(t *testing.T) {
	// after an identifier, '/' is division (not a regexp), so no ruby-regexp
	got, ok := HighlightRuby("a = b / c")
	if !ok {
		t.Fatal("highlight failed")
	}
	if strings.Contains(got, "ruby-regexp") {
		t.Errorf("division mis-detected as regexp: %q", got)
	}
}

func TestHighlightConstantVsIdent(t *testing.T) {
	got, _ := HighlightRuby("Foo and bar")
	if !strings.Contains(got, `ruby-constant">Foo`) {
		t.Errorf("constant: %q", got)
	}
	if !strings.Contains(got, `ruby-identifier">bar`) {
		t.Errorf("identifier: %q", got)
	}
}

func TestEmptyHighlight(t *testing.T) {
	got, ok := HighlightRuby("")
	if !ok || got != "" {
		t.Errorf("empty highlight: %q %v", got, ok)
	}
}

func TestMarkdownAndRdocVerbatimMultiline(t *testing.T) {
	if got := ToMarkdownString("  a\n  b\n"); !strings.Contains(got, "    a") {
		t.Errorf("md verbatim: %q", got)
	}
	if got := ToRdocString("  a\n  b\n"); !strings.Contains(got, "  a") {
		t.Errorf("rdoc verbatim: %q", got)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	if wrapText("", 76) != "" {
		t.Error("wrap empty")
	}
	if wrapText("   ", 76) != "" {
		t.Error("wrap whitespace")
	}
}

func TestItoaN(t *testing.T) {
	if itoaN(0) != "0" || itoaN(7) != "7" || itoaN(42) != "42" {
		t.Errorf("itoaN: %q %q %q", itoaN(0), itoaN(7), itoaN(42))
	}
}

func TestAlphaMarker(t *testing.T) {
	if alphaMarker('a', 1) != "a" || alphaMarker('a', 2) != "b" || alphaMarker('A', 27) != "A" {
		t.Error("alphaMarker")
	}
}

func TestFileLinkWithFragment(t *testing.T) {
	h := NewToHtml(nil)
	if got := h.genURL("file.rb#frag", "x"); got != `<a href="file_rb.html#frag">x</a>` {
		t.Errorf("file frag: %q", got)
	}
}

func TestParserRuleAtEOF(t *testing.T) {
	doc := Parse("---")
	if len(doc.Parts) != 1 {
		t.Fatalf("rule eof parts: %d", len(doc.Parts))
	}
	if _, ok := doc.Parts[0].(*Rule); !ok {
		t.Errorf("rule eof: %T", doc.Parts[0])
	}
}

func TestHeaderWhitespaceThenNewline(t *testing.T) {
	// "==   \ntext" -> header level 2 whose text is on the next line
	doc := Parse("==   \ntext\n")
	h, ok := doc.Parts[0].(*Heading)
	if !ok || h.Level != 2 || h.Text != "text" {
		t.Errorf("ws header: %+v", doc.Parts[0])
	}
}

func TestHeaderNewlineOnly(t *testing.T) {
	// "==\n" (no trailing space) -> header with empty text
	doc := Parse("==\n")
	h := doc.Parts[0].(*Heading)
	if h.Level != 2 || h.Text != "" {
		t.Errorf("nl header: %+v", h)
	}
}

func TestIsConstantNameEmpty(t *testing.T) {
	if isConstantName("") {
		t.Error("empty is not a constant")
	}
}

func TestHighlightStringInterpolation(t *testing.T) {
	got, ok := HighlightRuby(`x = "a#{b}c"`)
	if !ok || !strings.Contains(got, `ruby-node">&quot;a#{b}c&quot;`) {
		t.Errorf("interp: %q", got)
	}
}

func TestHighlightTrailingBang(t *testing.T) {
	got, _ := HighlightRuby("x = a.empty?")
	if !strings.Contains(got, `ruby-identifier">empty?`) {
		t.Errorf("bang ident: %q", got)
	}
}

func TestHighlightEscapeInStringAndRegexp(t *testing.T) {
	if got, ok := HighlightRuby(`s = "a\"b"`); !ok || !strings.Contains(got, "ruby-string") {
		t.Errorf("esc string: %q", got)
	}
	if got, ok := HighlightRuby(`r = /a\/b/`); !ok || !strings.Contains(got, "ruby-regexp") {
		t.Errorf("esc regexp: %q", got)
	}
}

func TestHighlightSymbolEdge(t *testing.T) {
	// ':' at end of input is not a symbol start
	if h := (&rubyHL{src: ":"}); h.isSymbolStart() {
		t.Error("lone trailing colon")
	}
}

func TestTextDoubleQuoteEntity(t *testing.T) {
	// &quot; entity branch + close-quote toggling
	got := textToHTML("&quot;hi&quot;")
	if !strings.Contains(got, entOpenDQuote) || !strings.Contains(got, entCloseDQuote) {
		t.Errorf("quot entity: %q", got)
	}
	// '' double close-quote
	got2 := textToHTML("a''b")
	if !strings.Contains(got2, entCloseDQuote) {
		t.Errorf("tick double: %q", got2)
	}
}

func TestLabelListEmptyContinue(t *testing.T) {
	// "[a]\n[b]\n  d\n" exercises the LABEL empty-continue + empty-break paths
	got := ToHTML("[a]\n[b]\n  desc\n")
	if !strings.Contains(got, "<dt>a</dt>") || !strings.Contains(got, "<dt>b</dt>") {
		t.Errorf("label continue: %q", got)
	}
}

func TestHighlightFloatAndHexNumbers(t *testing.T) {
	got, _ := HighlightRuby("a = 0xff + 1.5e3")
	if !strings.Contains(got, `ruby-value">0xff`) || !strings.Contains(got, `ruby-value">1.5e3`) {
		t.Errorf("numbers: %q", got)
	}
}

func TestAddRegexpHandlingExclusive(t *testing.T) {
	am := newAttributeManager()
	before := am.exclusiveBitmap
	am.addRegexpHandling(reHyperlink, attrUserBase<<16, true)
	if am.exclusiveBitmap == before {
		t.Error("exclusive bit not set")
	}
}

func TestNonUpdatingMatchingPair(t *testing.T) {
	// nested same-delimiter so the inner span is already attributed when the
	// outer match is retried, exercising the !updated restore branch.
	got := ToHTML("*a *b* c*\n")
	if got == "" {
		t.Fatal("empty")
	}
}

func TestNestedQualifiedInsideClass(t *testing.T) {
	// "class A::B" inside "class Outer" exercises parentFull != "".
	src := "class Outer\n  class A::B\n    def m; end\n  end\nend\n"
	top := Extract("t.rb", src)
	outer := top.ClassesAndModule[0]
	if outer.Name != "Outer" {
		t.Fatalf("outer: %+v", outer)
	}
	// Outer should contain module A, which contains class B
	a := outer.findNested("A")
	if a == nil || a.FullName != "Outer::A" {
		t.Fatalf("A: %+v", a)
	}
	if b := a.findNested("B"); b == nil || b.FullName != "Outer::A::B" {
		t.Errorf("B: %+v", b)
	}
}

func TestListItemStartInvalidType(t *testing.T) {
	h := NewToHtml(nil)
	if got := h.listItemStart(&ListItem{}, ListType(99)); got != "" {
		t.Errorf("invalid list type should be empty: %q", got)
	}
}

func TestRDocLabelPrefixStrip(t *testing.T) {
	h := NewToHtml(nil)
	if got := h.handleRDocLink("rdoc-label:label-foo"); !strings.Contains(got, "foo") {
		t.Errorf("label- prefix: %q", got)
	}
}

func TestFootnoteSup(t *testing.T) {
	// genURL wraps the link in <sup> when the id attribute names a footnote.
	h := NewToHtml(nil)
	got := h.genURL("rdoc-label:x:foottext-1", "1")
	if !strings.Contains(got, "<sup>") {
		t.Errorf("footnote sup: %q", got)
	}
}

func TestAnchorOnlyURL(t *testing.T) {
	scheme, u, id := parseURL("#frag")
	if scheme != "" || u != "#frag" || id != "" {
		t.Errorf("anchor url: %q %q %q", scheme, u, id)
	}
}

func TestTextMismatchedTT(t *testing.T) {
	// an unclosed <tt> is emitted as-is (mismatched-tag branch)
	got := textToHTML("<tt>unclosed")
	if !strings.Contains(got, "<tt>") {
		t.Errorf("mismatched tt: %q", got)
	}
}

func TestTextEscapedChar(t *testing.T) {
	// "\x" emits x (escaped-suppressed-crossref branch)
	got := textToHTML(`\x`)
	if got != "x" {
		t.Errorf("escaped char: %q", got)
	}
}

func TestTextQuotEntitySkip(t *testing.T) {
	got := textToHTML("&#39;hi&#39;")
	if !strings.Contains(got, entOpenSQuote) || !strings.Contains(got, entCloseSQuote) {
		t.Errorf("&#39; entity: %q", got)
	}
}

func TestTextApostropheAfterWord(t *testing.T) {
	// word' -> closing single quote (after_word branch)
	got := textToHTML("dogs' bones")
	if !strings.Contains(got, entCloseSQuote) {
		t.Errorf("apostrophe after word: %q", got)
	}
}

func TestTextBacktickAfterWord(t *testing.T) {
	// a backtick after a word stays literal
	got := textToHTML("word`")
	if !strings.Contains(got, "`") {
		t.Errorf("backtick after word: %q", got)
	}
}

func TestInlineCodeWithHashAndDot(t *testing.T) {
	// +#foo+ exercises the [#\\]? lead and +abc.+ a punctuation body char
	if got := ToHTML("a +#foo+ b\n"); got != "\n<p>a <code>#foo</code> b</p>\n" {
		t.Errorf("hash word: %q", got)
	}
	if got := ToHTML("a +abc.+ b\n"); got != "\n<p>a <code>abc.</code> b</p>\n" {
		t.Errorf("dot word: %q", got)
	}
}

func TestInlineCodeTrailingNonSpace(t *testing.T) {
	// "+abc!+" exercises the optional trailing \S before the closing delimiter
	got := ToHTML("a +abc!+ b\n")
	if got != "\n<p>a <code>abc!</code> b</p>\n" {
		t.Errorf("trailing nonspace: %q", got)
	}
}

func TestNonUpdatingPairRestore(t *testing.T) {
	// nested bold where the inner span is already attributed forces the
	// non-updating restore branch in the matching-pair scan.
	got := ToHTML("**bold**\n")
	if got == "" {
		t.Fatal("empty")
	}
}

func TestEmptyListItemModel(t *testing.T) {
	// a ListItem with no parts renders a blank line (model.accept empty branch)
	it := &ListItem{}
	f := NewToHtml(nil)
	f.startAccepting()
	f.list = []ListType{ListBullet}
	f.inListEntry = []string{""}
	it.accept(f)
	// the blank line for an empty item emits nothing; the closing </li> is
	// deferred (emitted lazily by the next item or list-end), so the visible
	// output is just the opening tag.
	if f.endAccepting() != "<li>" {
		t.Errorf("empty list item: %q", f.endAccepting())
	}
}

func TestMarkdownInlineLinkPassthrough(t *testing.T) {
	// inline regexp-handling fragments are emitted as their raw text in md/rdoc
	if got := ToMarkdownString("see http://x.com here\n"); !strings.Contains(got, "http://x.com") {
		t.Errorf("md link: %q", got)
	}
	if got := ToRdocString("see http://x.com here\n"); !strings.Contains(got, "http://x.com") {
		t.Errorf("rdoc link: %q", got)
	}
}

func TestMarkdownRdocVerbatimBlank(t *testing.T) {
	// a blank line inside a verbatim block (the empty-line skip branch)
	if got := ToMarkdownString("  a\n\n  b\n"); !strings.Contains(got, "    a") || !strings.Contains(got, "    b") {
		t.Errorf("md verb blank: %q", got)
	}
	if got := ToRdocString("  a\n\n  b\n"); !strings.Contains(got, "  a") || !strings.Contains(got, "  b") {
		t.Errorf("rdoc verb blank: %q", got)
	}
}

func TestHighlightUnterminatedMidBlock(t *testing.T) {
	// an unterminated string/regexp on a non-final line returns the bail path
	if _, ok := HighlightRuby("a = \"x\nb = 1"); ok {
		t.Error("unterminated string mid-block should fall back")
	}
	if _, ok := HighlightRuby("a =~ /x\nb = 1"); ok {
		t.Error("unterminated regexp mid-block should fall back")
	}
}
