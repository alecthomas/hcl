// Package hcl implements parsing, encoding and decoding of HCL from Go types.
//
// Its purpose is to provide idiomatic Go functions and types for HCL.
package hcl

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
)

type Node interface{ node() }

// AST for HCL.
type AST struct {
	Pos lexer.Position

	Entries []*Entry `@@*`
}

func (*AST) node() {}

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

type Attribute struct {
	Pos lexer.Position

	Key   string `@Ident "="`
	Value *Value `@@`
}

func (*Attribute) node() {}

func (a *Attribute) String() string {
	return fmt.Sprintf("%s = %s", a.Key, a.Value)
}

type Block struct {
	Pos lexer.Position

	Name   string   `@Ident`
	Labels []string `@( Ident | String )*`
	Body   []*Entry `"{" @@* "}"`
}

func (*Block) node() {}

type MapEntry struct {
	Pos lexer.Position

	Comments []string `@Comment*`

	Key   string `@(Ident | String) ":"`
	Value *Value `@@`
}

func (*MapEntry) node() {}

type Bool bool

func (b *Bool) Capture(values []string) error { *b = values[0] == "true"; return nil }

type Value struct {
	Pos lexer.Position

	Comments []string `@Comment*`

	Bool *Bool       `(  @("true" | "false")`
	Num  *float64    ` | @Number`
	Str  *string     ` | @String`
	List []*Value    ` | "[" ( @@ ( "," @@ )* )? ","? "]"`
	Map  []*MapEntry ` | "{" ( @@ ( "," @@ )* ","? )? "}" )`
}

func (*Value) node() {}

func (v *Value) String() string {
	switch {
	case v.Bool != nil:
		return fmt.Sprintf("%v", *v.Bool)
	case v.Num != nil:
		return fmt.Sprintf("%g", *v.Num)
	case v.Str != nil:
		return fmt.Sprintf("%q", *v.Str)
	case v.List != nil:
		entries := []string{}
		for _, e := range v.List {
			entries = append(entries, e.String())
		}
		return fmt.Sprintf("[%s]", strings.Join(entries, ", "))
	case v.Map != nil:
		entries := []string{}
		for _, e := range v.Map {
			entries = append(entries, fmt.Sprintf("%q: %s", e.Key, e.Value))
		}
		return fmt.Sprintf("{%s}", strings.Join(entries, ", "))
	default:
		panic("??")
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
	parser = participle.MustBuild(&AST{}, participle.Lexer(lex), participle.Unquote())
)

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
