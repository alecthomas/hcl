// Package hcl implements parsing, encoding and decoding of HCL from Go types.
//
// Its purpose is to provide idiomatic Go functions and types for HCL.
package hcl

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// Position in source file.
type Position = lexer.Position

// Node is the the interface implemented by all AST nodes.
type Node interface {
	Position() Position
	Detach() bool
	children() (children []Node)
}

// Entries in the root of the AST or a Block.
type Entries []Entry

func (e Entries) MarshalJSON() ([]byte, error) {
	out := make([]json.RawMessage, 0, len(e))
	for _, entry := range e {
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		var kind string
		switch entry.(type) {
		case *Attribute:
			kind = "attribute"
		case *Block:
			kind = "block"
		}
		out = append(out, []byte(fmt.Sprintf(`{%q: %s}`, kind, raw)))
	}
	return json.Marshal(out)
}

// AST for HCL.
type AST struct {
	Pos lexer.Position `parser:""`

	Entries          Entries  `parser:"@@*"`
	TrailingComments []string `parser:"@Comment*"`
	Schema           bool     `parser:""`
}

func (a *AST) Detach() bool { return false }

// Clone the AST.
func (a *AST) Clone() *AST {
	if a == nil {
		return nil
	}
	out := &AST{
		Pos:              a.Pos,
		TrailingComments: cloneStrings(a.TrailingComments),
		Schema:           a.Schema,
	}
	out.Entries = make(Entries, len(a.Entries))
	for i, entry := range a.Entries {
		out.Entries[i] = entry.Clone()
	}
	addParentRefs(nil, out)
	return out
}

func (a *AST) Position() Position { return a.Pos }

func (a *AST) children() (children []Node) {
	for _, entry := range a.Entries {
		children = append(children, entry)
	}
	return
}

// Entry at the top-level of a HCL file or block.
type Entry interface {
	Detach() bool
	Clone() Entry
	EntryKey() string
	Node
}

// RecursiveEntry is an Entry representing that a schema is recursive.
type RecursiveEntry struct{}

func (*RecursiveEntry) Position() Position          { return Position{} }
func (*RecursiveEntry) children() (children []Node) { return nil }
func (*RecursiveEntry) Clone() Entry                { return &RecursiveEntry{} }
func (*RecursiveEntry) Detach() bool                { return false }
func (*RecursiveEntry) EntryKey() string            { panic("unimplemented") }

var _ Entry = &RecursiveEntry{}

// Attribute is a key=value attribute.
type Attribute struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Comments []string `parser:"@Comment*"`

	Key   string `parser:"@Ident"`
	Value Value  `parser:"( '=':Punct @@ )?"`

	Default  Value   `parser:"( '(' ( (  'default' '(' @@ ')'"`
	Enum     []Value `parser:"         | 'enum' '(' @@ (',' @@)* ')'"`
	Optional bool    `parser:"         | @'optional' ) )+ ')' )?"`
}

var _ Entry = &Attribute{}

func (a *Attribute) Detach() bool       { return detachEntry(a.Parent, a) }
func (a *Attribute) Position() Position { return a.Pos }
func (a *Attribute) EntryKey() string   { return a.Key }
func (a *Attribute) children() (children []Node) {
	return []Node{a.Value, a.Default}
}
func (a *Attribute) String() string {
	return fmt.Sprintf("%s = %s", a.Key, a.Value)
}

// Clone the AST.
func (a *Attribute) Clone() Entry {
	if a == nil {
		return nil
	}
	return &Attribute{
		Pos:      a.Pos,
		Comments: cloneStrings(a.Comments),
		Key:      a.Key,
		Value:    a.Value.Clone(),
		Optional: a.Optional,
	}
}

// Block represents am optionally labelled HCL block.
type Block struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Comments []string `parser:"@Comment*"`

	Name     string   `parser:"@Ident"`
	Repeated bool     `parser:"( '(' @'repeated' ')' )?"`
	Labels   []string `parser:"@( Ident | String )*"`
	Body     Entries  `parser:"'{' @@*"`

	TrailingComments []string `parser:"@Comment* '}'"`
}

var _ Entry = &Block{}

func (b *Block) Position() Position { return b.Pos }

// EntryKey implements Entry
func (b *Block) EntryKey() string { return b.Name }

// Detach Block from parent.
func (b *Block) Detach() bool {
	return detachEntry(b.Parent, b)
}

func (b *Block) children() (children []Node) {
	for _, entry := range b.Body {
		children = append(children, entry)
	}
	return
}

// Clone the AST.
func (b *Block) Clone() Entry {
	if b == nil {
		return nil
	}
	out := &Block{
		Pos:              b.Pos,
		Comments:         cloneStrings(b.Comments),
		Name:             b.Name,
		Labels:           cloneStrings(b.Labels),
		Body:             make(Entries, len(b.Body)),
		TrailingComments: cloneStrings(b.TrailingComments),
		Repeated:         b.Repeated,
	}
	for i, entry := range b.Body {
		out.Body[i] = entry.Clone()
	}
	return out
}

// MapEntry represents a key+value in a map.
type MapEntry struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Comments []string `parser:"@Comment*"`

	Key   Value `parser:"@@ ':'"`
	Value Value `parser:"@@"`
}

func (e *MapEntry) Detach() bool {
	value, ok := e.Parent.(*Map)
	if !ok {
		return false
	}
	for i, seek := range value.Entries {
		if seek == e {
			value.Entries = append(value.Entries[:i], value.Entries[i+1:]...)
			return true
		}
	}
	return false
}

func (e *MapEntry) Position() Position { return e.Pos }

func (e *MapEntry) children() (children []Node) {
	return []Node{e.Key, e.Value}
}

// Clone the AST.
func (e *MapEntry) Clone() *MapEntry {
	if e == nil {
		return nil
	}
	return &MapEntry{
		Pos:      e.Pos,
		Key:      e.Key.Clone(),
		Value:    e.Value.Clone(),
		Comments: cloneStrings(e.Comments),
	}
}

// Bool represents a parsed boolean value.
type Bool struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Bool bool `parser:"@'true':Ident | 'false':Ident"`
}

var _ Value = &Bool{}

func (b *Bool) Detach() bool                { return false }
func (b *Bool) Position() lexer.Position    { return b.Pos }
func (b *Bool) children() (children []Node) { return nil }
func (b *Bool) Clone() Value                { clone := *b; return &clone }
func (b *Bool) String() string              { return strconv.FormatBool(b.Bool) }
func (b *Bool) value()                      {}

func (b *Bool) Capture(values []string) error { b.Bool = values[0] == "true"; return nil } // nolint: golint

var needsOctalPrefix = regexp.MustCompile(`^0\d+$`)

// Number of arbitrary precision.
type Number struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Float *big.Float `parser:"@Number"`
}

var _ Value = &Number{}

func (n *Number) Detach() bool                { return false }
func (n *Number) Position() lexer.Position    { return n.Pos }
func (n *Number) children() (children []Node) { return nil }
func (n *Number) Clone() Value {
	clone := *n
	clone.Float.Copy(n.Float)
	return &clone
}
func (n *Number) value() {}

func (n *Number) String() string   { return n.Float.String() }
func (n *Number) GoString() string { return n.String() }

// Parse override because big.Float doesn't directly support 0-prefix octal parsing... why?
func (n *Number) Parse(lex *lexer.PeekingLexer) error {
	token := lex.Peek()
	if token.Type != numberType {
		return participle.NextMatch
	}
	token = lex.Next()
	value := token.Value
	if needsOctalPrefix.MatchString(value) {
		value = "0o" + value[1:]
	}
	n.Float = big.NewFloat(0)
	_, _, err := n.Float.Parse(value, 0)
	return err
}

// Value represents a terminal value, either scalar or a map or list.
type Value interface {
	value()
	Clone() Value
	String() string
	Node
}

// Type of a Value.
type Type struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Ident string `parser:"@('string':Ident | 'number':Ident | 'boolean':Ident)"`
}

var _ Value = &Type{}

func (t *Type) value()                      {}
func (t *Type) Clone() Value                { clone := *t; return &clone }
func (t *Type) String() string              { return t.Ident }
func (t *Type) Detach() bool                { return false }
func (t *Type) Position() lexer.Position    { return t.Pos }
func (t *Type) children() (children []Node) { return nil }

// Call represents a function call.
type Call struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Args []Value `parser:"'(' @@ ( ',' @@ )* ')'"`
}

var _ Node = &Call{}

func (f *Call) Clone() *Call {
	if f == nil {
		return nil
	}
	clone := *f
	clone.Args = make([]Value, len(f.Args))
	for i, arg := range clone.Args {
		clone.Args[i] = arg.Clone()
	}
	return &clone
}
func (f *Call) String() string {
	args := make([]string, 0, len(f.Args))
	for i, arg := range f.Args {
		args[i] = arg.String()
	}
	return fmt.Sprintf("(%s)", strings.Join(args, ", "))
}
func (f *Call) Detach() bool             { return false }
func (f *Call) Position() lexer.Position { return f.Pos }
func (f *Call) children() (children []Node) {
	out := make([]Node, len(f.Args))
	for i, arg := range f.Args {
		out[i] = arg
	}
	return out
}

// String literal.
type String struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Str string `parser:"@(String | Ident)"`
}

var _ Value = &String{}

func (s *String) Clone() Value                { clone := *s; return &clone }
func (s *String) String() string              { return strconv.Quote(s.Str) }
func (s *String) Detach() bool                { return false }
func (s *String) Position() lexer.Position    { return s.Pos }
func (s *String) children() (children []Node) { return nil }
func (s *String) value()                      {}

// Heredoc represents a heredoc string.
type Heredoc struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Delimiter string `parser:"(@Heredoc"`
	Doc       string `parser:" @(Body | EOL)* End)"`
}

var _ Value = &Heredoc{}

func (h *Heredoc) Clone() Value { clone := *h; return &clone }
func (h *Heredoc) String() string {
	return fmt.Sprintf("<<%s%s\n%s", h.Delimiter, h.Doc, strings.TrimPrefix(h.Delimiter, "-"))
}
func (h *Heredoc) value()                      {}
func (h *Heredoc) Detach() bool                { return false }
func (h *Heredoc) Position() lexer.Position    { return h.Pos }
func (h *Heredoc) children() (children []Node) { return nil }

// GetHeredoc gets the heredoc as a string.
//
// This will correctly format indented heredocs.
func (h *Heredoc) GetHeredoc() string {
	heredoc := h.Doc[1:] // Removes a lexing artefact.
	if h.Delimiter[0] != '-' {
		return heredoc
	}
	return dedent(heredoc)
}

// A List of values.
type List struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	List []Value `parser:"( '[' ( @@ ( ',' @@ )* )? ','? ']' )"`
}

func (l *List) Clone() Value {
	out := *l
	for i, value := range l.List {
		out.List[i] = value.Clone()
	}
	return &out
}

func (l *List) String() string {
	out := strings.Builder{}
	out.WriteRune('[')
	for i, e := range l.List {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(e.String())
	}
	out.WriteRune(']')
	return out.String()
}

var _ Value = &List{}

func (l *List) Detach() bool                { return false }
func (l *List) Position() lexer.Position    { return l.Pos }
func (l *List) children() (children []Node) { return nil }
func (l *List) value()                      {}

// A Map of key to value.
type Map struct {
	Pos    lexer.Position `parser:""`
	Parent Node           `parser:""`

	Entries []*MapEntry `parser:"( '{' ( @@ ( ',' @@ )* ','? )? '}' )"`
}

func (m *Map) Clone() Value {
	out := *m
	for i, entry := range m.Entries {
		out.Entries[i] = entry.Clone()
	}
	return &out
}
func (m *Map) String() string {
	out := &strings.Builder{}
	out.WriteRune('{')
	for i, e := range m.Entries {
		if i > 0 {
			out.WriteString(", ")
		}
		fmt.Fprintf(out, "%s: %s", e.Key, e.Value)
	}
	out.WriteRune('}')
	return out.String()
}

var _ Value = &Map{}

func (m *Map) Detach() bool             { return false }
func (m *Map) Position() lexer.Position { return m.Pos }
func (m *Map) children() (children []Node) {
	for _, entry := range m.Entries {
		children = append(children, entry)
	}
	return
}
func (m *Map) value() {}

var (
	lex = lexer.Must(lexer.New(lexer.Rules{
		"Root": {
			{"Ident", `\b[[:alpha:]]\w*(-\w+)*\b`, nil},
			{"Number", `^[-+]?[0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?`, nil},
			{"Heredoc", `<<[-]?(\w+\b)`, lexer.Push("Heredoc")},
			{"String", `"(\\\d\d\d|\\.|[^"])*"|'(\\\d\d\d|\\.|[^'])*'`, nil},
			{"Punct", `[][*?{}=:,()|]`, nil},
			{"Comment", `(?:(?://|#)[^\n]*)|/\*.*?\*/`, nil},
			{"Whitespace", `\s+`, nil},
		},
		"Heredoc": {
			{"End", `\n\s*\b\1\b`, lexer.Pop()},
			{"EOL", `\n`, nil},
			{"Body", `[^\n]+`, nil},
		},
	}))
	numberType = lex.Symbols()["Number"]
	parser     = participle.MustBuild[AST](
		participle.Lexer(lex),
		participle.Map(unquoteString, "String"),
		participle.Map(cleanHeredocStart, "Heredoc"),
		participle.Map(stripComment, "Comment"),
		participle.Elide("Whitespace"),
		participle.Union[Entry](&Block{}, &Attribute{}),
		participle.Union[Value](&Bool{}, &Type{}, &String{}, &Number{}, &List{}, &Map{}, &Heredoc{}),
		// We need lookahead to ensure prefixed comments are associated with the right nodes.
		participle.UseLookahead(50))
)

var stripCommentRe = regexp.MustCompile(`^//\s*|^/\*|\*/$`)

func stripComment(token lexer.Token) (lexer.Token, error) {
	token.Value = stripCommentRe.ReplaceAllString(token.Value, "")
	return token, nil
}

func unquoteString(token lexer.Token) (lexer.Token, error) {
	if token.Value[0] == '\'' {
		token.Value = "\"" + strings.ReplaceAll(token.Value[1:len(token.Value)-1], "\"", "\\\"") + "\""
	}
	var err error
	token.Value, err = strconv.Unquote(token.Value)
	if err != nil {
		return token, fmt.Errorf("%s: %w", token.Pos, err)
	}
	return token, nil
}

// <<EOF -> EOF
func cleanHeredocStart(token lexer.Token) (lexer.Token, error) {
	token.Value = token.Value[2:]
	return token, nil
}

// Parse HCL from an io.Reader.
func Parse(r io.Reader) (*AST, error) {
	hcl, err := parser.Parse("", r)
	if err != nil {
		return nil, err
	}
	return hcl, AddParentRefs(hcl)
}

// ParseString parses HCL from a string.
func ParseString(str string) (*AST, error) {
	hcl, err := parser.ParseString("", str)
	if err != nil {
		return nil, err
	}
	return hcl, AddParentRefs(hcl)
}

// ParseBytes parses HCL from bytes.
func ParseBytes(data []byte) (*AST, error) {
	hcl, err := parser.ParseBytes("", data)
	if err != nil {
		return nil, err
	}
	return hcl, AddParentRefs(hcl)
}

func cloneStrings(strings []string) []string {
	if strings == nil {
		return nil
	}
	out := make([]string, len(strings))
	copy(out, strings)
	return out
}

func detachEntry(parent Node, entry Entry) bool {
	var entries *Entries
	switch node := parent.(type) {
	case *Block:
		entries = &node.Body
	case *AST:
		entries = &node.Entries
	}
	if entries == nil {
		return false
	}
	for i, e := range *entries {
		if e == entry {
			*entries = append((*entries)[:i], (*entries)[i+1:]...)
			return true
		}
	}
	return false
}
