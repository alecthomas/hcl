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
	"github.com/alecthomas/participle/lexer/regex"
	"github.com/alecthomas/repr"
)

// Node is the the interface implemented by all AST nodes.
type Node interface{ node() }

// AST for HCL.
type AST struct {
	Pos lexer.Position `parser:"" json:"-"`

	Entries []*Entry `parser:"@@*" json:"entries"`

	TrailingComments []string `parser:"@Comment*" json:"trailing_comments,omitempty"`

	Schema bool `parser:"" json:"schema,omitempty"`
}

func (*AST) node() {}

// Entry at the top-level of a HCL file or block.
type Entry struct {
	Pos lexer.Position `parser:"" json:"-"`

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

// Attribute is a key+value attribute.
type Attribute struct {
	Pos lexer.Position `parser:"" json:"-"`

	Comments []string `parser:"@Comment*" json:"comments,omitempty"`

	Key   string `parser:"@Ident '='" json:"key"`
	Value *Value `parser:"@@" json:"value"`

	// Set for schemas when the attribute is optional.
	Optional bool `parser:"" json:"optional,omitempty"`
}

func (*Attribute) node() {}

func (a *Attribute) String() string {
	return fmt.Sprintf("%s = %s", a.Key, a.Value)
}

// Block represents am optionally labelled HCL block.
type Block struct {
	Pos lexer.Position `parser:"" json:"-"`

	Comments []string `parser:"@Comment*" json:"comments,omitempty"`

	Name   string   `parser:"@Ident" json:"name"`
	Labels []string `parser:"@( Ident | String )*" json:"labels,omitempty"`
	Body   []*Entry `parser:"'{' @@*" json:"body"`

	TrailingComments []string `parser:"@Comment* '}'" json:"trailing_comments,omitempty"`

	// The block can be repeated. This is surfaced in schemas.
	Repeated bool `parser:"" json:"repeated,omitempty"`
}

func (*Block) node() {}

// MapEntry represents a key+value in a map.
type MapEntry struct {
	Pos lexer.Position `parser:"" json:"-"`

	Comments []string `parser:"@Comment*" json:"comments,omitempty"`

	Key   *Value `parser:"@@ ':'" json:"key"`
	Value *Value `parser:"@@" json:"value"`
}

func (*MapEntry) node() {}

// Bool represents a parsed boolean value.
type Bool bool

func (b *Bool) Capture(values []string) error { *b = values[0] == "true"; return nil } // nolint: golint

// Value is a scalar, list or map.
type Value struct {
	Pos lexer.Position `parser:"" json:"-"`

	Bool     *Bool       `parser:"(  @('true' | 'false')" json:"bool,omitempty"`
	Number   *big.Float  `parser:" | @Number" json:"number,omitempty"`
	Type     *string     `parser:" | @('number':Ident | 'string':Ident | 'boolean':Ident)" json:"type,omitempty"`
	Str      *string     `parser:" | @(String | Ident)" json:"str,omitempty"`
	HaveList bool        `parser:" | ( @'['" json:"have_list,omitempty"` // Need this to detect empty lists.
	List     []*Value    `parser:"     ( @@ ( ',' @@ )* )? ','? ']' )" json:"list,omitempty"`
	HaveMap  bool        `parser:" | ( @'{'" json:"have_map,omitempty"` // Need this to detect empty maps.
	Map      []*MapEntry `parser:"     ( @@ ( ',' @@ )* ','? )? '}' ) )" json:"map,omitempty"`
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

var (
	lex = lexer.Must(regex.New(`
		Ident = \b[[:alpha:]]\w*(-\w+)*\b
		Number = \b^[-+]?[0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?\b
		String = "(\\\d\d\d|\\.|[^"])*"
		Punct = [][{}=:,]
		Comment = //[^\n]*|/\*.*?\*/

		whitespace = \s+
	`))
	parser = participle.MustBuild(&AST{},
		participle.Lexer(lex),
		participle.Unquote(),
		participle.Map(stripComment, "Comment"),
		// We need lookahead to ensure prefixed comments are associated with the right nodes.
		participle.UseLookahead(50))
)

var stripCommentRe = regexp.MustCompile(`^//\s*|^/\*|\*/$`)

func stripComment(token lexer.Token) (lexer.Token, error) {
	token.Value = stripCommentRe.ReplaceAllString(token.Value, "")
	return token, nil
}

// Parse HCL from an io.Reader.
func Parse(r io.Reader) (*AST, error) {
	hcl := &AST{}
	return hcl, parser.Parse(r, hcl)
}

// ParseString parses HCL from a string.
func ParseString(str string) (*AST, error) {
	hcl := &AST{}
	return hcl, parser.ParseString(str, hcl)
}

// ParseBytes parses HCL from bytes.
func ParseBytes(data []byte) (*AST, error) {
	hcl := &AST{}
	return hcl, parser.ParseBytes(data, hcl)
}
