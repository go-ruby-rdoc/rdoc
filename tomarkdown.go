package rdoc

// ToMarkdown renders the markup model to Markdown, mirroring
// RDoc::Markup::ToMarkdown for the common block constructs. Inline bold/em/tt
// map to **/*/` and headings to ATX (#). This focuses on the deterministic
// block structure; complex inline link handling falls back to the raw text and
// deeply nested lists are not re-indented (the gem's per-level prefix tracking
// is out of scope for this secondary formatter).

import "strings"

// ToMarkdown is a Markdown output formatter.
type ToMarkdown struct {
	res        []string
	listMarker []string
	listIndex  []int
}

// NewToMarkdown creates a Markdown formatter.
func NewToMarkdown() *ToMarkdown { return &ToMarkdown{} }

// ToMarkdownString converts markup text directly to Markdown.
func ToMarkdownString(markup string) string {
	doc := Parse(markup)
	f := NewToMarkdown()
	doc.Accept(f)
	return f.endAccepting()
}

func (m *ToMarkdown) startAccepting()    { m.res = nil }
func (m *ToMarkdown) endAccepting() string { return strings.Join(m.res, "") }
func (m *ToMarkdown) out(s string)        { m.res = append(m.res, s) }

func (m *ToMarkdown) acceptParagraph(p *Paragraph) {
	m.out(wrapText(markdownInline(p.text()), 76) + "\n")
}

func (m *ToMarkdown) acceptVerbatim(v *Verbatim) {
	for _, line := range strings.Split(strings.TrimRight(v.text(), "\n"), "\n") {
		if line == "" {
			m.out("\n")
		} else {
			m.out("    " + line + "\n")
		}
	}
	m.out("\n")
}

func (m *ToMarkdown) acceptRule(*Rule) { m.out("---\n") }

func (m *ToMarkdown) acceptHeading(h *Heading) {
	level := h.Level
	if level > 6 {
		// RDoc::Markup::ToMarkdown emits no ATX marker for levels above 6.
		m.out(markdownInline(h.Text) + "\n")
		return
	}
	m.out(strings.Repeat("#", level) + " " + markdownInline(h.Text) + "\n")
}

func (m *ToMarkdown) acceptBlankLine(*BlankLine) { m.out("\n") }

func (m *ToMarkdown) acceptBlockQuote(b *BlockQuote) {
	inner := NewToMarkdown()
	inner.startAccepting()
	for _, p := range b.Parts {
		p.accept(inner)
	}
	m.out(blockquotePrefix(inner.endAccepting()))
}

// blockquotePrefix prefixes each non-empty line of body with "> ", mirroring the
// Markdown/RDoc block-quote rendering.
func blockquotePrefix(body string) string {
	body = strings.TrimRight(body, "\n")
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if ln != "" {
			lines[i] = "> " + ln
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *ToMarkdown) acceptRaw(r *Raw) { m.out(strings.Join(r.Parts, "\n")) }

func (m *ToMarkdown) acceptListStart(l *List) {
	switch l.Type {
	case ListBullet:
		m.listMarker = append(m.listMarker, "*   ")
	case ListNumber, ListLalpha, ListUalpha:
		m.listMarker = append(m.listMarker, "1.  ")
	default:
		m.listMarker = append(m.listMarker, ":   ")
	}
	m.listIndex = append(m.listIndex, 0)
}

func (m *ToMarkdown) acceptListEnd(l *List) {
	m.listMarker = m.listMarker[:len(m.listMarker)-1]
	m.listIndex = m.listIndex[:len(m.listIndex)-1]
	if l.Type == ListLabel || l.Type == ListNote {
		m.out("\n")
	}
}

func (m *ToMarkdown) acceptListItemStart(it *ListItem) {
	marker := m.listMarker[len(m.listMarker)-1]
	switch {
	case strings.HasPrefix(marker, "1."):
		m.listIndex[len(m.listIndex)-1]++
		m.out(itoaN(m.listIndex[len(m.listIndex)-1]) + ".  ")
	case strings.HasPrefix(marker, ":"):
		for _, lbl := range it.Label {
			m.out(markdownInline(lbl) + "\n")
		}
		m.out(":   ")
	default:
		m.out(marker)
	}
}

// itoaN renders a non-negative int (list ordinal) without importing strconv.
func itoaN(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func (m *ToMarkdown) acceptListItemEnd(*ListItem) {}

// markdownInline maps the inline attribute flow to Markdown emphasis markers.
func markdownInline(text string) string {
	am := newAttributeManager()
	flow := am.flow(text)
	var b strings.Builder
	// The base attribute manager registers no regexp handlers, so the flow
	// contains only strings and attribute changes (links pass through as text).
	for _, f := range flow {
		switch v := f.(type) {
		case flowString:
			b.WriteString(string(v))
		case attrChanger:
			writeMarkdownTags(&b, v)
		}
	}
	return b.String()
}

func writeMarkdownTags(b *strings.Builder, c attrChanger) {
	// close then open, matching the formatter convention
	if c.turnOff&attrBold != 0 {
		b.WriteString("**")
	}
	if c.turnOff&attrEM != 0 {
		b.WriteString("*")
	}
	if c.turnOff&attrTT != 0 {
		b.WriteString("`")
	}
	if c.turnOn&attrBold != 0 {
		b.WriteString("**")
	}
	if c.turnOn&attrEM != 0 {
		b.WriteString("*")
	}
	if c.turnOn&attrTT != 0 {
		b.WriteString("`")
	}
}

var _ Formatter = (*ToMarkdown)(nil)
