package rdoc

// This file ports RDoc::Markup::Parser: the tokenizer turns markup text into a
// stream of tokens; the recursive-descent parser turns that stream into a
// Document. The structure deliberately mirrors the Ruby source so behaviour
// matches the gem.

import (
	"regexp"
	"strings"
)

// token kinds, mirroring the symbols the Ruby tokenizer emits.
type tokKind int

const (
	tkNewline tokKind = iota
	tkHeader
	tkRule
	tkBullet
	tkLalpha
	tkUalpha
	tkNumber
	tkLabel
	tkNote
	tkBlockquote
	tkText
	tkBreak
)

func (k tokKind) isList() bool {
	switch k {
	case tkBullet, tkLabel, tkLalpha, tkNote, tkNumber, tkUalpha:
		return true
	}
	return false
}

func (k tokKind) listType() ListType {
	switch k {
	case tkBullet:
		return ListBullet
	case tkLabel:
		return ListLabel
	case tkLalpha:
		return ListLalpha
	case tkNote:
		return ListNote
	case tkNumber:
		return ListNumber
	case tkUalpha:
		return ListUalpha
	}
	return ListBullet
}

// token carries a kind, its string data, an integer payload (header level /
// rule weight) and the column at which it begins.
type token struct {
	kind   tokKind
	str    string
	num    int
	column int
}

// parser holds the token stream and the cursor used by the recursive-descent
// parse. It mirrors RDoc::Markup::Parser.
type parser struct {
	tokens  []token
	current token
	hasCur  bool
}

var (
	reLeadingSpaces = regexp.MustCompile(`^ +`)
	reNewline       = regexp.MustCompile(`^\r?\n`)
	reHeader        = regexp.MustCompile(`^(=+)(\s*)`)
	reHeaderNL      = regexp.MustCompile(`^\r?\n`)
	reRule          = regexp.MustCompile(`^(-{3,}) *\r?(?:\n|$)`)
	reBullet        = regexp.MustCompile(`^([*-]) +(\S)`)
	reAlphaNum      = regexp.MustCompile(`^([a-zA-Z]|\d+)\. +(\S)`)
	reLabel         = regexp.MustCompile(`^\[(.*?)\]( +|\r?(?:\n|$))`)
	reNote          = regexp.MustCompile(`^(.*?)::( +|\r?(?:\n|$))`)
	reBlockquote    = regexp.MustCompile(`^>>> *(\w+)?(?:\r?\n|$)`)
	reText          = regexp.MustCompile(`^(.*?)(  )?\r?(?:\n|$)`)
	reLowercase     = regexp.MustCompile(`[a-z]`)
	reUppercase     = regexp.MustCompile(`[A-Z]`)
)

// reSpaceSepLetterEnd matches the SPACE_SEPARATED_LETTER_CLASS at end of a
// paragraph line (used to decide whether to join with a space).
var reSpaceSepLetterEnd = regexp.MustCompile(`(?:[\p{Nd}\p{L}\p{M}\p{Pc}]|[!-~])$`)

// Parse tokenizes and parses RDoc markup text into a Document. It is the public
// equivalent of RDoc::Markup::Parser.parse.
func Parse(text string) *Document {
	p := &parser{}
	p.tokenize(text)
	doc := &Document{}
	p.parse(doc, 0)
	return doc
}

// tokenize converts input into the token stream, mirroring Parser#tokenize.
func (p *parser) tokenize(input string) {
	// Work line by line tracking column. The Ruby scanner advances a column
	// counter; we reproduce that by walking the string with a byte cursor and
	// a per-line column counter.
	s := input
	column := 0

	emit := func(t token) { p.tokens = append(p.tokens, t) }

	for len(s) > 0 {
		// leading spaces -> column shift, no token
		if m := reLeadingSpaces.FindString(s); m != "" {
			column += len(m)
			s = s[len(m):]
			continue
		}

		pos := column

		// NEWLINE
		if m := reNewline.FindString(s); m != "" {
			emit(token{kind: tkNewline, str: m, column: pos})
			s = s[len(m):]
			column = 0
			continue
		}

		// HEADER (then optional TEXT). Ruby: /(=+)(\s*)/ where the whitespace
		// group may include a newline.
		if m := reHeader.FindStringSubmatch(s); m != nil {
			level := len(m[1])
			// If the whitespace group begins with a newline (or is empty and a
			// newline follows the '='), the header has no inline text: consume
			// only the '=' run and leave the newline for the NEWLINE token.
			if m[2] != "" && reHeaderNL.MatchString(m[2]) {
				emit(token{kind: tkHeader, num: level, column: pos})
				column += len(m[1])
				s = s[len(m[1]):]
				continue
			}
			// header with text: consume "=+\s*" (the whitespace may span a
			// newline) then the rest of that line (.*) as the heading text.
			emit(token{kind: tkHeader, num: level, column: pos})
			consumed := len(m[0])
			// recompute the column after consuming whitespace that may contain
			// newlines, so the text token's column is correct.
			if nl := strings.LastIndexByte(m[0], '\n'); nl >= 0 {
				column = len(m[0]) - nl - 1
			} else {
				column += consumed
			}
			s = s[consumed:]
			textPos := column
			line := lineRest(s) // up to but not including the newline
			txt := strings.TrimSuffix(line, "\r")
			emit(token{kind: tkText, str: txt, column: textPos})
			column += len(line)
			s = s[len(line):]
			continue
		}

		// RULE
		if m := reRule.FindStringSubmatch(s); m != nil {
			emit(token{kind: tkRule, num: len(m[1]) - 2, column: pos})
			// consume up to (not including) the newline so the NEWLINE token is
			// emitted next, matching the Ruby `*\r?$` semantics.
			adv := strings.Index(m[0], "\n")
			if adv < 0 {
				adv = len(m[0])
			}
			column += adv
			s = s[adv:]
			continue
		}

		// BULLET
		if m := reBullet.FindStringSubmatch(s); m != nil {
			emit(token{kind: tkBullet, str: m[1], column: pos})
			// consume marker + spaces but unscan the captured non-space char
			consumed := len(m[0]) - len(m[2])
			column += consumed
			s = s[consumed:]
			continue
		}

		// LALPHA / UALPHA / NUMBER
		if m := reAlphaNum.FindStringSubmatch(s); m != nil {
			label := m[1]
			var k tokKind
			switch {
			case reLowercase.MatchString(label):
				k = tkLalpha
			case reUppercase.MatchString(label):
				k = tkUalpha
			default:
				k = tkNumber
			}
			emit(token{kind: k, str: label, column: pos})
			consumed := len(m[0]) - len(m[2])
			column += consumed
			s = s[consumed:]
			continue
		}

		// LABEL
		if m := reLabel.FindStringSubmatch(s); m != nil {
			emit(token{kind: tkLabel, str: m[1], column: pos})
			consumed := labelConsumed(m[0])
			column += consumed
			s = s[consumed:]
			continue
		}

		// NOTE
		if m := reNote.FindStringSubmatch(s); m != nil {
			emit(token{kind: tkNote, str: m[1], column: pos})
			consumed := labelConsumed(m[0])
			column += consumed
			s = s[consumed:]
			continue
		}

		// BLOCKQUOTE
		if m := reBlockquote.FindStringSubmatch(s); m != nil {
			word := m[1]
			emit(token{kind: tkBlockquote, str: word, column: pos})
			// advance past ">>> ", leaving the word (unscanned) and the
			// trailing newline as their own tokens.
			adv := blockquoteConsumed(m[0], word)
			column += adv
			s = s[adv:]
			continue
		}

		// TEXT (with optional BREAK on a double-space line ending)
		m := reText.FindStringSubmatch(s)
		full := m[0]
		txt := m[1]
		brk := m[2]
		emit(token{kind: tkText, str: txt, column: pos})
		if brk != "" {
			emit(token{kind: tkBreak, str: brk, column: pos + len(txt)})
		}
		// advance up to (not including) the trailing newline
		adv := len(full)
		if strings.HasSuffix(full, "\n") {
			if strings.HasSuffix(full, "\r\n") {
				adv -= 2
			} else {
				adv--
			}
		}
		column += adv
		s = s[adv:]
	}
}

// lineRest returns s up to but not including the next newline (or all of s),
// mirroring StringScanner#scan(/.*/).
func lineRest(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// labelConsumed returns how much of a LABEL/NOTE match to consume: the whole
// match, minus a trailing newline (which must remain as its own NEWLINE token).
func labelConsumed(full string) int {
	adv := len(full)
	if strings.HasSuffix(full, "\r\n") {
		adv -= 2
	} else if strings.HasSuffix(full, "\n") {
		adv--
	}
	return adv
}

// blockquoteConsumed computes how many bytes of a blockquote match to consume,
// leaving the optional word and any trailing newline unconsumed.
func blockquoteConsumed(full, word string) int {
	adv := len(full)
	if strings.HasSuffix(full, "\r\n") {
		adv -= 2
	} else if strings.HasSuffix(full, "\n") {
		adv--
	}
	if word != "" {
		adv -= len(word)
	}
	return adv
}

// get pulls the next token, mirroring Parser#get.
func (p *parser) get() (token, bool) {
	if len(p.tokens) == 0 {
		p.hasCur = false
		return token{}, false
	}
	p.current = p.tokens[0]
	p.tokens = p.tokens[1:]
	p.hasCur = true
	return p.current, true
}

// unget returns the current token to the stream, mirroring Parser#unget.
func (p *parser) unget() {
	if p.hasCur {
		p.tokens = append([]token{p.current}, p.tokens...)
		p.hasCur = false
	}
}

// peek returns the next token without consuming it, mirroring Parser#peek_token.
func (p *parser) peek() (token, bool) {
	if len(p.tokens) == 0 {
		return token{}, false
	}
	return p.tokens[0], true
}

// skip consumes the next token if it matches kind, mirroring Parser#skip
// (non-raising form: callers here never need the error variant).
func (p *parser) skip(kind tokKind) bool {
	t, ok := p.get()
	if !ok {
		return false
	}
	if t.kind == kind {
		return true
	}
	p.unget()
	return false
}

// parse fills parent with elements until end-of-stream or a token in a column
// less than indent. Mirrors Parser#parse.
func (p *parser) parse(parent container, indent int) {
	for {
		t, ok := p.get()
		if !ok {
			break
		}

		switch t.kind {
		case tkBreak:
			parent.push(&BlankLine{})
			p.skipNewlines()
			continue
		case tkNewline:
			parent.push(&BlankLine{})
			p.skipNewlines()
			continue
		}

		if t.column < indent {
			p.unget()
			return
		} else if t.column > indent {
			p.unget()
			parent.push(p.buildVerbatim(indent))
			continue
		}

		switch {
		case t.kind == tkHeader:
			parent.push(p.buildHeading(t.num))
		case t.kind == tkRule:
			parent.push(&Rule{Weight: t.num})
			p.skip(tkNewline)
		case t.kind == tkText:
			p.unget()
			parent.push(p.buildParagraph(indent))
		case t.kind == tkBlockquote:
			// consume the rest of the blockquote intro line
			for {
				tt, ok := p.get()
				if !ok || tt.kind == tkNewline {
					break
				}
			}
			col := 0
			if pt, ok := p.peek(); ok {
				col = pt.column
			}
			bq := &BlockQuote{}
			p.parse(bq, col)
			parent.push(bq)
		case t.kind.isList():
			p.unget()
			parent.push(p.buildList(indent))
		}
	}
}

// skipNewlines skips a run of NEWLINE tokens (the `skip :NEWLINE, false`
// idiom used after a blank line).
func (p *parser) skipNewlines() {
	p.skip(tkNewline)
}

// buildHeading builds a Heading, mirroring Parser#build_heading.
func (p *parser) buildHeading(level int) *Heading {
	t, ok := p.get()
	if ok && t.kind == tkText {
		p.skip(tkNewline)
		return &Heading{Level: level, Text: t.str}
	}
	if ok {
		p.unget()
	}
	return &Heading{Level: level, Text: ""}
}

// buildParagraph builds a Paragraph flush to margin, mirroring
// Parser#build_paragraph.
func (p *parser) buildParagraph(margin int) *Paragraph {
	para := &Paragraph{}
	for {
		t, ok := p.get()
		if !ok {
			break
		}
		if t.kind == tkText && t.column == margin {
			data := t.str
			para.push(data)
			if pt, ok := p.peek(); ok && pt.kind == tkBreak {
				break
			}
			if p.skip(tkNewline) && reSpaceSepLetterEnd.MatchString(data) {
				// append a space to the just-pushed part
				para.Parts[len(para.Parts)-1] += " "
			}
		} else {
			p.unget()
			break
		}
	}
	if n := len(para.Parts); n > 0 {
		para.Parts[n-1] = strings.TrimSuffix(para.Parts[n-1], " ")
	}
	return para
}

// buildVerbatim builds a Verbatim indented from margin, mirroring
// Parser#build_verbatim.
func (p *parser) buildVerbatim(margin int) *Verbatim {
	vb := &Verbatim{}
	minIndent := -1
	genLeading := true
	line := ""

	for {
		t, ok := p.get()
		if !ok {
			break
		}
		if t.kind == tkNewline {
			line += t.str
			vb.push(line)
			line = ""
			genLeading = true
			continue
		}
		if t.column <= margin {
			p.unget()
			break
		}
		if genLeading {
			indent := t.column - margin
			line += strings.Repeat(" ", indent)
			if minIndent < 0 || indent < minIndent {
				minIndent = indent
			}
			genLeading = false
		}

		switch {
		case t.kind == tkHeader:
			line += strings.Repeat("=", t.num)
			peekCol := t.column + t.num
			if pt, ok := p.peek(); ok {
				peekCol = pt.column
			}
			indent := peekCol - t.column - t.num
			if indent > 0 {
				line += strings.Repeat(" ", indent)
			}
		case t.kind == tkRule:
			width := 2 + t.num
			line += strings.Repeat("-", width)
			peekCol := t.column + width
			if pt, ok := p.peek(); ok {
				peekCol = pt.column
			}
			indent := peekCol - t.column - width
			if indent > 0 {
				line += strings.Repeat(" ", indent)
			}
		case t.kind == tkBreak || t.kind == tkText:
			line += t.str
		case t.kind == tkBlockquote:
			line += ">>>"
			if pt, ok := p.peek(); ok && pt.kind != tkNewline {
				ind := pt.column - t.column - 3
				if ind > 0 {
					line += strings.Repeat(" ", ind)
				}
			}
		default: // list tokens
			marker := listMarker(t)
			line += marker
			if pt, ok := p.peek(); ok && pt.kind != tkNewline {
				indent := pt.column - t.column - len(marker)
				if indent > 0 {
					line += strings.Repeat(" ", indent)
				}
			}
		}
	}

	if line != "" {
		vb.push(line + "\n")
	}
	if minIndent > 0 {
		for i, part := range vb.Parts {
			if part != "\n" {
				if len(part) >= minIndent {
					vb.Parts[i] = part[minIndent:]
				} else {
					vb.Parts[i] = ""
				}
			}
		}
	}
	vb.normalize()
	return vb
}

// listMarker reconstructs the literal list marker text for a verbatim line,
// mirroring the list_marker switch in build_verbatim.
func listMarker(t token) string {
	switch t.kind {
	case tkBullet:
		return t.str
	case tkLabel:
		return "[" + t.str + "]"
	case tkNote:
		return t.str + "::"
	default: // LALPHA, NUMBER, UALPHA
		return t.str + "."
	}
}

// buildList builds a List flush to margin, mirroring Parser#build_list.
func (p *parser) buildList(margin int) *List {
	list := &List{}
	var label []string

	for len(p.tokens) > 0 {
		t, ok := p.get()
		if !ok {
			break
		}

		if !t.kind.isList() {
			p.unget()
			break
		}

		lt := t.kind.listType()
		if t.column < margin || (list.typed && list.Type != lt) {
			p.unget()
			break
		}

		list.Type = lt
		list.typed = true

		data := t.str
		var itemLabel []string

		// mirror `peek_type, _, column, = peek_token`: the item's content
		// margin is the column of the token after the list marker.
		peekType, _, column := p.peekInfo()
		if !p.hasPeek() {
			column = t.column
		}

		if t.kind == tkNote || t.kind == tkLabel {
			if label == nil {
				label = []string{}
			}
			if peekType == tkNewline && p.hasPeek() {
				// description not on the same line
				for {
					pt, hasPt := p.peek()
					if !hasPt || pt.kind != tkNewline {
						break
					}
					p.get()
					peekType, _, column = p.peekInfo()
				}
				var empty int // 0=false,1=true,2=continue
				if !p.hasPeek() || column < margin {
					empty = 1
				} else if column == margin {
					switch {
					case peekType == t.kind:
						empty = 2
					case peekType.isList():
						empty = 1
					default:
						empty = 0
					}
				} else {
					empty = 0
				}
				if empty != 0 {
					label = append(label, data)
					if empty == 2 {
						continue
					}
					break
				}
			}
		} else {
			data = ""
		}

		if label != nil {
			itemLabel = append(label, data)
			label = nil
		}

		item := &ListItem{Label: itemLabel}
		p.parse(item, column)
		list.push(item)
	}

	if list.empty() {
		if label == nil {
			return list
		}
		if list.Type != ListLabel && list.Type != ListNote {
			return list
		}
		item := &ListItem{Label: label}
		item.push(&BlankLine{})
		list.push(item)
	}

	return list
}

// peekInfo returns the kind, str and column of the next token (zero values if
// none), mirroring `peek_type, _, column, = peek_token`.
func (p *parser) peekInfo() (tokKind, string, int) {
	if len(p.tokens) == 0 {
		return tkNewline, "", 0
	}
	t := p.tokens[0]
	return t.kind, t.str, t.column
}

func (p *parser) hasPeek() bool { return len(p.tokens) > 0 }

// container is the interface for things the parser appends elements to:
// Document, BlockQuote and ListItem all implement push(Element).
type container interface {
	push(Element)
}

var (
	_ container = (*Document)(nil)
	_ container = (*BlockQuote)(nil)
	_ container = (*ListItem)(nil)
)
