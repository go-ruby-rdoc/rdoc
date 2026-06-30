package rdoc

// This file ports RDoc::Markup::AttributeManager and the inline-flow model
// (AttrChanger / RegexpHandling fragments). The attribute manager converts an
// inline string into a "flow" of fragments where each fragment is either a
// literal string, an attribute on/off change, or a regexp-handling span (a
// hyperlink/crossref placeholder resolved later by the formatter).

import (
	"regexp"
	"strings"
)

// attribute bits. In RDoc the "regexp_handling" special bit is allocated first
// (bit 0), then the AttributeManager registers BOLD, EM, TT (the word pairs) in
// that order, giving the bit values below. The next free bit (32) is the base
// for formatter-registered regexp handlers (HYPERLINK, CROSSREF, ...).
const (
	attrRegexp uint = 1 << iota // regexp_handling marker bit (value 1)
	attrBold                    // value 2
	attrEM                      // value 4
	attrTT                      // value 8
	attrUserBase                // value 16, first formatter-registered handler bit
)

// non-printing control chars used by the masking scheme, mirroring the Ruby
// constants.
const (
	chNull         = "\x00"
	chProtect      = "\x04" // PROTECT_ATTR (A_PROTECT)
	chNonPrintBeg  = "\x01" // NON_PRINTING_START
	chNonPrintEnd  = "\x02" // NON_PRINTING_END
)

// flowItem is one element of the inline flow.
type flowItem interface{ isFlow() }

// flowString is literal text.
type flowString string

func (flowString) isFlow() {}

// attrChanger turns attributes on/off between fragments.
type attrChanger struct {
	turnOn  uint
	turnOff uint
}

func (attrChanger) isFlow() {}

// regexpHandling is a span captured by a regexp handler (e.g. a hyperlink),
// carrying the attribute bit that selects the handler and the matched text.
type regexpHandling struct {
	bit  uint
	text string
}

func (regexpHandling) isFlow() {}

// regexpHandler pairs a compiled pattern with the attribute bit it sets.
type regexpHandler struct {
	re  *regexp.Regexp
	bit uint
}

// attributeManager converts inline markup to a flow. It mirrors
// RDoc::Markup::AttributeManager but is specialised to the fixed set of
// attributes RDoc registers (bold/em/tt + html tags) plus formatter-supplied
// regexp handlers.
type attributeManager struct {
	matchingWordPairs map[string]uint
	wordPairMap       []wordPair
	htmlTags          map[string]uint
	protectable       []string
	regexpHandlings   []regexpHandler
	exclusiveBitmap   uint
}

type wordPair struct {
	re  *regexp.Regexp
	bit uint
}

// newAttributeManager builds the manager with RDoc's default registrations.
func newAttributeManager() *attributeManager {
	am := &attributeManager{
		matchingWordPairs: map[string]uint{},
		htmlTags:          map[string]uint{},
		protectable:       []string{"<"},
	}
	am.addWordPair("*", "*", attrBold, true)
	am.addWordPair("_", "_", attrEM, true)
	am.addWordPair("+", "+", attrTT, true)

	am.addHTML("em", attrEM, true)
	am.addHTML("i", attrEM, true)
	am.addHTML("b", attrBold, true)
	am.addHTML("tt", attrTT, true)
	am.addHTML("code", attrTT, true)
	return am
}

func (am *attributeManager) addWordPair(start, stop string, bit uint, exclusive bool) {
	if start == stop {
		am.matchingWordPairs[start] = bit
	} else {
		pattern := regexp.MustCompile(regexp.QuoteMeta(start) + `(\S+)` + regexp.QuoteMeta(stop))
		am.wordPairMap = append(am.wordPairMap, wordPair{pattern, bit})
	}
	am.protectable = appendUniq(am.protectable, start[:1])
	if exclusive {
		am.exclusiveBitmap |= bit
	}
}

func (am *attributeManager) addHTML(tag string, bit uint, exclusive bool) {
	am.htmlTags[strings.ToLower(tag)] = bit
	if exclusive {
		am.exclusiveBitmap |= bit
	}
}

func (am *attributeManager) addRegexpHandling(pattern *regexp.Regexp, bit uint, exclusive bool) {
	am.regexpHandlings = append(am.regexpHandlings, regexpHandler{pattern, bit})
	if exclusive {
		am.exclusiveBitmap |= bit
	}
}

func (am *attributeManager) exclusive(bit uint) bool { return bit&am.exclusiveBitmap != 0 }

// flow converts str into the inline flow, mirroring AttributeManager#flow.
func (am *attributeManager) flow(str string) []flowItem {
	st := &attrState{
		str:   str,
		attrs: newAttrSpan(len(str)),
		am:    am,
	}
	st.maskProtectedSequences()
	// attrs span is sized to masked length (mask only substitutes, never grows)
	st.attrs = newAttrSpan(len(st.str))

	st.convertAttrs(true)
	st.convertHTML(true)
	st.convertRegexpHandlings(true)
	st.convertAttrs(false)
	st.convertHTML(false)
	st.convertRegexpHandlings(false)

	st.unmaskProtectedSequences()
	return st.splitIntoFlow()
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// attrState holds the mutable string + per-byte attribute bitmap during a flow
// conversion.
type attrState struct {
	str   string
	attrs *attrSpan
	am    *attributeManager
}

// attrSpan stores a bitmap per byte position, mirroring RDoc::Markup::AttrSpan.
type attrSpan struct {
	bits []uint
}

func newAttrSpan(n int) *attrSpan { return &attrSpan{bits: make([]uint, n)} }

// setAttrs ORs bit into [start, start+length); returns whether it actually
// changed anything (mirrors AttrSpan#set_attrs returning the "updated" flag).
func (a *attrSpan) setAttrs(start, length int, bit uint) bool {
	updated := false
	for i := start; i < start+length && i < len(a.bits); i++ {
		if a.bits[i]&bit == 0 {
			updated = true
		}
		a.bits[i] |= bit
	}
	return updated
}

func (a *attrSpan) at(i int) uint {
	if i < 0 || i >= len(a.bits) {
		return 0
	}
	return a.bits[i]
}

var (
	reProtectUnderscore = regexp.MustCompile(`(?i)__([a-z]+)__`)
)

// maskProtectedSequences escapes __word__ and backslash-escaped protectable
// chars, mirroring AttributeManager#mask_protected_sequences.
func (st *attrState) maskProtectedSequences() {
	st.str = reProtectUnderscore.ReplaceAllString(st.str,
		"_"+chProtect+"_"+chProtect+"${1}_"+chProtect+"_"+chProtect)

	prot := regexp.QuoteMeta(strings.Join(st.am.protectable, ""))
	// (\A|[^\\])\\([prot])  => \1\2PROTECT
	reEsc := regexp.MustCompile(`(^|[^\\])\\([` + prot + `])`)
	// Apply repeatedly to handle adjacent matches like Ruby's single global gsub
	// with overlapping anchors. One pass with the leading-char capture matches
	// Ruby semantics for non-overlapping cases; loop to be safe.
	for {
		next := reEsc.ReplaceAllString(st.str, "${1}${2}"+chProtect)
		if next == st.str {
			break
		}
		st.str = next
	}
	reDouble := regexp.MustCompile(`\\(\\[` + prot + `])`)
	st.str = reDouble.ReplaceAllString(st.str, "${1}")
}

// unmaskProtectedSequences reverses the protect markers, mirroring
// AttributeManager#unmask_protected_sequences.
func (st *attrState) unmaskProtectedSequences() {
	// (.)PROTECT => \1NUL  -- replace each protect char (preceded by any char)
	var b strings.Builder
	runes := []byte(st.str)
	for i := 0; i < len(runes); i++ {
		if i+1 < len(runes) && string(runes[i+1]) == chProtect {
			b.WriteByte(runes[i])
			b.WriteString(chNull)
			i++ // skip the protect char
		} else {
			b.WriteByte(runes[i])
		}
	}
	st.str = b.String()
}

// convertAttrs handles word-pair (matching + mapped) attributes.
func (st *attrState) convertAttrs(exclusive bool) {
	st.convertMatchingWordPairs(exclusive)
	st.convertWordPairMap(exclusive)
}

// convertMatchingWordPairs implements the RDoc matching-word-pair rule.
//
// The Ruby regex is:
//
//	/(?:^|\W|ALL)\K(TAG)(\1*[#\\]?[\w:PROTECT.\/\[\]-]+?\S?)\1(?!\1)(?=ALL|\W|$)/
//
// where TAG is the delimiter (e.g. "*"), ALL is the class of all delimiters and
// \K resets the match start. RE2 has no backreferences, so this is implemented
// as a direct left-to-right scan reproducing the same boundary conditions.
func (st *attrState) convertMatchingWordPairs(exclusive bool) {
	sel := map[byte]uint{}
	for k, bit := range st.am.matchingWordPairs {
		if exclusive == st.am.exclusive(bit) {
			sel[k[0]] = bit
		}
	}
	if len(sel) == 0 {
		return
	}
	all := map[byte]bool{}
	for k := range st.am.matchingWordPairs {
		all[k[0]] = true
	}
	st.scanMatchingPairs(sel, all)
}

// scanMatchingPairs performs one full left-to-right scan, replacing every
// matching pair. It loops until no change, mirroring the `1 while gsub!`.
func (st *attrState) scanMatchingPairs(sel map[byte]uint, all map[byte]bool) {
	for {
		matched := st.scanMatchingPairsOnce(sel, all)
		if !matched {
			return
		}
	}
}

func (st *attrState) scanMatchingPairsOnce(sel map[byte]uint, all map[byte]bool) bool {
	s := st.str
	for i := 0; i < len(s); i++ {
		bit, ok := sel[s[i]]
		if !ok {
			continue
		}
		// preceding boundary: ^ | \W | any delimiter char
		if i > 0 {
			prev := s[i-1]
			if !(isNonWord(prev) || all[prev]) {
				continue
			}
		}
		delim := s[i]
		// the opening delimiter must not be followed by PROTECT
		if i+1 < len(s) && s[i+1] == chProtect[0] {
			continue
		}
		// match the word: (\1*[#\\]?[\w:PROTECT./\[\]-]+?\S?) then \1 (?!\1)
		end, wordStart, wordEnd, found := matchPairWord(s, i, delim)
		if !found {
			continue
		}
		// trailing boundary: ALL | \W | $
		if end+1 < len(s) {
			nx := s[end+1]
			if !(isNonWord(nx) || all[nx]) {
				continue
			}
		}
		word := s[wordStart:wordEnd]
		updated := st.attrs.setAttrs(wordStart, wordEnd-wordStart, bit)
		openLen := wordStart - i
		closeLen := end - wordEnd + 1
		var open, clo string
		if updated {
			open = strings.Repeat(chNull, openLen)
			clo = strings.Repeat(chNull, closeLen)
		} else {
			open = s[i:wordStart]
			clo = s[wordEnd : end+1]
		}
		st.str = s[:i] + open + word + clo + s[end+1:]
		return true
	}
	return false
}

// matchPairWord matches the word body of a matching pair beginning at the
// opening delimiter index start. It returns the index of the closing delimiter,
// the word span [wordStart,wordEnd), and whether a match was found. It mirrors
// the inner regex (\1*[#\\]?[\w:PROTECT./\[\]-]+?\S?)\1(?!\1).
func matchPairWord(s string, start int, delim byte) (closeIdx, wordStart, wordEnd int, ok bool) {
	p := start + 1
	wordStart = p
	// \1* : extra leading delimiters are part of the word
	for p < len(s) && s[p] == delim {
		p++
	}
	// [#\\]?
	if p < len(s) && (s[p] == '#' || s[p] == '\\') {
		p++
	}
	bodyStart := p
	// [\w:PROTECT./\[\]-]+? (non-greedy, at least one) then optional \S?, then \1
	for q := bodyStart; q < len(s); q++ {
		if !isWordBodyChar(s[q]) {
			break
		}
		// non-greedy: try smallest body first. After at least one body char,
		// allow an optional single \S then require the closing delimiter.
		// q is last consumed body char index. Try close right after, or after
		// one extra \S char.
		// require at least one body char => q >= bodyStart
		// option A: close at q+1
		if q+1 < len(s) && s[q+1] == delim {
			if !(q+2 < len(s) && s[q+2] == delim) { // (?!\1)
				return q + 1, wordStart, q + 1, true
			}
		}
		// option B: one trailing \S then close at q+2
		if q+1 < len(s) && isNonSpace(s[q+1]) && q+2 < len(s) && s[q+2] == delim {
			if !(q+3 < len(s) && s[q+3] == delim) {
				return q + 2, wordStart, q + 2, true
			}
		}
	}
	return 0, 0, 0, false
}

func isNonWord(b byte) bool {
	return !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_')
}
func isNonSpace(b byte) bool { return b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '\f' && b != '\v' }
func isWordBodyChar(b byte) bool {
	if !isNonWord(b) {
		return true // \w
	}
	switch b {
	case ':', chProtect[0], '.', '/', '[', ']', '-':
		return true
	}
	return false
}

// convertWordPairMap handles non-matching word pairs (different start/stop).
// RDoc registers none by default, so this is a faithful but rarely-exercised
// path kept for completeness.
func (st *attrState) convertWordPairMap(exclusive bool) {
	for _, wp := range st.am.wordPairMap {
		if exclusive != st.am.exclusive(wp.bit) {
			continue
		}
		for {
			loc := wp.re.FindStringSubmatchIndex(st.str)
			if loc == nil {
				break
			}
			ws, we := loc[4], loc[5] // group 2 = the word
			word := st.str[ws:we]
			updated := st.attrs.setAttrs(ws, we-ws, wp.bit)
			if !updated {
				break
			}
			startLen := loc[3] - loc[2]
			stopLen := loc[7] - loc[6]
			st.str = st.str[:loc[2]] + strings.Repeat(chNull, startLen) + word +
				strings.Repeat(chNull, stopLen) + st.str[loc[1]:]
		}
	}
}

func (st *attrState) convertHTML(exclusive bool) {
	var tags []string
	for tag, bit := range st.am.htmlTags {
		if exclusive == st.am.exclusive(bit) {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return
	}
	for _, tag := range sortedTags(tags) {
		bit := st.am.htmlTags[tag]
		re := regexp.MustCompile(`(?i)<(` + regexp.QuoteMeta(tag) + `)>(.*?)</` + regexp.QuoteMeta(tag) + `>`)
		for {
			loc := re.FindStringSubmatchIndex(st.str)
			if loc == nil {
				break
			}
			tagS, tagE := loc[2], loc[3]
			contS, contE := loc[4], loc[5]
			tagLen := (tagE - tagS) + 2 // "<tag>".length
			cont := st.str[contS:contE]
			st.attrs.setAttrs(contS, contE-contS, bit)
			seq := strings.Repeat(chNull, tagLen)
			// replace <tag>cont</tag> with seq+cont+seq+NUL (same total length)
			st.str = st.str[:loc[0]] + seq + cont + seq + chNull + st.str[loc[1]:]
		}
	}
}

func (st *attrState) convertRegexpHandlings(exclusive bool) {
	for _, h := range st.am.regexpHandlings {
		if exclusive != st.am.exclusive(h.bit) {
			continue
		}
		locs := h.re.FindAllStringSubmatchIndex(st.str, -1)
		for _, loc := range locs {
			// capture 0 if no groups, else group 1
			capStart, capEnd := loc[0], loc[1]
			if len(loc) >= 4 && loc[2] >= 0 {
				capStart, capEnd = loc[2], loc[3]
			}
			st.attrs.setAttrs(capStart, capEnd-capStart, h.bit|attrRegexp)
		}
	}
}

// splitIntoFlow chunks the string by attribute change, mirroring
// AttributeManager#split_into_flow.
func (st *attrState) splitIntoFlow() []flowItem {
	var res []flowItem
	current := uint(0)
	strLen := len(st.str)

	copyString := func(start, end int) string {
		return strings.ReplaceAll(st.str[start:end], chNull, "")
	}

	i := 0
	for i < strLen && st.str[i] == 0 {
		i++
	}
	startPos := i

	for i < strLen {
		newAttr := st.attrs.at(i)
		if newAttr != current {
			if i > startPos {
				res = append(res, flowString(copyString(startPos, i)))
				startPos = i
			}
			res = append(res, changeAttribute(current, newAttr))
			current = newAttr

			if current&attrRegexp != 0 {
				for i < strLen && st.attrs.at(i)&attrRegexp != 0 {
					i++
				}
				res = append(res, regexpHandling{bit: current, text: copyString(startPos, i)})
				startPos = i
				continue
			}
		}
		i++
		for i < strLen && st.str[i] == 0 {
			i++
		}
	}

	if startPos < strLen {
		res = append(res, flowString(copyString(startPos, strLen)))
	}
	if current != 0 {
		res = append(res, changeAttribute(current, 0))
	}
	return res
}

// changeAttribute computes the attrChanger from current to new.
func changeAttribute(current, next uint) attrChanger {
	diff := current ^ next
	return attrChanger{turnOn: next & diff, turnOff: current & diff}
}

func keysOf(m map[string]uint) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// stable order: by the natural order of the three delimiters * _ +
	sortStrings(out)
	return out
}

func sortedTags(t []string) []string {
	out := append([]string(nil), t...)
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
