package hcl

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/repr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHeredoc(t *testing.T) {
	value := &Value{
		HeredocDelimiter: "-EOF",
		Heredoc:          strp("\n    hello\n  world"),
	}
	require.Equal(t, "  hello\nworld", value.GetHeredoc())
	value = &Value{
		HeredocDelimiter: "EOF",
		Heredoc:          strp("\n  hello\n  world"),
	}
	require.Equal(t, "  hello\n  world", value.GetHeredoc())
}

func TestClone(t *testing.T) {
	ast, err := ParseString(complexHCLExample)
	require.NoError(t, err)
	clone := ast.Clone()
	require.Equal(t, ast, clone)
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		hcl      string
		fail     bool
		expected *AST
	}{
		{name: "Heredoc",
			hcl: `
				doc = <<EOF
some thing
or another
EOF
			`,
			expected: &AST{
				Entries: []*Entry{
					attr("doc", heredoc("EOF", "\nsome thing\nor another")),
				},
			},
		},
		{name: "IndentedHeredoc",
			hcl: `
				doc = <<-EOF
some thing
or another
EOF
			`,
			expected: &AST{
				Entries: []*Entry{
					attr("doc", heredoc("-EOF", "\nsome thing\nor another")),
				},
			},
		},
		{name: "EmptyHeredoc",
			hcl: `
				doc = <<EOF
EOF
			`,
			expected: &AST{
				Entries: []*Entry{
					attr("doc", &Value{HeredocDelimiter: "EOF"}),
				},
			},
		},
		{name: "Comments",
			hcl: `
				// A comment
				attr = true
			`,
			expected: hcl(&Entry{
				Attribute: &Attribute{
					Key:      "attr",
					Value:    hbool(true),
					Comments: []string{"A comment"},
				},
			}),
		},
		{name: "Attributes",
			hcl: `
				true_bool = true
				false_bool = false
				str = "string"
				float = 1.234
				list = [1, 2, 3]
				map = {
					"a": 1,
					b: "str"
				}
			`,
			expected: &AST{
				Entries: []*Entry{
					attr("true_bool", hbool(true)),
					attr("false_bool", hbool(false)),
					attr("str", str("string")),
					attr("float", num(1.234)),
					attr("list", list(num(1), num(2), num(3))),
					attr("map", hmap(
						hkv("a", num(1)),
						hkv("b", str("str")),
					)),
				},
			},
		},
		{name: "Block",
			hcl: `
				block {
					str = "string"
				}
			`,
			expected: hcl(
				block("block", nil, attr("str", str("string"))),
			),
		},
		{name: "BlockWithLabels",
			hcl: `
				block label0 "label1" {}
			`,
			expected: hcl(
				block("block", []string{"label0", "label1"}),
			),
		},
		{name: "NestedBlocks",
			hcl: `
				block { nested {} }
			`,
			expected: hcl(block("block", nil, block("nested", nil))),
		},
		{name: "EmptyList",
			hcl:      `a = []`,
			expected: hcl(attr("a", list()))},
		{name: "TrailingComments",
			hcl: `
					a = true
					// trailing comment
				`,
			expected: trailingComments(hcl(attr("a", hbool(true))), "trailing comment")},
		{name: "SingleQuotedString",
			hcl: `
				a = 'hello\nworld'
				`,
			expected: hcl(attr("a", str("hello\nworld"))),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hcl, err := ParseString(test.hcl)
			if test.fail {
				assert.Error(t, err)
			} else if assert.NoError(t, err) {
				normaliseAST(hcl)
				assert.Equal(t,
					repr.String(test.expected, repr.Indent("  ")),
					repr.String(hcl, repr.Indent("  ")))
			}
		})
	}
}

func heredoc(delim, s string) *Value {
	return &Value{HeredocDelimiter: delim, Heredoc: &s}
}

func hbool(b bool) *Value {
	return &Value{Bool: (*Bool)(&b)}
}

func normaliseAST(hcl *AST) *AST {
	hcl.Pos = lexer.Position{}
	normaliseEntries(hcl.Entries)
	return hcl
}

func normaliseEntries(entries []*Entry) {
	for _, entry := range entries {
		entry.Parent = nil
		entry.Pos = lexer.Position{}
		if entry.Block != nil {
			entry.Block.Pos = lexer.Position{}
			entry.Block.Parent = nil
			normaliseEntries(entry.Block.Body)
		} else {
			entry.Attribute.Pos = lexer.Position{}
			entry.Attribute.Parent = nil
			val := entry.Attribute.Value
			normaliseValue(val)
		}
	}
}

func normaliseValue(val *Value) {
	val.Pos = lexer.Position{}
	val.Parent = nil
	for _, entry := range val.Map {
		entry.Pos = lexer.Position{}
		entry.Parent = nil
		normaliseValue(entry.Key)
		normaliseValue(entry.Value)
	}
	for _, entry := range val.List {
		normaliseValue(entry)
	}
}

func list(elements ...*Value) *Value {
	return &Value{List: elements, HaveList: true}
}

func hmap(kv ...*MapEntry) *Value {
	return &Value{Map: kv, HaveMap: true}
}

func hkv(k string, v *Value) *MapEntry {
	return &MapEntry{Key: &Value{Str: &k}, Value: v}
}

func hcl(entries ...*Entry) *AST {
	return &AST{Entries: entries}
}

func trailingComments(ast *AST, comments ...string) *AST {
	ast.TrailingComments = comments
	return ast
}

func block(name string, labels []string, entries ...*Entry) *Entry {
	return &Entry{Block: &Block{
		Name:   name,
		Labels: labels,
		Body:   entries,
	}}
}

func attr(k string, v *Value) *Entry {
	return &Entry{
		Attribute: &Attribute{Key: k, Value: v},
	}
}

func str(s string) *Value {
	return &Value{Str: &s}
}

func num(n float64) *Value {
	s := fmt.Sprintf("%g", n)
	b, _, _ := big.ParseFloat(s, 10, 64, 0)
	return &Value{Number: b}
}
