package rdoc

// This file defines the RDoc code-object model and a focused reader of Ruby
// source into it. It mirrors the subset of RDoc::TopLevel / RDoc::ClassModule /
// RDoc::AnyMethod / RDoc::Constant / RDoc::Attr / RDoc::Alias that captures
// class/module/method/constant/attribute declarations together with their
// attached documentation comments, visibility, parameters and call-seq.
//
// It is intentionally a focused line/block reader rather than a full Ruby
// parser: it recognises the common declaration forms RDoc documents. Source
// that uses metaprogramming to define methods is out of scope (as it is for
// RDoc's static parser without :method: directives).

// Visibility mirrors RDoc method/attribute visibility.
type Visibility string

const (
	// Public visibility (the default).
	Public Visibility = "public"
	// Private visibility (after a bare `private`).
	Private Visibility = "private"
	// Protected visibility (after a bare `protected`).
	Protected Visibility = "protected"
)

// TopLevel is the root code object for one source file. Mirrors
// RDoc::TopLevel.
type TopLevel struct {
	Name             string
	ClassesAndModule []*ClassModule
}

// ClassModule is a class or module declaration with its members. Mirrors
// RDoc::ClassModule (NormalClass / NormalModule).
type ClassModule struct {
	// IsModule distinguishes `module` from `class`.
	IsModule bool
	// Name is the simple (last-segment) name as written, e.g. "Bar" in
	// "class Foo::Bar".
	Name string
	// FullName is the qualified name including the lexical nesting.
	FullName string
	// Superclass is the parent class name for "class X < Y" (empty otherwise).
	Superclass string
	// Comment is the attached documentation comment (RDoc markup source).
	Comment string
	Methods   []*AnyMethod
	Constants []*Constant
	Attrs     []*Attr
	Aliases   []*Alias
	// Nested holds classes and modules declared inside this one. Qualified
	// declarations like "class A::B" synthesize the intermediate module A and
	// nest B inside it, mirroring RDoc's namespace expansion.
	Nested []*ClassModule
}

// findNested returns the direct child class/module with the given simple name,
// or nil.
func (cm *ClassModule) findNested(name string) *ClassModule {
	for _, c := range cm.Nested {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// AnyMethod is a method declaration. Mirrors RDoc::AnyMethod.
type AnyMethod struct {
	Name       string
	Params     string // parameter list including parentheses, e.g. "(a, b)"
	Singleton  bool   // true for "def self.x" / "def Klass.x"
	Visibility Visibility
	Comment    string
	// CallSeq is the contents of a "call-seq:" directive in the comment, if any.
	CallSeq string
}

// Constant is a constant assignment. Mirrors RDoc::Constant.
type Constant struct {
	Name    string
	Value   string
	Comment string
}

// Attr is an attr_reader/writer/accessor declaration. Mirrors RDoc::Attr.
type Attr struct {
	Name       string
	RW         string // "R", "W" or "RW"
	Visibility Visibility
	Comment    string
}

// Alias is an alias_method / alias declaration. Mirrors RDoc::Alias.
type Alias struct {
	NewName string
	OldName string
	Comment string
}
