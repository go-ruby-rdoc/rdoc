package rdoc

// Inline conversion for the HTML formatter: turning the attribute-manager flow
// into HTML (tag on/off, hyperlinks, tidy links) and the Text smart-quote pass.

import (
	"regexp"
	"strings"
)

// toHTML converts an inline markup string to HTML, mirroring
// RDoc::Markup::ToHtml#to_html (convert_flow over the attribute flow, then the
// RDoc::Text#to_html entity pass).
func (h *ToHtml) toHTML(item string) string {
	flow := h.am.flow(item)
	converted := h.convertFlow(flow)
	return textToHTML(converted)
}

// convertFlow turns the flow into a string, applying tag on/off and resolving
// regexp handlings, mirroring ToHtml#convert_flow.
func (h *ToHtml) convertFlow(flow []flowItem) string {
	var res strings.Builder
	for _, f := range flow {
		switch v := f.(type) {
		case flowString:
			h.appendFragment(&res, escapeHTML(string(v)))
		case attrChanger:
			h.offTags(&res, v)
			h.onTags(&res, v)
		case regexpHandling:
			h.appendFragment(&res, h.convertRegexpHandling(v))
		}
	}
	return res.String()
}

func (h *ToHtml) appendFragment(res *strings.Builder, frag string) {
	res.WriteString(frag)
}

// onTags / offTags emit the opening/closing HTML for attribute changes.
func (h *ToHtml) onTags(res *strings.Builder, c attrChanger) {
	for _, t := range tagsFor(c.turnOn, true) {
		h.appendFragment(res, t)
	}
}

func (h *ToHtml) offTags(res *strings.Builder, c attrChanger) {
	for _, t := range tagsFor(c.turnOff, false) {
		h.appendFragment(res, t)
	}
}

// tagsFor maps the bold/em/tt bits to their HTML tags. on selects open vs close.
func tagsFor(bits uint, on bool) []string {
	var out []string
	// order: off processes in reverse of the gem's tag list; the gem stores
	// tags in a stack and emits matching pairs. For the BOLD/TT/EM set used
	// here the simple deterministic order below reproduces the gem output for
	// the single-attribute spans RDoc produces.
	type tag struct {
		bit        uint
		open, clos string
	}
	tags := []tag{
		{attrBold, "<strong>", "</strong>"},
		{attrTT, "<code>", "</code>"},
		{attrEM, "<em>", "</em>"},
	}
	if on {
		for _, t := range tags {
			if bits&t.bit != 0 {
				out = append(out, t.open)
			}
		}
	} else {
		for i := len(tags) - 1; i >= 0; i-- {
			if bits&tags[i].bit != 0 {
				out = append(out, tags[i].clos)
			}
		}
	}
	return out
}

// convertRegexpHandling resolves a hyperlink/tidylink/rdoclink/crossref span.
func (h *ToHtml) convertRegexpHandling(rh regexpHandling) string {
	switch {
	case rh.bit&bitHyperlink != 0:
		url := escapeHTML(rh.text)
		return h.genURL(url, url)
	case rh.bit&bitRDocLink != 0:
		return h.handleRDocLink(rh.text)
	case rh.bit&bitTidyLink != 0:
		return h.handleTidyLink(rh.text)
	case rh.bit&bitCrossRef != 0:
		return h.handleCrossRef(rh.text)
	}
	return escapeHTML(rh.text)
}

var (
	reTidyBraces     = regexp.MustCompile(`^\{(.*?)\}\[(.*?)\](.*)$`)
	reTidySingleWord = regexp.MustCompile(`^(\S+)\[(.*?)\](.*)$`)
	reImageExt       = regexp.MustCompile(`(?i)\.(gif|png|jpg|jpeg|bmp)$`)
	reRDocFileLink   = regexp.MustCompile(`(?i)^((?:[^/#]*/)*)([^/#]+)\.(rb|rdoc|md)(?:$|#)`)
)

// handleTidyLink renders "label[url]" / "{label}[url]", mirroring
// convert_complete_tidy_link. Inline markup inside a braced label
// ("{a *b* c}[url]") renders as plain text rather than nested tags; the gem's
// fragment-accumulating tidy-link state machine is out of scope.
func (h *ToHtml) handleTidyLink(text string) string {
	var m []string
	if m = reTidyBraces.FindStringSubmatch(text); m == nil {
		if m = reTidySingleWord.FindStringSubmatch(text); m == nil {
			return text
		}
	}
	label := m[1]
	url := escapeHTML(m[2])
	var labelHTML string
	if strings.HasPrefix(label, "rdoc-image:") {
		labelHTML = h.handleRDocLink(label)
	} else {
		labelHTML = h.toHTML(label)
	}
	return h.genURL(url, labelHTML)
}

// handleRDocLink renders an rdoc-scheme link, mirroring handle_RDOCLINK.
func (h *ToHtml) handleRDocLink(url string) string {
	switch {
	case strings.HasPrefix(url, "rdoc-ref:"):
		return escapeHTML(strings.TrimPrefix(url, "rdoc-ref:"))
	case strings.HasPrefix(url, "rdoc-label:"):
		text := strings.TrimPrefix(url, "rdoc-label:")
		switch {
		case strings.HasPrefix(text, "label-"):
			text = strings.TrimPrefix(text, "label-")
		case strings.HasPrefix(text, "footmark-"):
			text = strings.TrimPrefix(text, "footmark-")
		case strings.HasPrefix(text, "foottext-"):
			text = strings.TrimPrefix(text, "foottext-")
		}
		return h.genURL(escapeHTML(url), escapeHTML(text))
	case strings.HasPrefix(url, "rdoc-image:"):
		rest := strings.TrimPrefix(url, "rdoc-image:")
		u, alt := splitImageAlt(rest)
		if alt != "" {
			return `<img src="` + escapeHTML(u) + `" alt="` + escapeHTML(alt) + `">`
		}
		return `<img src="` + escapeHTML(u) + `">`
	}
	if m := regexp.MustCompile(`(?i)^rdoc-[a-z]+:`).FindString(url); m != "" {
		return escapeHTML(url[len(m):])
	}
	return escapeHTML(url)
}

// splitImageAlt splits "path:alt" where the ':' is not part of a URL scheme,
// mirroring the $'.split(/:(?!\/)/, 2) in handle_RDOCLINK.
func splitImageAlt(s string) (string, string) {
	// split at the first ':' not followed by '/', mirroring split(/:(?!\/)/, 2).
	for i := 0; i < len(s); i++ {
		if s[i] == ':' && !(i+1 < len(s) && s[i+1] == '/') {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

// handleCrossRef resolves a bare name via the configured CrossRef resolver,
// mirroring ToHtmlCrossref. A leading backslash suppresses the link.
func (h *ToHtml) handleCrossRef(text string) string {
	if strings.HasPrefix(text, "\\") {
		return escapeHTML(text[1:])
	}
	if h.opts.CrossRef != nil {
		if href, ok := h.opts.CrossRef(text); ok {
			return `<a href="` + escapeHTML(href) + `">` + escapeHTML(text) + "</a>"
		}
	}
	return escapeHTML(text)
}

// genURL builds the anchor (or img) for a link, mirroring ToHtml#gen_url.
func (h *ToHtml) genURL(url, text string) string {
	scheme, u, id := parseURL(url)

	if (scheme == "http" || scheme == "https" || scheme == "link") && reImageExt.MatchString(u) {
		return `<img src="` + u + `" />`
	}

	// Rewrite *.rb/*.rdoc/*.md links to their generated .html names (unless a
	// link: scheme or an absolute http(s) URL).
	if scheme != "link" && !regexp.MustCompile(`(?i)^https?:`).MatchString(u) {
		if m := reRDocFileLink.FindStringSubmatch(u); m != nil {
			tail := u[len(m[0]):]
			// reRDocFileLink consumes the '#' (its $ branch) only when followed
			// by end; preserve any fragment via tail.
			frag := ""
			if idx := strings.Index(u, "#"); idx >= 0 && idx >= len(m[1])+len(m[2])+1+len(m[3]) {
				frag = u[idx:]
				tail = ""
			}
			u = m[1] + strings.ReplaceAll(m[2], ".", "_") + "_" + m[3] + ".html" + frag + tail
		}
	}

	// strip the actual "scheme:/*" prefix from the displayed text (mirroring
	// text.sub(%r%^#{scheme}:/*%i, '')). Only the resolved scheme is stripped,
	// so a label like "foo:bar" keeps its colon.
	t := text
	if scheme != "" {
		t = regexp.MustCompile(`(?i)^`+regexp.QuoteMeta(scheme)+`:/*`).ReplaceAllString(t, "")
	}
	t = regexp.MustCompile(`^[*^](\d+)$`).ReplaceAllString(t, "$1")

	link := "<a" + id + ` href="` + u + `">` + t + "</a>"
	if strings.Contains(id, `"foot`) {
		link = "<sup>" + link + "</sup>"
	}
	return link
}

var (
	reRDocLabelURL = regexp.MustCompile(`^rdoc-label:([^:]*)(?::(.*))?`)
	reSchemeColon  = regexp.MustCompile(`([A-Za-z]+):(.*)`)
)

// parseURL splits a URL into (scheme, url, id-attr), mirroring
// Formatter#parse_url. gen_relative_url (for non-anchor link: targets) reduces
// to the path itself in this pure, single-document core.
func parseURL(url string) (scheme, outURL, id string) {
	outURL = url
	switch {
	case reRDocLabelURL.MatchString(url):
		m := reRDocLabelURL.FindStringSubmatch(url)
		scheme = "link"
		path := "#" + m[1]
		if m[2] != "" {
			id = ` id="` + m[2] + `"`
		}
		outURL = path
	case reSchemeColon.MatchString(url):
		m := reSchemeColon.FindStringSubmatch(url)
		scheme = strings.ToLower(m[1])
	case strings.HasPrefix(url, "#"):
		// anchor-only: scheme stays empty, url unchanged
	default:
		scheme = "http"
	}
	return scheme, outURL, id
}
