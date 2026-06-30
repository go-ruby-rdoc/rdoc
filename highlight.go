package rdoc

// This file implements a focused Ruby syntax highlighter that reproduces the
// HTML RDoc emits for verbatim code blocks via RDoc::Parser::RipperStateLex +
// RDoc::TokenStream.to_html, for the common token classes (identifiers,
// keywords, constants, instance/class/global variables, numbers, symbols,
// strings, regexps, comments and operators).
//
// It is wired in as the default HTMLOptions.Highlighter by HighlightRuby. The
// full RipperStateLex covers more exotic Ruby (heredocs, complex string
// interpolation -> ruby-node, %-literals); those fall back to a plain escaped
// block via the (code, false) return so the deterministic core stays faithful.

import (
	"strings"
)

// rubyKeywords is the set RDoc's lexer tags as ruby-keyword.
var rubyKeywords = map[string]bool{
	"__ENCODING__": true, "__LINE__": true, "__FILE__": true, "BEGIN": true, "END": true,
	"alias": true, "and": true, "begin": true, "break": true, "case": true, "class": true,
	"def": true, "defined?": true, "do": true, "else": true, "elsif": true, "end": true,
	"ensure": true, "false": true, "for": true, "if": true, "in": true, "module": true,
	"next": true, "nil": true, "not": true, "or": true, "redo": true, "rescue": true,
	"retry": true, "return": true, "self": true, "super": true, "then": true, "true": true,
	"undef": true, "unless": true, "until": true, "when": true, "while": true, "yield": true,
}

// HighlightRuby highlights Ruby source the way RDoc does, returning the HTML and
// true on success. For constructs this focused highlighter does not model
// faithfully it returns ("", false) so the caller falls back to a plain block.
func HighlightRuby(code string) (string, bool) {
	hl := &rubyHL{src: code}
	out, ok := hl.run()
	if !ok {
		return "", false
	}
	return out, true
}

type rubyHL struct {
	src string
	pos int
	out strings.Builder
	ok  bool
}

func (h *rubyHL) run() (string, bool) {
	h.ok = true
	// afterDef tracks that the previous significant token was `def`, so the
	// next identifier becomes a method title (ruby-identifier ruby-title).
	afterDef := false
	for h.pos < len(h.src) {
		c := h.src[h.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			h.out.WriteByte(c)
			h.pos++
		case c == '#':
			h.span("ruby-comment", h.scanComment())
		case c == '"' || c == '\'':
			str, isNode, ok := h.scanString(c)
			if !ok {
				return "", false
			}
			if isNode {
				h.span("ruby-node", str)
			} else {
				h.span("ruby-string", str)
			}
		case c == '@' && !(h.pos+1 < len(h.src) && h.src[h.pos+1] == '@'):
			// a single @ivar is ruby-ivar; @@class vars and $globals are
			// emitted as plain identifiers by RDoc's lexer.
			h.span("ruby-ivar", h.scanVar())
		case c == '@' || c == '$':
			h.span("ruby-identifier", h.scanVar())
		case c == ':' && h.isSymbolStart():
			h.span("ruby-value", h.scanSymbol())
		case isDigit(c):
			h.span("ruby-value", h.scanNumber())
		case c == '/' && h.regexpAllowed():
			rx, ok := h.scanRegexp()
			if !ok {
				return "", false
			}
			h.span("ruby-regexp", rx)
		case isIdentStart(c):
			word := h.scanIdent()
			switch {
			case h.isLabel(word):
				// hash key "a:" -> ruby-value including the colon
				h.span("ruby-value", word+":")
				h.pos++ // consume ':'
			case rubyKeywords[word]:
				h.span("ruby-keyword", word)
			case afterDef:
				h.span("ruby-identifier ruby-title", word)
			case isConstantName(word):
				h.span("ruby-constant", word)
			default:
				h.span("ruby-identifier", word)
			}
			afterDef = word == "def"
			continue
		case isOperatorChar(c):
			op := h.scanOperator()
			if op == "" {
				// punctuation that RDoc does not wrap (= ( ) , [ ] { } ; etc.)
				h.out.WriteString(escapeHTML(string(c)))
				h.pos++
			} else {
				h.span("ruby-operator", op)
			}
		default:
			// any other punctuation passes through escaped and unwrapped
			h.out.WriteString(escapeHTML(string(c)))
			h.pos++
		}
		if !h.ok {
			return "", false
		}
		afterDef = afterDef && (c == ' ' || c == '\t')
	}
	return h.out.String(), true
}

func (h *rubyHL) span(class, text string) {
	h.out.WriteString(`<span class="` + class + `">` + escapeHTML(text) + "</span>")
}

func (h *rubyHL) scanComment() string {
	start := h.pos
	for h.pos < len(h.src) && h.src[h.pos] != '\n' {
		h.pos++
	}
	return h.src[start:h.pos]
}

func (h *rubyHL) scanVar() string {
	start := h.pos
	h.pos++ // @ or $
	if h.pos < len(h.src) && h.src[h.pos] == '@' {
		h.pos++ // @@class var
	}
	for h.pos < len(h.src) && isIdentPart(h.src[h.pos]) {
		h.pos++
	}
	return h.src[start:h.pos]
}

func (h *rubyHL) scanSymbol() string {
	start := h.pos
	h.pos++ // ':'
	for h.pos < len(h.src) && isIdentPart(h.src[h.pos]) {
		h.pos++
	}
	return h.src[start:h.pos]
}

func (h *rubyHL) scanNumber() string {
	start := h.pos
	for h.pos < len(h.src) {
		c := h.src[h.pos]
		if isDigit(c) || c == '.' || c == '_' || c == 'e' || c == 'E' ||
			c == 'x' || c == 'b' || c == 'o' || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') ||
			((c == '+' || c == '-') && h.pos > start && (h.src[h.pos-1] == 'e' || h.src[h.pos-1] == 'E')) {
			h.pos++
			continue
		}
		break
	}
	return h.src[start:h.pos]
}

func (h *rubyHL) scanString(q byte) (string, bool, bool) {
	start := h.pos
	h.pos++ // opening quote
	isNode := false
	for h.pos < len(h.src) {
		c := h.src[h.pos]
		if c == '\\' {
			h.pos += 2
			continue
		}
		if q == '"' && c == '#' && h.pos+1 < len(h.src) && h.src[h.pos+1] == '{' {
			isNode = true
		}
		if c == q {
			h.pos++
			return h.src[start:h.pos], isNode, true
		}
		if c == '\n' {
			// unterminated within block: bail to plain rendering
			return "", false, false
		}
		h.pos++
	}
	return "", false, false
}

func (h *rubyHL) scanRegexp() (string, bool) {
	start := h.pos
	h.pos++ // opening /
	for h.pos < len(h.src) {
		c := h.src[h.pos]
		if c == '\\' {
			h.pos += 2
			continue
		}
		if c == '/' {
			h.pos++
			// flags
			for h.pos < len(h.src) && isIdentPart(h.src[h.pos]) {
				h.pos++
			}
			return h.src[start:h.pos], true
		}
		if c == '\n' {
			return "", false
		}
		h.pos++
	}
	return "", false
}

func (h *rubyHL) scanIdent() string {
	start := h.pos
	for h.pos < len(h.src) && isIdentPart(h.src[h.pos]) {
		h.pos++
	}
	// trailing ? or ! (method names / defined?)
	if h.pos < len(h.src) && (h.src[h.pos] == '?' || h.src[h.pos] == '!') {
		h.pos++
	}
	return h.src[start:h.pos]
}

// operatorChars are the bytes that can begin a wrapped operator token.
func isOperatorChar(c byte) bool {
	switch c {
	case '+', '-', '*', '/', '%', '=', '<', '>', '!', '&', '|', '^', '~':
		return true
	}
	return false
}

// scanOperator returns the longest operator at the cursor that RDoc wraps, or
// "" if the leading char is punctuation RDoc leaves unwrapped.
func (h *rubyHL) scanOperator() string {
	rest := h.src[h.pos:]
	for _, op := range rubyOperators {
		if strings.HasPrefix(rest, op) {
			h.pos += len(op)
			return op
		}
	}
	return ""
}

// rubyOperators is ordered longest-first so multi-char operators win.
var rubyOperators = []string{
	"<=>", "===", "**=", "<<=", ">>=", "&&=", "||=", "...",
	"==", "!=", ">=", "<=", "=~", "!~", "&&", "||", "**", "<<", ">>",
	"+=", "-=", "*=", "/=", "%=", "|=", "&=", "^=", "..",
	"+", "-", "*", "/", "%", "<", ">", "!", "&", "|", "^", "~",
}

func (h *rubyHL) isSymbolStart() bool {
	// ':' followed by an identifier char, and not '::'
	if h.pos+1 >= len(h.src) {
		return false
	}
	if h.src[h.pos+1] == ':' {
		return false
	}
	return isIdentStart(h.src[h.pos+1])
}

// isLabel reports whether the just-scanned identifier is immediately followed
// by a single ':' (a hash-key label like "a:"), which RDoc tags as ruby-value.
func (h *rubyHL) isLabel(word string) bool {
	if h.pos >= len(h.src) || h.src[h.pos] != ':' {
		return false
	}
	if h.pos+1 < len(h.src) && h.src[h.pos+1] == ':' {
		return false
	}
	return true
}

// regexpAllowed reports whether a '/' begins a regexp rather than division. The
// heuristic: a regexp is allowed at the start, or when the previous
// non-space output ended with an operator/開き-bracket context. RipperStateLex
// tracks this precisely; here we approximate by looking back at the last
// non-space byte of the source consumed so far.
func (h *rubyHL) regexpAllowed() bool {
	i := h.pos - 1
	for i >= 0 && (h.src[i] == ' ' || h.src[i] == '\t') {
		i--
	}
	if i < 0 {
		return true
	}
	switch h.src[i] {
	case '=', '(', ',', '[', '{', '~', '!', '|', '&', ';', '\n':
		return true
	}
	return false
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }

func isConstantName(word string) bool {
	if word == "" {
		return false
	}
	return word[0] >= 'A' && word[0] <= 'Z'
}
