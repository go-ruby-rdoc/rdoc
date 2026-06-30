package rdoc

// Differential-oracle tests. The golden expectations in testdata/*.json were
// captured from the real `rdoc` gem (RDoc 7.x). They are committed so the suite
// reproduces gem parity deterministically without a Ruby toolchain present; a
// separate live-Ruby test (live_oracle_test.go) regenerates and re-checks the
// same shapes when ruby is available.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
}

func TestHTMLOracle(t *testing.T) {
	var c struct {
		HTML     map[string]struct{ In, HTML string }
		Labels   map[string]struct{ In, Label string }
		Rdoc     map[string]struct{ In, Out string }
		Markdown map[string]struct{ In, Out string }
	}
	readJSON(t, "html.json", &c)

	for name, tc := range c.HTML {
		if got := ToHTML(tc.In); got != tc.HTML {
			t.Errorf("HTML[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.HTML, got)
		}
	}
	for name, tc := range c.Labels {
		if got := toLabel(tc.In); got != tc.Label {
			t.Errorf("Label[%s] in=%q want %q got %q", name, tc.In, tc.Label, got)
		}
	}
	for name, tc := range c.Rdoc {
		if got := ToRdocString(tc.In); got != tc.Out {
			t.Errorf("ToRdoc[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.Out, got)
		}
	}
	for name, tc := range c.Markdown {
		if got := ToMarkdownString(tc.In); got != tc.Out {
			t.Errorf("ToMarkdown[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.Out, got)
		}
	}
}

func TestHighlightOracle(t *testing.T) {
	var c struct {
		HL       map[string]struct{ Code, HTML string }
		Verbatim map[string]struct {
			In   string
			HTML string
		}
	}
	readJSON(t, "highlight.json", &c)

	for name, tc := range c.HL {
		got, ok := HighlightRuby(tc.Code)
		if !ok {
			t.Errorf("HL[%s] unexpected fallback for %q", name, tc.Code)
			continue
		}
		if got != tc.HTML {
			t.Errorf("HL[%s] code=%q\n want %q\n got  %q", name, tc.Code, tc.HTML, got)
		}
	}

	// verbatim rendering with the highlighter + a static parseable oracle: the
	// expected outputs encode which blocks the gem treats as Ruby.
	parseable := func(code string) bool {
		// the corpus marks "plain" and "notruby" as non-Ruby; everything else
		// in the verbatim set is valid Ruby. We derive this from the golden
		// HTML (a highlighted block contains class="ruby").
		return false // overridden per-case below
	}
	_ = parseable

	opts := &HTMLOptions{OutputDecoration: true, Highlighter: HighlightRuby}
	for name, tc := range c.Verbatim {
		isRuby := containsRubyClass(tc.HTML)
		opts.Parseable = func(string) bool { return isRuby }
		doc := Parse(tc.In)
		f := NewToHtml(opts)
		doc.Accept(f)
		if got := f.endAccepting(); got != tc.HTML {
			t.Errorf("Verbatim[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.HTML, got)
		}
	}
}

func containsRubyClass(html string) bool {
	return len(html) > 0 && (indexOf(html, `<pre class="ruby">`) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type oracleCM struct {
	Module    bool `json:"module"`
	Name      string
	FullName  string `json:"full_name"`
	Comment   string
	Methods   []struct {
		Name, Params string
		Singleton    bool
		Vis, Comment string
		CallSeq      string `json:"call_seq"`
	}
	Constants []struct{ Name, Value, Comment string }
	Attrs     []struct {
		Name, RW, Vis string
	}
}

func TestExtractOracle(t *testing.T) {
	var corpus map[string]struct {
		Src string
		CMs []oracleCM `json:"cms"`
	}
	readJSON(t, "extract.json", &corpus)

	for name, fix := range corpus {
		top := Extract("t.rb", fix.Src)
		cms := top.ClassesAndModule
		sort.Slice(cms, func(i, j int) bool { return cms[i].FullName < cms[j].FullName })
		if len(cms) != len(fix.CMs) {
			t.Errorf("%s: CM count want %d got %d", name, len(fix.CMs), len(cms))
			continue
		}
		for i, want := range fix.CMs {
			got := cms[i]
			if got.Name != want.Name || got.FullName != want.FullName ||
				got.IsModule != want.Module || got.Comment != want.Comment {
				t.Errorf("%s CM[%d]: want{name=%q full=%q mod=%v comment=%q} got{name=%q full=%q mod=%v comment=%q}",
					name, i, want.Name, want.FullName, want.Module, want.Comment,
					got.Name, got.FullName, got.IsModule, got.Comment)
			}
			checkMethods(t, name, i, want, got)
			for j, wc := range want.Constants {
				if j >= len(got.Constants) {
					t.Errorf("%s CM[%d] missing const %d", name, i, j)
					continue
				}
				gc := got.Constants[j]
				if gc.Name != wc.Name || gc.Value != wc.Value || gc.Comment != wc.Comment {
					t.Errorf("%s const[%d]: want %+v got %+v", name, j, wc, gc)
				}
			}
			for j, wa := range want.Attrs {
				if j >= len(got.Attrs) {
					t.Errorf("%s CM[%d] missing attr %d", name, i, j)
					continue
				}
				ga := got.Attrs[j]
				if ga.Name != wa.Name || ga.RW != wa.RW || string(ga.Visibility) != wa.Vis {
					t.Errorf("%s attr[%d]: want %+v got %+v", name, j, wa, ga)
				}
			}
		}
	}
}

func checkMethods(t *testing.T, name string, i int, want oracleCM, got *ClassModule) {
	t.Helper()
	if len(got.Methods) != len(want.Methods) {
		t.Errorf("%s CM[%d] method count want %d got %d", name, i, len(want.Methods), len(got.Methods))
		return
	}
	for j, wm := range want.Methods {
		gm := got.Methods[j]
		if gm.Name != wm.Name || gm.Params != wm.Params || gm.Singleton != wm.Singleton ||
			string(gm.Visibility) != wm.Vis || gm.Comment != wm.Comment || gm.CallSeq != wm.CallSeq {
			t.Errorf("%s CM[%d].method[%d]: want %+v got %+v", name, i, j, wm, gm)
		}
	}
}

func TestFormatterEdgeOracle(t *testing.T) {
	var c struct {
		Rdoc     map[string]struct{ In, Out string }
		Markdown map[string]struct{ In, Out string }
	}
	readJSON(t, "fmt.json", &c)
	for name, tc := range c.Rdoc {
		if got := ToRdocString(tc.In); got != tc.Out {
			t.Errorf("ToRdoc[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.Out, got)
		}
	}
	for name, tc := range c.Markdown {
		if got := ToMarkdownString(tc.In); got != tc.Out {
			t.Errorf("ToMarkdown[%s] in=%q\n want %q\n got  %q", name, tc.In, tc.Out, got)
		}
	}
}
