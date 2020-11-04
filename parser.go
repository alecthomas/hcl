// Package hcl implements parsing, encoding and decoding of HCL from Go types.
//
// Its purpose is to provide idiomatic Go functions and types for HCL.
package hcl

import (
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/stateful"
	"github.com/alecthomas/repr"
)

// Node is the the interface implemented by all AST nodes.
type Node interface{ node() }

// AST for HCL.
type AST struct {
	Pos lexer.Position `parser:"" json:"-"`

	Entries          []*Entry `parser:"@@*" json:"entries,omitempty"`
	TrailingComments []string `parser:"@Comment*" json:"trailing_comments,omitempty"`
	Schema           bool     `parser:"" json:"schema,omitempty"`
}

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
	out.Entries = make([]*Entry, len(a.Entries))
	for i, entry := range a.Entries {
		out.Entries[i] = entry.Clone()
	}
	addParentRefs(nil, out)
	return out
}

func (*AST) node() {}

// Entry at the top-level of a HCL file or block.
type Entry struct {
	Pos    lexer.Position `parser:"" json:"-"`
	Parent Node           `parser:"" json:"-"`

	Attribute *Attribute `parser:"(   @@" json:"attribute,omitempty"`
	Block     *Block     `parser:"  | @@ )" json:"block,omitempty"`
}

func (*Entry) node() {}

// Key of the attribute or block.
func (e *Entry) Key() string {
	switch {
	case e.Attribute != nil:
		return e.Attribute.Key

	case e.Block != nil:
		return e.Block.Name

	default:
		panic("???")
	}
}

// Clone the AST.
func (e *Entry) Clone() *Entry {
	if e == nil {
		return nil
	}
	return &Entry{
		Pos:       e.Pos,
		Attribute: e.Attribute.Clone(),
		Block:     e.Block.Clone(),
	}
}

// Attribute is a key+value attribute.
type Attribute struct {
	Pos    lexer.Position `parser:"" json:"-"`
	Parent Node           `parser:"" json:"-"`

	Comments []string `parser:"@Comment*" json:"comments,omitempty"`

	Key   string `parser:"@Ident '='" json:"key"`
	Value *Value `parser:"@@" json:"value"`

	// This will be populated during unmarshalling.
	Default *Value `parser:"" json:"default,omitempty"`

	// This will be parsed from the enum tag and will be helping the validation during unmarshalling
	Enum []*Value `parser:"" json:"enum,omitempty"`

	// Set for schemas when the attribute is optional.
	Optional bool `parser:"" json:"optional,omitempty"`
}

func (*Attribute) node() {}

func (a *Attribute) String() string {
	return fmt.Sprintf("%s = %s", a.Key, a.Value)
}

// Clone the AST.
func (a *Attribute) Clone() *Attribute {
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
	Pos    lexer.Position `parser:"" json:"-"`
	Parent Node           `parser:"" json:"-"`

	Comments []string `parser:"@Comment*" json:"comments,omitempty"`

	Name   string   `parser:"@Ident" json:"name"`
	Labels []string `parser:"@( Ident | String )*" json:"labels,omitempty"`
	Body   []*Entry `parser:"'{' @@*" json:"body"`

	TrailingComments []string `parser:"@Comment* '}'" json:"trailing_comments,omitempty"`

	// The block can be repeated. This is surfaced in schemas.
	Repeated bool `parser:"" json:"repeated,omitempty"`
}

func (*Block) node() {}

// Clone the AST.
func (b *Block) Clone() *Block {
	if b == nil {
		return nil
	}
	out := &Block{
		Pos:              b.Pos,
		Comments:         cloneStrings(b.Comments),
		Name:             b.Name,
		Labels:           cloneStrings(b.Labels),
		Body:             make([]*Entry, len(b.Body)),
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
	Pos    lexer.Position `parser:"" json:"-"`
	Parent Node           `parser:"" json:"-"`

	Comments []string `parser:"@Comment*" json:"comments,omitempty"`

	Key   *Value `parser:"@@ ':'" json:"key"`
	Value *Value `parser:"@@" json:"value"`
}

func (*MapEntry) node() {}

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
type Bool bool

func (b *Bool) Capture(values []string) error { *b = values[0] == "true"; return nil } // nolint: golint

// Value is a scalar, list or map.
type Value struct {
	Pos    lexer.Position `parser:"" json:"-"`
	Parent Node           `parser:"" json:"-"`

	Bool             *Bool       `parser:"(  @('true' | 'false')" json:"bool,omitempty"`
	Number           *big.Float  `parser:" | @Number" json:"number,omitempty"`
	Type             *string     `parser:" | @('number':Ident | 'string':Ident | 'boolean':Ident)" json:"type,omitempty"`
	Str              *string     `parser:" | @(String | Ident)" json:"str,omitempty"`
	HeredocDelimiter string      `parser:" | (@Heredoc" json:"heredoc_delimiter,omitempty"`
	Heredoc          *string     `parser:"     @(Body | EOL)* End)" json:"heredoc,omitempty"`
	HaveList         bool        `parser:" | ( @'['" json:"have_list,omitempty"` // Need this to detect empty lists.
	List             []*Value    `parser:"     ( @@ ( ',' @@ )* )? ','? ']' )" json:"list,omitempty"`
	HaveMap          bool        `parser:" | ( @'{'" json:"have_map,omitempty"` // Need this to detect empty maps.
	Map              []*MapEntry `parser:"     ( @@ ( ',' @@ )* ','? )? '}' ) )" json:"map,omitempty"`
}

// Clone the AST.
func (v *Value) Clone() *Value {
	if v == nil {
		return nil
	}
	out := &Value{}
	*out = *v
	switch {
	case out.Number != nil:
		out.Number = &big.Float{}
		out.Number.Copy(v.Number)

	case v.HaveList:
		out.List = make([]*Value, len(v.List))
		for i, value := range v.List {
			out.List[i] = value.Clone()
		}

	case v.HaveMap:
		out.Map = make([]*MapEntry, len(v.Map))
		for i, entry := range out.Map {
			out.Map[i] = entry.Clone()
		}
	}
	return out
}

func (*Value) node() {}

func (v *Value) String() string {
	switch {
	case v.Bool != nil:
		return fmt.Sprintf("%v", *v.Bool)

	case v.Number != nil:
		return v.Number.String()

	case v.Str != nil:
		return fmt.Sprintf("%q", *v.Str)

	case v.HeredocDelimiter != "":
		heredoc := ""
		if v.Heredoc != nil {
			heredoc = *v.Heredoc
		}
		return fmt.Sprintf("<<%s%s\n%s", v.HeredocDelimiter, heredoc, v.HeredocDelimiter)

	case v.HaveList:
		entries := []string{}
		for _, e := range v.List {
			entries = append(entries, e.String())
		}
		return fmt.Sprintf("[%s]", strings.Join(entries, ", "))

	case v.HaveMap:
		entries := []string{}
		for _, e := range v.Map {
			entries = append(entries, fmt.Sprintf("%s: %s", e.Key, e.Value))
		}
		return fmt.Sprintf("{%s}", strings.Join(entries, ", "))

	case v.Type != nil:
		return fmt.Sprintf("%s", *v.Type)

	default:
		panic(repr.String(v, repr.Hide(lexer.Position{})))
	}
}

// GetHeredoc gets the heredoc as a string.
//
// This will correctly format indented heredocs.
func (v *Value) GetHeredoc() string {
	if v == nil {
		return ""
	}
	heredoc := ""
	if v.Heredoc != nil {
		// The [1:] here removes a \n lexing artifact.
		heredoc = (*v.Heredoc)[1:]
	}
	if v.HeredocDelimiter[0] != '-' {
		return heredoc
	}
	return dedent(heredoc)
}

var (
	lex = lexer.Must(stateful.New(stateful.Rules{
		"Root": {
			{"Ident", `\b[[:alpha:]]\w*(-\w+)*\b`, nil},
			{"Number", `\b^[-+]?[0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?\b`, nil},
			{"Heredoc", `<<[-]?(\w+\b)`, stateful.Push("Heredoc")},
			{"String", `"(\\\d\d\d|\\.|[^"])*"|'(\\\d\d\d|\\.|[^'])*'`, nil},
			{"Punct", `[][{}=:,]`, nil},
			{"Comment", `(?:(?://|#)[^\n]*)|/\*.*?\*/`, nil},
			{"whitespace", `\s+`, nil},
		},
		"Heredoc": {
			{"End", `\n\b\1\b`, stateful.Pop()},
			{"EOL", `\n`, nil},
			{"Body", `[^\n]+`, nil},
		},
	}))
	parser = participle.MustBuild(&AST{},
		participle.Lexer(lex),
		participle.Unquote("String"),
		participle.Map(cleanHeredocStart, "Heredoc"),
		participle.Map(stripComment, "Comment"),
		// We need lookahead to ensure prefixed comments are associated with the right nodes.
		participle.UseLookahead(50))
)

var stripCommentRe = regexp.MustCompile(`^//\s*|^/\*|\*/$`)

func stripComment(token lexer.Token) (lexer.Token, error) {
	token.Value = stripCommentRe.ReplaceAllString(token.Value, "")
	return token, nil
}

// <<EOF -> EOF
func cleanHeredocStart(token lexer.Token) (lexer.Token, error) {
	token.Value = token.Value[2:]
	return token, nil
}

// Parse HCL from an io.Reader.
func Parse(r io.Reader) (*AST, error) {
	hcl := &AST{}
	err := parser.Parse(r, hcl)
	if err != nil {
		return nil, err
	}
	return hcl, AddParentRefs(hcl)
}

// ParseString parses HCL from a string.
func ParseString(str string) (*AST, error) {
	hcl := &AST{}
	err := parser.ParseString(str, hcl)
	if err != nil {
		return nil, err
	}
	return hcl, AddParentRefs(hcl)
}

// ParseBytes parses HCL from bytes.
func ParseBytes(data []byte) (*AST, error) {
	hcl := &AST{}
	err := parser.ParseBytes(data, hcl)
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
