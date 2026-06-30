package rdoc

// Ports of RDoc::Text helpers used by the HTML formatter: HTML escaping, the
// smart-quote/dash/entity conversion pass (Text#to_html), and the verbatim
// "parseable?" heuristic.

import (
	"regexp"
	"strings"
)

// escapeHTML mirrors CGI.escapeHTML: & < > " ' -> entities.
func escapeHTML(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// entity replacements used by textToHTML. The rdoc gem emits the literal UTF-8
// characters (RDoc::Text::TO_HTML_CHARACTERS), not numeric entities.
const (
	entCloseDQuote = "”" // ”
	entCloseSQuote = "’" // ’
	entCopyright   = "©" // ©
	entEllipsis    = "…" // …
	entEmDash      = "—" // —
	entEnDash      = "–" // –
	entOpenDQuote  = "“" // “
	entOpenSQuote  = "‘" // ‘
	entTrademark   = "®" // ®
)

var (
	reTTSpan   = regexp.MustCompile(`^<(tt|code)>.*?</(tt|code)>`)
	reTTOpen   = regexp.MustCompile(`^<(tt|code)>`)
	reHTMLTag  = regexp.MustCompile(`^<[^>]+/?s*>`)
	reEscapedC = regexp.MustCompile(`^\\(\S)`)
	reWordEnd  = regexp.MustCompile(`\w$`)
)

// textToHTML applies the RDoc::Text#to_html entity pass: smart quotes, dashes,
// ellipsis, (c)/(r), while leaving the contents of <tt>/<code> and other HTML
// tags untouched.
func textToHTML(text string) string {
	var b strings.Builder
	s := text
	insquotes := false
	indquotes := false
	afterWord := false

	for len(s) > 0 {
		switch {
		case matchPrefix(reTTSpan, s):
			m := reTTSpan.FindString(s)
			b.WriteString(strings.ReplaceAll(m, `\\`, `\`))
			s = s[len(m):]
		case matchPrefix(reTTOpen, s):
			m := reTTOpen.FindString(s)
			b.WriteString(m)
			s = s[len(m):]
		case matchPrefix(reHTMLTag, s):
			m := reHTMLTag.FindString(s)
			b.WriteString(m)
			s = s[len(m):]
		case matchPrefix(reEscapedC, s):
			m := reEscapedC.FindStringSubmatch(s)
			b.WriteString(m[1])
			s = s[len(m[0]):]
			afterWord = false
		case strings.HasPrefix(s, "...."):
			b.WriteString("." + entEllipsis)
			s = s[4:]
			afterWord = false
		case strings.HasPrefix(s, "..."):
			b.WriteString(entEllipsis)
			s = s[3:]
			afterWord = false
		case hasPrefixFold(s, "(c)"):
			b.WriteString(entCopyright)
			s = s[3:]
			afterWord = false
		case hasPrefixFold(s, "(r)"):
			b.WriteString(entTrademark)
			s = s[3:]
			afterWord = false
		case strings.HasPrefix(s, "---"):
			b.WriteString(entEmDash)
			s = s[3:]
			afterWord = false
		case strings.HasPrefix(s, "--"):
			b.WriteString(entEnDash)
			s = s[2:]
			afterWord = false
		// Quote characters always arrive HTML-escaped (the inline flow is
		// CGI-escaped before this pass), so only the entity forms are matched;
		// raw '"' / '\'' never reach here.
		case strings.HasPrefix(s, "&quot;"):
			if indquotes {
				b.WriteString(entCloseDQuote)
			} else {
				b.WriteString(entOpenDQuote)
			}
			indquotes = !indquotes
			s = s[len("&quot;"):]
			afterWord = false
		case strings.HasPrefix(s, "``"):
			b.WriteString(entOpenDQuote)
			s = s[2:]
			afterWord = false
		case strings.HasPrefix(s, "&#39;&#39;"):
			b.WriteString(entCloseDQuote)
			s = s[len("&#39;&#39;"):]
			afterWord = false
		case strings.HasPrefix(s, "`"):
			if insquotes || afterWord {
				b.WriteString("`")
				afterWord = false
			} else {
				b.WriteString(entOpenSQuote)
				insquotes = true
			}
			s = s[1:]
		case strings.HasPrefix(s, "&#39;"):
			if insquotes {
				b.WriteString(entCloseSQuote)
				insquotes = false
			} else if afterWord {
				b.WriteString(entCloseSQuote)
			} else {
				b.WriteString(entOpenSQuote)
				insquotes = true
			}
			s = s[len("&#39;"):]
			afterWord = false
		default:
			// advance to the next significant character: < \ . ( " ' ` & -
			idx := strings.IndexAny(s, "<\\.(\"'`&-")
			if idx < 0 {
				b.WriteString(s)
				s = ""
			} else if idx == 0 {
				// significant char that none of the above matched (e.g. lone
				// '&' followed by entity, or '(' not (c)/(r)); emit it raw.
				b.WriteByte(s[0])
				afterWord = false
				s = s[1:]
			} else {
				chunk := s[:idx]
				b.WriteString(chunk)
				afterWord = reWordEnd.MatchString(chunk)
				s = s[idx:]
			}
		}
	}
	return b.String()
}

func matchPrefix(re *regexp.Regexp, s string) bool {
	loc := re.FindStringIndex(s)
	return loc != nil && loc[0] == 0
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix)
}

// parseable reports whether text looks like valid Ruby, mirroring
// ToHtml#parseable?. The gem evaluates the block with the real Ruby parser,
// which this pure-Go core cannot reproduce, so detection is delegated to the
// optional HTMLOptions.Parseable host seam. When that is nil, blocks are never
// auto-detected as Ruby and render as a plain escaped <pre> (the deterministic,
// ruby-free output path).
func (h *ToHtml) parseable(text string) bool {
	if h.opts.Parseable == nil {
		return false
	}
	return h.opts.Parseable(text)
}
