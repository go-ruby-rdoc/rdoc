package rdoc

import "strings"

// This file defines the RDoc::Markup document model: the tree of block-level
// elements produced by the parser and consumed by the formatters. It mirrors
// the RDoc::Markup::* element classes.

// ListType identifies the kind of a List, mirroring RDoc::Markup::Parser's
// LIST_TOKENS.
type ListType int

const (
	// ListBullet is an unordered "* item" / "- item" list (:BULLET).
	ListBullet ListType = iota
	// ListLabel is a "[label] text" list (:LABEL).
	ListLabel
	// ListLalpha is a lower-alpha "a. item" ordered list (:LALPHA).
	ListLalpha
	// ListNote is a "label:: text" list (:NOTE).
	ListNote
	// ListNumber is a numeric "1. item" ordered list (:NUMBER).
	ListNumber
	// ListUalpha is an upper-alpha "A. item" ordered list (:UALPHA).
	ListUalpha
)

// String returns the RDoc symbol name for the list type (e.g. "BULLET").
func (t ListType) String() string {
	switch t {
	case ListBullet:
		return "BULLET"
	case ListLabel:
		return "LABEL"
	case ListLalpha:
		return "LALPHA"
	case ListNote:
		return "NOTE"
	case ListNumber:
		return "NUMBER"
	case ListUalpha:
		return "UALPHA"
	}
	return "BULLET"
}

// Element is the interface implemented by every node in the markup document
// model. accept dispatches to the visitor (a Formatter).
type Element interface {
	accept(v visitor)
}

// visitor is the internal interface every formatter implements. It mirrors the
// RDoc::Markup formatter visitor protocol (accept_* methods).
type visitor interface {
	acceptParagraph(*Paragraph)
	acceptVerbatim(*Verbatim)
	acceptRule(*Rule)
	acceptHeading(*Heading)
	acceptBlankLine(*BlankLine)
	acceptBlockQuote(*BlockQuote)
	acceptRaw(*Raw)
	acceptListStart(*List)
	acceptListEnd(*List)
	acceptListItemStart(*ListItem)
	acceptListItemEnd(*ListItem)
}

// Document is the root of the markup model, a sequence of block-level elements.
// It mirrors RDoc::Markup::Document.
type Document struct {
	Parts []Element
}

func (d *Document) push(e Element) { d.Parts = append(d.Parts, e) }

// Accept walks the document, invoking the formatter on each part. It is the
// public entry equivalent to RDoc::Markup::Document#accept.
func (d *Document) Accept(f Formatter) {
	f.startAccepting()
	for _, p := range d.Parts {
		p.accept(f)
	}
}

func (d *Document) accept(v visitor) {
	// A nested Document is flattened by walking its parts.
	for _, p := range d.Parts {
		p.accept(v)
	}
}

// Paragraph is a run of text. Its Parts are the raw text lines (joined with a
// space). Mirrors RDoc::Markup::Paragraph.
type Paragraph struct {
	Parts []string
}

func (p *Paragraph) push(s string) { p.Parts = append(p.Parts, s) }

// text joins the paragraph parts the way RDoc::Markup::Paragraph#text does:
// with the empty string. The inter-line spaces are baked into the parts by the
// parser (build_paragraph appends a trailing space when joining wrapped lines).
func (p *Paragraph) text() string {
	return strings.Join(p.Parts, "")
}

func (p *Paragraph) accept(v visitor) { v.acceptParagraph(p) }

// Verbatim is an indented literal/code block. Parts are the lines, each ending
// in a newline. Mirrors RDoc::Markup::Verbatim.
type Verbatim struct {
	Parts []string
	// format, when "ruby", forces Ruby syntax highlighting (set via a
	// :markup: ruby directive). The parser never sets this; it is exposed for
	// completeness and matches Verbatim#ruby?.
	format string
}

func (vb *Verbatim) push(s string) { vb.Parts = append(vb.Parts, s) }

func (vb *Verbatim) accept(v visitor) { v.acceptVerbatim(vb) }

// text returns the joined verbatim contents.
func (vb *Verbatim) text() string {
	out := ""
	for _, s := range vb.Parts {
		out += s
	}
	return out
}

// ruby reports whether this verbatim block is explicitly flagged as ruby.
func (vb *Verbatim) ruby() bool { return vb.format == "ruby" }

// normalize collapses runs of blank lines and trims trailing blank lines,
// mirroring RDoc::Markup::Verbatim#normalize.
func (vb *Verbatim) normalize() {
	parts := []string{}
	blanks := 0
	for _, p := range vb.Parts {
		if p == "\n" {
			blanks++
			continue
		}
		if blanks > 0 {
			parts = append(parts, "\n")
		}
		blanks = 0
		parts = append(parts, p)
	}
	vb.Parts = parts
}

// Rule is a horizontal rule (---). Weight mirrors the dash-count-minus-two
// stored by RDoc::Markup::Rule.
type Rule struct {
	Weight int
}

func (r *Rule) accept(v visitor) { v.acceptRule(r) }

// Heading is a section heading of a given Level (1-6 logically, clamped to 6
// when rendered). Mirrors RDoc::Markup::Heading.
type Heading struct {
	Level int
	Text  string
}

func (h *Heading) accept(v visitor) { v.acceptHeading(h) }

// aref returns the HTML-safe anchor reference for this heading
// ("label-" + ToLabel of the text), mirroring Heading#aref.
func (h *Heading) aref() string {
	return "label-" + toLabel(h.Text)
}

// BlankLine marks a blank line between blocks. Mirrors RDoc::Markup::BlankLine.
type BlankLine struct{}

func (b *BlankLine) accept(v visitor) { v.acceptBlankLine(b) }

// Raw is verbatim output passed through untouched. Mirrors RDoc::Markup::Raw.
type Raw struct {
	Parts []string
}

func (r *Raw) accept(v visitor) { v.acceptRaw(r) }

// BlockQuote groups indented quoted blocks. Mirrors RDoc::Markup::BlockQuote.
type BlockQuote struct {
	Parts []Element
}

func (b *BlockQuote) push(e Element) { b.Parts = append(b.Parts, e) }

func (b *BlockQuote) accept(v visitor) { v.acceptBlockQuote(b) }

// List is a bullet/number/label/note list. Mirrors RDoc::Markup::List.
type List struct {
	Type  ListType
	typed bool // whether Type has been assigned yet
	Items []*ListItem
}

func (l *List) push(it *ListItem) { l.Items = append(l.Items, it) }

func (l *List) empty() bool { return len(l.Items) == 0 }

func (l *List) accept(v visitor) {
	v.acceptListStart(l)
	for _, it := range l.Items {
		it.accept(v)
	}
	v.acceptListEnd(l)
}

// ListItem is one entry in a List. Label holds the label parts for LABEL/NOTE
// lists (nil for plain lists). Parts holds the nested block content. Mirrors
// RDoc::Markup::ListItem.
type ListItem struct {
	Label []string
	Parts []Element
}

func (it *ListItem) push(e Element) { it.Parts = append(it.Parts, e) }

func (it *ListItem) accept(v visitor) {
	v.acceptListItemStart(it)
	if len(it.Parts) == 0 {
		(&BlankLine{}).accept(v)
	}
	for _, p := range it.Parts {
		p.accept(v)
	}
	v.acceptListItemEnd(it)
}
