// Package rdoc is a pure-Go (CGO=0), MRI-faithful port of the core of Ruby's
// RDoc documentation tool (the "rdoc" gem): the markup parser, the inline
// attribute manager, the document model and the HTML formatter, plus a focused
// reader of Ruby source comments into the RDoc code-object model.
//
// The implementation mirrors RDoc::Markup::Parser, RDoc::Markup::AttributeManager,
// RDoc::Markup::Document and RDoc::Markup::ToHtml from rdoc 7.x. The goal is
// byte-for-byte parity with the gem on the markup -> HTML path for the markup
// constructs the gem supports.
//
// The file-system walking and on-disk "darkfish" HTML-site template generation
// of the real gem are deliberately out of scope: they are a thin host seam.
// Everything here is pure, deterministic, in-memory computation.
package rdoc
