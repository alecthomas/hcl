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

// AST for HCL.
type AST struct {
	Pos lexer.Position

	Entries []*Entry `@@*`
}

func (a *AST) String() string {
	out := []string{}
	for _, e := range a.Entries {
		out = append(out, e.String())
	}
	return strings.Join(out, "\n")
}

type Entry struct {
	Pos lexer.Position

	Comment string `@Comment?`

	Attribute *Attribute `(   @@`
	Block     *Block     `  | @@ )`
}

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

func (e *Entry) String() string {
	switch {
	case e.Attribute != nil:
		return e.Attribute.String()

	case e.Block != nil:
		return e.Block.String()

	default:
		panic("???")
	}
}

type Attribute struct {
	Pos lexer.Position

	Key   string `@Ident "="`
	Value *Value `@@`
}

func (a *Attribute) String() string {
	return fmt.Sprintf("%s = %s", a.Key, a.Value)
}

type Block struct {
	Pos lexer.Position

	Name   string   `@Ident`
	Labels []string `@( Ident | String )*`
	Body   []*Entry `"{" @@* "}"`
}

func (b *Block) String() string {
	w := &strings.Builder{}
	fmt.Fprintf(w, "%s ", b.Name)
	for _, label := range b.Labels {
		fmt.Fprintf(w, "%q ", label)
	}
	fmt.Fprintln(w, "{")
	for _, e := range b.Body {
		fmt.Fprintln(w, e)
	}
	fmt.Fprintln(w, "}")
	return w.String()
}

type MapEntry struct {
	Pos lexer.Position

	Comment string `@Comment?`

	Key   string `@(Ident | String) ":"`
	Value *Value `@@`
}

type Bool bool

func (b *Bool) Capture(values []string) error { *b = values[0] == "true"; return nil }

type Value struct {
	Pos lexer.Position

	Comment string `@Comment?`

	Bool *Bool       `(  @("true" | "false")`
	Num  *float64    ` | @Number`
	Str  *string     ` | @String`
	List []*Value    ` | "[" ( @@ ( "," @@ )* )? ","? "]"`
	Map  []*MapEntry ` | "{" ( @@ ( "," @@ )* ","? )? "}" )`
}

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
		Comment = //.*|/\*.*?\*/

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
