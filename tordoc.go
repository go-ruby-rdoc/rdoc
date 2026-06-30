package rdoc

// ToRdoc renders the markup model back to RDoc markup text, mirroring
// RDoc::Markup::ToRdoc for the common block constructs: paragraphs are wrapped
// at 76 columns, headings use "= ", rules are a 78-dash line, and inline
// bold/em/tt map to <b>/<em>/<tt>.

import "strings"

// ToRdoc is an RDoc-markup output formatter.
type ToRdoc struct {
	res        []string
	listMarker []string
	listIndex  []int
}

// NewToRdoc creates an RDoc-markup formatter.
func NewToRdoc() *ToRdoc { return &ToRdoc{} }

// ToRdocString converts markup text through the model and back to RDoc markup
// (a normalisation round-trip).
func ToRdocString(markup string) string {
	doc := Parse(markup)
	f := NewToRdoc()
	doc.Accept(f)
	return f.endAccepting()
}

func (r *ToRdoc) startAccepting()     { r.res = nil }
func (r *ToRdoc) endAccepting() string { return strings.Join(r.res, "") }
func (r *ToRdoc) out(s string)         { r.res = append(r.res, s) }

func (r *ToRdoc) acceptParagraph(p *Paragraph) {
	r.out(wrapText(rdocInline(p.text()), 76) + "\n")
}

func (r *ToRdoc) acceptVerbatim(v *Verbatim) {
	for _, line := range strings.SplitAfter(strings.TrimRight(v.text(), "\n"), "\n") {
		if line == "" {
			continue
		}
		r.out("  " + line)
		if !strings.HasSuffix(line, "\n") {
			r.out("\n")
		}
	}
	r.out("\n")
}

func (r *ToRdoc) acceptRule(*Rule) { r.out(strings.Repeat("-", 78) + "\n") }

func (r *ToRdoc) acceptHeading(h *Heading) {
	level := h.Level
	if level > 6 {
		r.out(rdocInline(h.Text) + "\n")
		return
	}
	r.out(strings.Repeat("=", level) + " " + rdocInline(h.Text) + "\n")
}

func (r *ToRdoc) acceptBlankLine(*BlankLine) { r.out("\n") }

func (r *ToRdoc) acceptBlockQuote(b *BlockQuote) {
	inner := NewToRdoc()
	inner.startAccepting()
	for _, p := range b.Parts {
		p.accept(inner)
	}
	r.out(blockquotePrefix(inner.endAccepting()))
}

func (r *ToRdoc) acceptRaw(raw *Raw) { r.out(strings.Join(raw.Parts, "\n")) }

func (r *ToRdoc) acceptListStart(l *List) {
	switch l.Type {
	case ListBullet:
		r.listMarker = append(r.listMarker, "* ")
	case ListNumber:
		r.listMarker = append(r.listMarker, "1. ")
	case ListLalpha:
		r.listMarker = append(r.listMarker, "a. ")
	case ListUalpha:
		r.listMarker = append(r.listMarker, "A. ")
	case ListNote:
		r.listMarker = append(r.listMarker, "note")
	default:
		r.listMarker = append(r.listMarker, "label")
	}
	r.listIndex = append(r.listIndex, 0)
}

func (r *ToRdoc) acceptListEnd(l *List) {
	r.listMarker = r.listMarker[:len(r.listMarker)-1]
	r.listIndex = r.listIndex[:len(r.listIndex)-1]
	if l.Type == ListLabel || l.Type == ListNote {
		r.out("\n")
	}
}

func (r *ToRdoc) acceptListItemStart(it *ListItem) {
	marker := r.listMarker[len(r.listMarker)-1]
	switch marker {
	case "1. ":
		r.listIndex[len(r.listIndex)-1]++
		r.out(itoaN(r.listIndex[len(r.listIndex)-1]) + ". ")
	case "a. ", "A. ":
		r.listIndex[len(r.listIndex)-1]++
		r.out(alphaMarker(marker[0], r.listIndex[len(r.listIndex)-1]) + ". ")
	case "label":
		for _, lbl := range it.Label {
			r.out("[" + rdocInline(lbl) + "]\n")
		}
		r.out("  ")
	case "note":
		for _, lbl := range it.Label {
			r.out(rdocInline(lbl) + "::\n")
		}
		r.out("  ")
	default:
		r.out(marker)
	}
}

func (r *ToRdoc) acceptListItemEnd(*ListItem) {}

// alphaMarker returns the n-th (1-based) alphabetic ordinal starting at base
// ('a' or 'A'), wrapping a..z (RDoc does not go beyond single letters in
// practice for the corpus we target).
func alphaMarker(base byte, n int) string {
	return string(base + byte((n-1)%26))
}

// rdocInline maps the inline flow to RDoc HTML-style tags (<b>/<em>/<tt>).
func rdocInline(text string) string {
	am := newAttributeManager()
	flow := am.flow(text)
	var b strings.Builder
	for _, f := range flow {
		switch v := f.(type) {
		case flowString:
			b.WriteString(string(v))
		case regexpHandling:
			b.WriteString(v.text)
		case attrChanger:
			writeRdocTags(&b, v)
		}
	}
	return b.String()
}

func writeRdocTags(b *strings.Builder, c attrChanger) {
	if c.turnOff&attrBold != 0 {
		b.WriteString("</b>")
	}
	if c.turnOff&attrEM != 0 {
		b.WriteString("</em>")
	}
	if c.turnOff&attrTT != 0 {
		b.WriteString("</tt>")
	}
	if c.turnOn&attrBold != 0 {
		b.WriteString("<b>")
	}
	if c.turnOn&attrEM != 0 {
		b.WriteString("<em>")
	}
	if c.turnOn&attrTT != 0 {
		b.WriteString("<tt>")
	}
}

// wrapText wraps text at lineLen columns on word boundaries, mirroring
// RDoc::Text#wrap.
func wrapText(text string, lineLen int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > lineLen {
			lines = append(lines, cur)
			cur = w
		} else {
			cur += " " + w
		}
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n")
}

var _ Formatter = (*ToRdoc)(nil)
