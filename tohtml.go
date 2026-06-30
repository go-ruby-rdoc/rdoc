package rdoc

// This file ports RDoc::Markup::ToHtml (and the relevant parts of
// RDoc::Markup::Formatter and RDoc::Text): rendering the document model to the
// HTML fragments the rdoc gem emits.

import (
	"regexp"
	"strings"
)

// Formatter is the public interface implemented by the output visitors
// (ToHtml, etc.). Document.Accept drives it.
type Formatter interface {
	visitor
	startAccepting()
	endAccepting() string
}

// HTMLOptions controls HTML rendering, mirroring the subset of RDoc::Options
// the ToHtml formatter consults.
type HTMLOptions struct {
	// OutputDecoration controls whether headings get id/anchor decoration
	// (RDoc::Options#output_decoration, default true).
	OutputDecoration bool
	// Pipe mirrors RDoc::Options#pipe (markdown-pipe mode): headings/verbatim
	// are emitted without anchors / highlighting.
	Pipe bool
	// Highlighter, if set, syntax-highlights a verbatim block that looks like
	// Ruby, returning (html, true). When nil or returning ok=false, verbatim is
	// emitted as a plain escaped <pre>. HighlightRuby is the built-in
	// implementation; the deterministic default leaves this nil so output is
	// a plain <pre> without needing a Ruby toolchain.
	Highlighter func(code string) (string, bool)
	// Parseable reports whether a verbatim block is valid Ruby and should be
	// highlighted. RDoc::Markup::ToHtml#parseable? evaluates the block with the
	// real Ruby parser, which cannot be reproduced without a Ruby toolchain, so
	// this is a host seam. When nil, verbatim blocks are never auto-detected as
	// Ruby (only blocks explicitly flagged via Verbatim format are highlighted).
	Parseable func(code string) bool
	// CrossRef, if set, resolves a bare name to a link target for
	// ToHtmlCrossref behaviour.
	CrossRef func(name string) (href string, ok bool)
}

// DefaultHTMLOptions returns options matching the rdoc gem defaults.
func DefaultHTMLOptions() *HTMLOptions {
	return &HTMLOptions{OutputDecoration: true}
}

// ToHtml renders an RDoc markup document model to HTML. It mirrors
// RDoc::Markup::ToHtml.
type ToHtml struct {
	opts        *HTMLOptions
	am          *attributeManager
	res         []string
	inListEntry []string
	list        []ListType
	crossref    bool

	// tidy-link accumulation state (for "{label}[url]" spanning fragments)
	tidyBuf    *strings.Builder
	tidyActive bool
}

// regexp-handler attribute bits for the HTML formatter, allocated above the
// word-pair/regexp marker bits.
const (
	bitHyperlink = attrUserBase << iota
	bitRDocLink
	bitTidyLink
	bitCrossRef
)

var (
	urlCharsClass     = `[A-Za-z0-9\-._~:/?#\[\]@!$&'()*+,;%=]`
	reHyperlink       = regexp.MustCompile(`(?:link:|https?:|mailto:|ftp:|irc:|www\.)` + urlCharsClass + `+\w`)
	reRDocLink        = regexp.MustCompile(`rdoc-[a-z]+:(?:[^\s\[\]]|\[\d+\])+`)
	reTidyLink = regexp.MustCompile(`(?:\{.*?\}|\b[^\s{}]*?)\[\S+?\]`)
	// reCrossRefDefault is a focused subset of RDoc's CROSSREF_REGEXP covering
	// the common references: "A::B.meth"/"A#meth", a bare class/constant name
	// "A::B::C" (requiring a trailing boundary so contractions like "can't" are
	// not flagged), and a stand-alone "#meth" / "::meth".
	reCrossRefDefault = regexp.MustCompile(
		`(` +
			`\\?(?:::)?[A-Z]\w*(?:::\w+)*[.#]\w+[!?=]?` +
			`|\\?(?:::)?[A-Z]\w*(?:::\w+)*` +
			`|\\?#\w+[!?=]?` +
			`|\\?::\w+[!?=]?` +
			`)(?:[@\s).?!,;<]|$)`)
)

// NewToHtml builds an HTML formatter with the given options (nil = defaults).
func NewToHtml(opts *HTMLOptions) *ToHtml {
	if opts == nil {
		opts = DefaultHTMLOptions()
	}
	am := newAttributeManager()
	am.addRegexpHandling(reHyperlink, bitHyperlink, false)
	am.addRegexpHandling(reRDocLink, bitRDocLink, false)
	am.addRegexpHandling(reTidyLink, bitTidyLink, false)
	h := &ToHtml{opts: opts, am: am}
	if opts.CrossRef != nil {
		am.addRegexpHandling(reCrossRefDefault, bitCrossRef, false)
		h.crossref = true
	}
	return h
}

// ToHTML is a convenience: parse markup text and render to HTML with default
// options, equivalent to the gem's markup -> ToHtml round trip.
func ToHTML(markup string) string {
	doc := Parse(markup)
	f := NewToHtml(nil)
	doc.Accept(f)
	return f.endAccepting()
}

func (h *ToHtml) startAccepting() {
	h.res = nil
	h.inListEntry = nil
	h.list = nil
}

func (h *ToHtml) endAccepting() string { return strings.Join(h.res, "") }

func (h *ToHtml) out(s string) { h.res = append(h.res, s) }

// --- visitor methods ---

func (h *ToHtml) acceptParagraph(p *Paragraph) {
	h.out("\n<p>")
	text := p.text()
	// join hard-wrapped lines that the parser kept separate is already handled;
	// the gem additionally collapses single newlines between letters. Our
	// paragraph text has no embedded newlines, so emit directly.
	h.out(h.toHTML(text))
	h.out("</p>\n")
}

func (h *ToHtml) acceptVerbatim(v *Verbatim) {
	text := strings.TrimRight(v.text(), " \t\r\n")
	klass := ""
	var content string
	if (v.ruby() || h.parseable(text)) && h.opts.Highlighter != nil {
		if hl, ok := h.opts.Highlighter(text); ok { //nolint:nestif
			klass = ` class="ruby"`
			if !strings.HasSuffix(hl, "\n") {
				hl += "\n"
			}
			content = hl
		} else {
			content = escapeHTML(text)
		}
	} else {
		content = escapeHTML(text)
	}
	if h.opts.Pipe {
		h.out("\n<pre><code>" + escapeHTML(text) + "\n</code></pre>\n")
	} else {
		h.out("\n<pre" + klass + ">" + content + "</pre>\n")
	}
}

func (h *ToHtml) acceptRule(r *Rule) { h.out("<hr>\n") }

func (h *ToHtml) acceptHeading(head *Heading) {
	level := head.Level
	if level > 6 {
		level = 6
	}
	label := head.aref()
	ls := itoa(level)
	if h.opts.OutputDecoration {
		h.out("\n<h" + ls + ` id="` + label + `">`)
	} else {
		h.out("\n<h" + ls + ">")
	}
	if h.opts.Pipe {
		h.out(h.toHTML(head.Text))
	} else {
		h.out(`<a href="#` + label + `">` + h.toHTML(head.Text) + "</a>")
	}
	h.out("</h" + ls + ">\n")
}

func (h *ToHtml) acceptBlankLine(*BlankLine) {}

func (h *ToHtml) acceptRaw(r *Raw) { h.out(strings.Join(r.Parts, "\n")) }

func (h *ToHtml) acceptBlockQuote(b *BlockQuote) {
	h.out("\n<blockquote>")
	for _, p := range b.Parts {
		p.accept(h)
	}
	h.out("</blockquote>\n")
}

func (h *ToHtml) acceptListStart(l *List) {
	h.list = append(h.list, l.Type)
	h.out(htmlListName(l.Type, true))
	h.inListEntry = append(h.inListEntry, "")
}

func (h *ToHtml) acceptListEnd(l *List) {
	h.list = h.list[:len(h.list)-1]
	tag := h.inListEntry[len(h.inListEntry)-1]
	h.inListEntry = h.inListEntry[:len(h.inListEntry)-1]
	if tag != "" {
		h.out(tag)
	}
	h.out(htmlListName(l.Type, false) + "\n")
}

func (h *ToHtml) acceptListItemStart(it *ListItem) {
	if tag := h.inListEntry[len(h.inListEntry)-1]; tag != "" {
		h.out(tag)
	}
	h.out(h.listItemStart(it, h.list[len(h.list)-1]))
}

func (h *ToHtml) acceptListItemEnd(it *ListItem) {
	h.inListEntry[len(h.inListEntry)-1] = listEndFor(h.list[len(h.list)-1])
}

// --- utilities ---

func htmlListName(t ListType, open bool) string {
	var pair [2]string
	switch t {
	case ListBullet:
		pair = [2]string{"<ul>", "</ul>"}
	case ListLabel:
		pair = [2]string{`<dl class="rdoc-list label-list">`, "</dl>"}
	case ListLalpha:
		pair = [2]string{`<ol style="list-style-type: lower-alpha">`, "</ol>"}
	case ListNote:
		pair = [2]string{`<dl class="rdoc-list note-list">`, "</dl>"}
	case ListNumber:
		pair = [2]string{"<ol>", "</ol>"}
	case ListUalpha:
		pair = [2]string{`<ol style="list-style-type: upper-alpha">`, "</ol>"}
	}
	if open {
		return pair[0]
	}
	return pair[1]
}

func (h *ToHtml) listItemStart(it *ListItem, t ListType) string {
	switch t {
	case ListBullet, ListLalpha, ListNumber, ListUalpha:
		return "<li>"
	case ListLabel, ListNote:
		var b strings.Builder
		for _, lbl := range it.Label {
			b.WriteString("<dt>" + h.toHTML(lbl) + "</dt>\n")
		}
		b.WriteString("<dd>")
		return b.String()
	}
	return ""
}

func listEndFor(t ListType) string {
	switch t {
	case ListLabel, ListNote:
		return "</dd>"
	default:
		return "</li>"
	}
}

// itoa renders a small non-negative int (heading levels 1..6).
func itoa(n int) string { return string(rune('0' + n)) }
