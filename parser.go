// Package hcl implements parsing, encoding and decoding of HCL from Go types.
//
// Its purpose is to provide idiomatic Go functions and types for HCL.
package hcl

import (
	"bytes"
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
	Pos lexer.Position

	Entries []*Entry `@@*`
}

func (a *AST) MarshalJSON() ([]byte, error) {
	m := &jsonVisitor{&bytes.Buffer{}}
	err := Visit(a, m.Visit)
	return m.Bytes(), err
}

func (*AST) node() {}

// Entry at the top-level of a HCL file or block.
type Entry struct {
	Pos lexer.Position

	Comments []string `@Comment*`

	Attribute *Attribute `(   @@`
	Block     *Block     `  | @@ )`
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
	Pos lexer.Position

	Key   string `@Ident "="`
	Value *Value `@@`
}

func (*Attribute) node() {}

func (a *Attribute) String() string {
	return fmt.Sprintf("%s = %s", a.Key, a.Value)
}

// Block represents am optionally labelled HCL block.
type Block struct {
	Pos lexer.Position

	Name   string   `@Ident`
	Labels []string `@( Ident | String )*`
	Body   []*Entry `"{" @@* "}"`
}

func (*Block) node() {}

// MapEntry represents a key+value in a map.
type MapEntry struct {
	Pos lexer.Position

	Comments []string `@Comment*`

	Key   *Value `@@ ":"`
	Value *Value `@@`
}

func (*MapEntry) node() {}

// Bool represents a parsed boolean value.
type Bool bool

func (b *Bool) Capture(values []string) error { *b = values[0] == "true"; return nil } // nolint: golint

// Value is a scalar, list or map.
type Value struct {
	Pos lexer.Position

	Bool     *Bool       `(  @("true" | "false")`
	Number   *big.Float  ` | @Number`
	Type     *string     ` | @("number":Ident | "string":Ident | "boolean":Ident)`
	Str      *string     ` | @(String | Ident)`
	HaveList bool        ` | ( @"["` // Need this to detect empty lists.
	List     []*Value    `     ( @@ ( "," @@ )* )? ","? "]" )`
	HaveMap  bool        ` | ( @"{"` // Need this to detect empty maps.
	Map      []*MapEntry `     ( @@ ( "," @@ )* ","? )? "}" ) )`
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
	parser = participle.MustBuild(&AST{}, participle.Lexer(lex), participle.Unquote(), participle.Map(stripComment, "Comment"))
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
