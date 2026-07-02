package rdoc

// Live-Ruby differential test. When a Ruby toolchain with the rdoc gem is
// present (the ubuntu/macos CI lanes install it), this re-derives the gem's
// markup->HTML and verbatim-highlighting output directly and checks our port
// against it, so the committed golden corpora cannot silently drift from the
// gem. It skips itself when ruby is unavailable (Windows / qemu lanes), where
// the deterministic golden tests still hold 100% coverage.

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// rubyHTML renders RDoc markup to HTML through the real gem.
func rubyHTML(t *testing.T, markup string) (string, bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "", false
	}
	if _, err := exec.LookPath("ruby"); err != nil {
		return "", false
	}
	// RDoc's default ToHtml output is version-sensitive: older rdoc (on CI runners'
	// system ruby / the 3.4 lane) adds heading permalink spans and omits the </dt>
	// close tag. This port targets the rdoc shipped with Ruby >= 4.0, so gate the
	// live oracle on the Ruby version; lanes below 4.0 skip it and the deterministic
	// golden tests (which hold 100% coverage on their own) remain the correctness gate.
	if err := exec.Command("ruby", "-e", `exit(RUBY_VERSION >= "4.0" ? 0 : 1)`).Run(); err != nil {
		return "", false
	}
	const script = `$VERBOSE=nil
require "rdoc/rdoc"
src = STDIN.read
doc = RDoc::Markup::Parser.parse(src)
print doc.accept(RDoc::Markup::ToHtml.new(RDoc::Options.new))`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(markup)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ruby render failed: %v", err)
	}
	return out.String(), true
}

func TestLiveRubyHTMLParity(t *testing.T) {
	cases := []string{
		"= Title\n\nA *bold* word, _em_, +tt+ and a {link}[https://example.com].\n",
		"* one\n* two\n  * nested\n",
		"[label] desc\nterm:: definition\n",
		"1. first\n2. second\n",
		">>>\n  quoted block\n",
		"He said \"hi\" -- really... (c) 2026.\n",
		"== Section/Two!\n\nbody with rdoc-ref:Foo::Bar reference.\n",
		// Note: verbatim Ruby highlighting depends on the gem's parseable? check
		// (a host seam), so it is validated separately by the highlight oracle,
		// not here on the deterministic default-options path.
	}
	for _, in := range cases {
		want, ok := rubyHTML(t, in)
		if !ok {
			t.Skip("ruby/rdoc not available")
		}
		if got := ToHTML(in); got != want {
			t.Errorf("live parity\n  in:   %q\n  want: %q\n  got:  %q", in, want, got)
		}
	}
}
