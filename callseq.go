package rdoc

import "strings"

// extractCallSeq splits a normalized comment into (comment, call-seq), mirroring
// RDoc's handling of a "call-seq:" directive: the indented lines following the
// directive become the call-seq, and they are removed from the comment text.
// If there is no directive, callSeq is empty and comment is returned unchanged.
func extractCallSeq(comment string) (string, string) {
	lines := strings.Split(comment, "\n")
	var out []string
	var seq []string
	inSeq := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if !inSeq {
			if strings.EqualFold(trimmed, "call-seq:") {
				inSeq = true
				out = append(out, "") // the directive line leaves a blank
				continue
			}
			out = append(out, ln)
			continue
		}
		// inside call-seq: indented (or blank) lines belong to the sequence
		if trimmed == "" {
			inSeq = false
			out = append(out, ln)
			continue
		}
		if strings.HasPrefix(ln, "  ") || strings.HasPrefix(ln, "\t") {
			seq = append(seq, strings.TrimLeft(ln, " \t"))
			continue
		}
		inSeq = false
		out = append(out, ln)
	}
	if len(seq) == 0 {
		return comment, ""
	}
	callSeq := strings.Join(seq, "\n") + "\n"
	return strings.Join(out, "\n"), callSeq
}
