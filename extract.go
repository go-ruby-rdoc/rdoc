package rdoc

// Focused reader of Ruby source into the code-object model. It walks the source
// line by line, tracking lexical nesting (class/module/def...end), the pending
// documentation comment block, and the current visibility, recognising the
// declaration forms RDoc documents statically.

import (
	"regexp"
	"strings"
)

var (
	reClass     = regexp.MustCompile(`^(class|module)\s+([A-Z]\w*(?:::[A-Z]\w*)*)\s*(?:<\s*(\S+))?\s*$`)
	reClassBody = regexp.MustCompile(`^(class|module)\b`)
	reDef       = regexp.MustCompile(`^def\s+(?:(self|[A-Z]\w*)\.)?([\w]+[?!=]?|\[\]=?|[<>=!+\-*/%]+|<=>|==|<<|>>)\s*(\(.*?\))?`)
	reConst     = regexp.MustCompile(`^([A-Z]\w*)\s*=\s*(.+?)\s*$`)
	reAttr      = regexp.MustCompile(`^attr(_reader|_writer|_accessor)?\s+(.+?)\s*$`)
	reVisib     = regexp.MustCompile(`^(public|private|protected)\s*$`)
	reAlias     = regexp.MustCompile(`^alias_method\s+:(\w+[?!=]?)\s*,\s*:(\w+[?!=]?)`)
	reAliasKw   = regexp.MustCompile(`^alias\s+:?(\w+[?!=]?)\s+:?(\w+[?!=]?)`)
	reEnd       = regexp.MustCompile(`^end\b`)
	reBlockOpen = regexp.MustCompile(`^(begin|if|unless|while|until|case|for)\b|\bdo\b\s*(\|[^|]*\|)?\s*$`)
	reSymbol    = regexp.MustCompile(`:(\w+[?!=]?)`)
)

// Extract parses Ruby source into a TopLevel code object named after fileName.
// It is the focused equivalent of running RDoc::Parser::Ruby over one file.
func Extract(fileName, source string) *TopLevel {
	e := &extractor{
		top:   &TopLevel{Name: fileName},
		lines: splitLines(source),
	}
	e.run()
	return e.top
}

type scope struct {
	cm         *ClassModule // nil at file top level
	visibility Visibility
	depth      int // nesting depth at which this scope's `end` closes
}

type extractor struct {
	top   *TopLevel
	lines []string

	stack       []*scope
	comment     []string // accumulating raw comment lines (without leading '#')
	haveComment bool
	depth       int
}

func (e *extractor) run() {
	// file-level scope (top-level defs would attach to an Object class in RDoc;
	// here they are dropped unless inside a class/module, matching the common
	// documentation case).
	e.stack = append(e.stack, &scope{cm: nil, visibility: Public, depth: 0})

	for idx := 0; idx < len(e.lines); idx++ {
		raw := e.lines[idx]
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimLeft(line, " \t")

		// =begin ... =end block comment
		if trimmed == "=begin" || strings.HasPrefix(trimmed, "=begin ") {
			e.resetComment()
			e.haveComment = true
			for idx++; idx < len(e.lines); idx++ {
				bt := strings.TrimRight(e.lines[idx], "\r")
				if bt == "=end" || strings.HasPrefix(strings.TrimLeft(bt, " \t"), "=end") {
					break
				}
				e.comment = append(e.comment, bt)
			}
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			e.accumComment(trimmed)
			continue
		}
		if trimmed == "" {
			// a blank line breaks a pending comment block only if it is the
			// =begin/=end style; inline blank lines inside a leading comment are
			// rare. RDoc keeps the comment attached across a single blank line
			// only within the block; a blank source line ends attachment.
			e.resetComment()
			continue
		}

		switch {
		case reClass.MatchString(trimmed):
			e.handleClass(trimmed)
		case reDef.MatchString(trimmed):
			e.handleDef(trimmed)
			if !isOneLineBlock(trimmed) {
				e.depth++
			}
		case reVisib.MatchString(trimmed):
			m := reVisib.FindStringSubmatch(trimmed)
			e.cur().visibility = Visibility(m[1])
			e.resetComment()
		case reAlias.MatchString(trimmed):
			e.handleAlias(reAlias.FindStringSubmatch(trimmed))
		case reAliasKw.MatchString(trimmed):
			e.handleAlias(reAliasKw.FindStringSubmatch(trimmed))
		case reAttr.MatchString(trimmed):
			e.handleAttr(reAttr.FindStringSubmatch(trimmed))
		case reConst.MatchString(trimmed) && e.cur().cm != nil:
			m := reConst.FindStringSubmatch(trimmed)
			e.cur().cm.Constants = append(e.cur().cm.Constants, &Constant{
				Name: m[1], Value: m[2], Comment: e.takeComment(),
			})
		case reEnd.MatchString(trimmed):
			e.handleEnd()
		default:
			if opensBlock(trimmed) && !isOneLineBlock(trimmed) {
				e.depth++
			}
			e.resetComment()
		}
	}
}

// opensBlock reports whether a line opens a multi-line block (a leading block
// keyword). It is deliberately conservative: it only matches lines that begin
// with a block keyword so trailing-modifier `if`/`while` do not open a block.
func opensBlock(trimmed string) bool {
	return reBlockOpen.MatchString(trimmed)
}

// isOneLineBlock reports whether a block is closed on the same line (contains an
// `end` after a `;` or inline), so it should not change nesting depth.
func isOneLineBlock(trimmed string) bool {
	return strings.Contains(trimmed, ";") && strings.Contains(trimmed, "end") ||
		strings.HasSuffix(trimmed, " end") || strings.HasSuffix(trimmed, "}")
}

func (e *extractor) cur() *scope { return e.stack[len(e.stack)-1] }

// accumComment appends a comment line, stripping the leading '#' and one space,
// mirroring RDoc comment normalization. A bare "##" block marker is dropped.
func (e *extractor) accumComment(trimmed string) {
	body := trimmed[1:] // drop first '#'
	// a leading second '#' on the very first line ("##") is a block marker
	if !e.haveComment && body == "#" {
		e.haveComment = true
		e.comment = nil
		return
	}
	if strings.HasPrefix(body, "#") && !e.haveComment {
		// "## text" style: drop the extra '#'
		body = body[1:]
	}
	body = strings.TrimPrefix(body, " ")
	e.comment = append(e.comment, body)
	e.haveComment = true
}

func (e *extractor) resetComment() {
	e.comment = nil
	e.haveComment = false
}

// takeComment returns the accumulated comment as RDoc markup source and clears
// it. Trailing blank lines are trimmed, matching RDoc.
func (e *extractor) takeComment() string {
	if !e.haveComment {
		return ""
	}
	lines := e.comment
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	e.resetComment()
	return strings.Join(lines, "\n")
}

func (e *extractor) handleClass(trimmed string) {
	m := reClass.FindStringSubmatch(trimmed)
	isModule := m[1] == "module"
	qual := m[2]
	comment := e.takeComment()

	// Split a qualified name (A::B::C) into segments. All but the last become
	// intermediate modules (created on demand); the last is the declared
	// class/module itself. This mirrors RDoc's namespace expansion.
	segments := strings.Split(qual, "::")
	parent := e.cur().cm // nil at file top level
	parentFull := ""
	if parent != nil {
		parentFull = parent.FullName + "::"
	}

	for i, seg := range segments {
		last := i == len(segments)-1
		full := parentFull + strings.Join(segments[:i+1], "::")
		cm := e.lookupCM(parent, seg)
		if cm == nil {
			cm = &ClassModule{Name: seg, FullName: full}
			e.registerCM(parent, cm)
		}
		if last {
			cm.IsModule = isModule
			cm.Superclass = m[3]
			cm.Comment = comment
		} else if cm.Comment == "" {
			// intermediate namespace segments are modules
			cm.IsModule = true
		}
		parent = cm
	}

	e.depth++
	e.stack = append(e.stack, &scope{cm: parent, visibility: Public, depth: e.depth})
}

// lookupCM finds an existing class/module by simple name under parent (or at
// top level when parent is nil).
func (e *extractor) lookupCM(parent *ClassModule, name string) *ClassModule {
	if parent == nil {
		for _, cm := range e.top.ClassesAndModule {
			if cm.Name == name {
				return cm
			}
		}
		return nil
	}
	return parent.findNested(name)
}

// registerCM adds cm under parent (or at top level when parent is nil).
func (e *extractor) registerCM(parent *ClassModule, cm *ClassModule) {
	if parent == nil {
		e.top.ClassesAndModule = append(e.top.ClassesAndModule, cm)
	} else {
		parent.Nested = append(parent.Nested, cm)
	}
}

func (e *extractor) handleDef(trimmed string) {
	m := reDef.FindStringSubmatch(trimmed)
	singleton := m[1] != ""
	params := m[3]
	if params == "" {
		// RDoc normalises a parameterless method to "()".
		params = "()"
	}
	meth := &AnyMethod{
		Name:       m[2],
		Params:     params,
		Singleton:  singleton,
		Visibility: e.cur().visibility,
	}
	meth.Comment, meth.CallSeq = extractCallSeq(e.takeComment())
	if cm := e.cur().cm; cm != nil {
		cm.Methods = append(cm.Methods, meth)
	}
}

func (e *extractor) handleAttr(m []string) {
	rw := "R"
	switch m[1] {
	case "_writer":
		rw = "W"
	case "_accessor":
		rw = "RW"
	case "_reader", "":
		rw = "R"
	}
	comment := e.takeComment()
	vis := e.cur().visibility
	for _, sm := range reSymbol.FindAllStringSubmatch(m[2], -1) {
		if cm := e.cur().cm; cm != nil {
			cm.Attrs = append(cm.Attrs, &Attr{
				Name: sm[1], RW: rw, Visibility: vis, Comment: comment,
			})
		}
	}
}

func (e *extractor) handleAlias(m []string) {
	comment := e.takeComment()
	if cm := e.cur().cm; cm != nil {
		cm.Aliases = append(cm.Aliases, &Alias{
			NewName: m[1], OldName: m[2], Comment: comment,
		})
	}
}

func (e *extractor) handleEnd() {
	e.resetComment()
	if e.cur().cm != nil && e.cur().depth == e.depth {
		e.stack = e.stack[:len(e.stack)-1]
		e.depth--
		return
	}
	if e.depth > 0 {
		e.depth--
	}
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}
