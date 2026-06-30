<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-rdoc/brand/main/social/go-ruby-rdoc-rdoc.png" alt="go-ruby-rdoc/rdoc" width="720"></p>

# rdoc — go-ruby-rdoc

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-rdoc.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the core of Ruby's
[RDoc](https://ruby.github.io/rdoc/) documentation tool** — the markup parser,
the inline attribute manager, the document model and the HTML formatter, plus a
focused reader of Ruby source comments into the RDoc code-object model. It turns
RDoc markup text into the exact HTML the `rdoc` gem emits, and reads
class/module/method/constant/attribute declarations together with their attached
documentation comments — **without any Ruby runtime**.

It is a sibling of [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp)
(the Onigmo engine), [go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB
compiler) and [go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (the Psych
emitter), and is intended as the documentation backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby).

> **What it is — and isn't.** Parsing RDoc markup into the document model and
> rendering it to HTML/Markdown/RDoc is fully deterministic and needs **no
> interpreter**, so it lives here as pure Go with byte-for-byte parity against
> the gem. Two things are host seams: (1) verbatim **Ruby syntax highlighting**,
> because the gem's `parseable?` check evaluates the block with the real Ruby
> parser — a built-in [`HighlightRuby`](#ruby-highlighting) covers the common
> token classes, gated behind an `HTMLOptions.Parseable` callback; and (2) the
> on-disk **darkfish** HTML-site/template generation (file-walking, asset
> copying), which is a thin output-writing layer left to the caller.

## What works

- **`RDoc::Markup::Parser`** — a faithful recursive-descent port producing the
  document model: paragraphs, headings (`= H1` … `====== H6`), bullet / numbered
  / lower-alpha / upper-alpha / label (`[x]`) / note (`x::`) lists (nested),
  indented verbatim/code blocks, rules (`---`), block quotes (`>>>`) and blank
  lines.
- **`RDoc::Markup::AttributeManager`** — the inline markup engine: `*bold*`,
  `_emphasis_`, `+code+`, the `<b>/<i>/<em>/<tt>/<code>` HTML tags, backslash
  escaping, `__word__` protection, hyperlinks (`{text}[url]`, `word[url]`, bare
  URLs, `link:`/`mailto:`/`ftp:`/`www.`), `rdoc-ref:`/`rdoc-label:`/`rdoc-image:`
  links and cross-references.
- **`RDoc::Markup::ToHtml`** — renders the model to the gem's exact HTML, including
  heading anchors (`id="label-…"` via `ToLabel`), the smart-quote/dash/ellipsis/
  ©/® entity pass (`RDoc::Text#to_html`), pipe mode, and cross-reference linking
  (`ToHtmlCrossref`).
- **`RDoc::Markup::ToMarkdown` / `ToRdoc`** — the Markdown and RDoc-markup
  formatters for the common block constructs.
- **Code-object extraction** — reads Ruby source into `TopLevel` /
  `ClassModule` / `AnyMethod` / `Constant` / `Attr` / `Alias` with attached
  comments, visibility (`public`/`private`/`protected`), method parameters,
  `call-seq:`, namespace expansion (`class A::B` → module `A` + class `B`) and
  `=begin`/`=end` block comments.

## Usage

```go
import "github.com/go-ruby-rdoc/rdoc"

html := rdoc.ToHTML("= Title\n\nA *bold* word and a {link}[https://example.com].\n")
// "\n<h1 id=\"label-Title\">…</h1>\n\n<p>A <strong>bold</strong> word …</p>\n"

doc := rdoc.Parse(markup)          // the document model
md  := rdoc.ToMarkdownString(markup)
rd  := rdoc.ToRdocString(markup)

top := rdoc.Extract("foo.rb", source) // the code-object model
for _, cm := range top.ClassesAndModule {
    // cm.Name, cm.Comment, cm.Methods, cm.Constants, cm.Attrs, …
}
```

### Ruby highlighting

Verbatim blocks are emitted as a plain `<pre>` by default (the deterministic,
Ruby-free path). To reproduce the gem's syntax-highlighted `<pre class="ruby">`,
plug in the built-in highlighter and a `Parseable` oracle:

```go
opts := &rdoc.HTMLOptions{
    OutputDecoration: true,
    Highlighter:      rdoc.HighlightRuby,        // built-in token highlighter
    Parseable:        myRubyParseableCheck,      // host seam: is this valid Ruby?
}
f := rdoc.NewToHtml(opts)
doc.Accept(f)
out := f.EndAccepting() // not exported — use rdoc helpers; see godoc
```

## Faithfulness

Parity is enforced by a differential oracle: golden HTML / labels / Markdown /
RDoc / highlighter / extraction expectations captured from the real `rdoc` gem
(RDoc 7.x) live in `testdata/` and are checked on every run. The deterministic,
Ruby-free tests alone hold 100% coverage, so CI passes without a Ruby toolchain;
the lanes that have Ruby additionally re-derive and re-check the same shapes.

## Tests & coverage

```sh
GOWORK=off go test -cover ./...
```

100% statement coverage is enforced in CI across three OSes (Linux/macOS/Windows)
and the six supported 64-bit architectures (amd64, arm64, riscv64, loong64,
ppc64le, s390x).

## License

BSD-3-Clause. Copyright the go-ruby-rdoc/rdoc authors.
