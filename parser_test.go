package hcl

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

func TestDetach(t *testing.T) {
	ast, err := ParseString(`
		one {}
		two {}
		three {}
	`)
	assert.NoError(t, err)
	ok := ast.Entries[1].Detach()
	assert.True(t, ok, "Could not detach %#v", ast.Entries[1])

	actual, err := MarshalAST(ast)
	assert.NoError(t, err)
	assert.Equal(t, `one {}

three {}
`, string(actual))
}

func TestGetHeredoc(t *testing.T) {
	value := &Heredoc{
		Delimiter: "-EOF",
		Doc:       "\n    hello\n  world",
	}
	assert.Equal(t, "  hello\nworld", value.GetHeredoc())
	value = &Heredoc{
		Delimiter: "EOF",
		Doc:       "\n  hello\n  world",
	}
	assert.Equal(t, "  hello\n  world", value.GetHeredoc())
}

func TestClone(t *testing.T) {
	ast, err := ParseString(complexHCLExample)
	assert.NoError(t, err)
	clone := ast.Clone()
	assert.Equal(t, ast, clone, assert.Exclude[Position]())
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
				Entries: []Entry{
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
				Entries: []Entry{
					attr("doc", heredoc("-EOF", "\n\tsome thing\n\tor another")),
				},
			},
		},
		{name: "EmptyHeredoc",
			hcl: `
				doc = <<EOF
EOF
			`,
			expected: &AST{
				Entries: []Entry{
					attr("doc", &Heredoc{Delimiter: "EOF"}),
				},
			},
		},
		{name: "Comments",
			hcl: `
				// A comment
				attr = true

				# Another comment
				attr2 = true
			`,
			expected: hcl(
				&Attribute{
					Key:      "attr",
					Value:    hbool(true),
					Comments: []string{"A comment"},
				},
				&Attribute{
					Key:      "attr2",
					Value:    hbool(true),
					Comments: []string{"Another comment"},
				},
			),
		},
		{name: "Comments with internal whitespace",
			hcl: `
				// Uncomment this to use it
				// block {
				//   env = {
				//     KEY: value
				//   } 
				// }
			`,
			expected: &AST{
				Entries: []Entry{
					&Comment{Comments: []string{
						"Uncomment this to use it",
						"block {",
						"  env = {",
						"    KEY: value",
						"  } ",
						"}",
					}},
				},
				TrailingComments: []string{
					"Uncomment this to use it",
					"block {",
					"  env = {",
					"    KEY: value",
					"  } ",
					"}",
				},
			},
		},
		{name: "Multiline comments with varying indentation",
			hcl: `
				block {
					//env = {
					//  KEY: value
					//}
				}
				block {
					//   env = {
					//     KEY: value
					//   }
				}
			`,
			expected: hcl(
				&Block{
					Name: "block",
					Body: []Entry{
						&Comment{
							Comments: []string{
								"env = {",
								"  KEY: value",
								"}",
							},
						},
					},
					TrailingComments: []string{
						"env = {",
						"  KEY: value",
						"}",
					},
				},
				&Block{
					Name: "block",
					Body: []Entry{
						&Comment{
							Comments: []string{
								"env = {",
								"  KEY: value",
								"}",
							},
						},
					},
					TrailingComments: []string{
						"env = {",
						"  KEY: value",
						"}",
					},
				},
			),
		},
		{name: "Attributes",
			hcl: `
				true_bool = true
				false_bool = false
				str = "string"
				int = 1
				negative_int = -1
				float = 1.234
				negative_float = -1.234
				list = [1, 2, 3]
				map = {
					"a": 1,
					b: "str"
				}
			`,
			expected: &AST{
				Entries: []Entry{
					attr("true_bool", hbool(true)),
					attr("false_bool", hbool(false)),
					attr("str", str("string")),
					attr("int", num(1)),
					attr("negative_int", num(-1)),
					attr("float", num(1.234)),
					attr("negative_float", num(-1.234)),
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
		{name: "BlockWithTrailingComments",
			hcl: `
					block {
					  attr = false

					  // trailing comment
					}
				`,
			expected: hcl(
				&Block{
					Name: "block",
					Body: []Entry{
						attr("attr", hbool(false)),
						&Comment{
							Comments: []string{"trailing comment"},
						},
					},
					TrailingComments: []string{"trailing comment"},
				},
			),
		},
		{name: "EmptyList",
			hcl:      `a = []`,
			expected: hcl(attr("a", list()))},
		{name: "TrailingComments",
			hcl: `
					a = true
					// trailing comment
				`,
			expected: &AST{
				Entries: []Entry{
					attr("a", hbool(true)),
					&Comment{Comments: []string{"trailing comment"}},
				},
				TrailingComments: []string{"trailing comment"},
			}},
		{name: "AttributeWithoutValue",
			hcl: `
				attr
				`,
			expected: hcl(attr("attr", nil))},
		{name: "Zero",
			hcl:      `num = 0`,
			expected: hcl(attr("num", num(0)))},
		{name: "DoubleQuotedString",
			hcl:      `str = "hello\nworld"`,
			expected: hcl(attr("str", str("hello\nworld")))},
		{name: "SingleQuotedString",
			hcl:      `a = 'hello\n"world"'`,
			expected: hcl(attr("a", str("hello\n\"world\"")))},
		{name: "BoolLiteralInMap",
			hcl: `
				map = {key: "true"}
			`,
			expected: &AST{
				Entries: []Entry{
					attr("map", hmap(hkv("key", str("true")))),
				},
			}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Determine if this test expects Comment nodes to be preserved
			expectsComments := false
			for _, entry := range test.expected.Entries {
				if _, isComment := entry.(*Comment); isComment {
					expectsComments = true
					break
				}
			}
			// Also check nested entries in blocks
			if !expectsComments {
				for _, entry := range test.expected.Entries {
					if block, isBlock := entry.(*Block); isBlock {
						for _, bodyEntry := range block.Body {
							if _, isComment := bodyEntry.(*Comment); isComment {
								expectsComments = true
								break
							}
						}
						if expectsComments {
							break
						}
					}
				}
			}

			var hcl *AST
			var err error
			if expectsComments {
				hcl, err = ParseString(test.hcl, WithDetachedComments(true))
			} else {
				hcl, err = ParseString(test.hcl)
			}

			if test.fail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				normaliseAST(hcl)
				assert.Equal(t, test.expected, hcl)
			}
		})
	}
}

func TestHeredocIndented(t *testing.T) {
	hcl, err := ParseString(`
	doc = <<-EOF
	some thing
	or another
	EOF
`)
	assert.NoError(t, err)
	expected := "some thing\nor another"
	assert.Equal(t, expected, hcl.Entries[0].(*Attribute).Value.(*Heredoc).GetHeredoc())
}

func heredoc(delim, s string) Value {
	return &Heredoc{Delimiter: delim, Doc: s}
}

func hbool(b bool) Value {
	return &Bool{Bool: b}
}

func normaliseAST(hcl *AST) {
	if hcl == nil {
		return
	}
	hcl.Pos = lexer.Position{}
	normaliseEntries(hcl.Entries)
}

func normaliseEntries(entries []Entry) {
	for _, entry := range entries {
		switch entry := entry.(type) {
		case *Comment:
			entry.Pos = lexer.Position{}
			entry.Parent = nil

		case *Block:
			entry.Pos = lexer.Position{}
			entry.Parent = nil
			normaliseEntries(entry.Body)

		case *Attribute:
			entry.Pos = lexer.Position{}
			entry.Parent = nil
			val := entry.Value
			normaliseValue(val)
			normaliseValue(entry.Default)
			for _, enum := range entry.Enum {
				normaliseValue(enum)
			}
		}
	}
}

func normaliseValue(val Value) {
	rv := reflect.ValueOf(val)
	if val == nil || rv.IsNil() {
		return
	}
	rv = reflect.Indirect(rv)
	rv.FieldByName("Pos").Set(reflect.ValueOf(lexer.Position{}))
	parent := rv.FieldByName("Parent")
	parent.Set(reflect.Zero(parent.Type()))
	switch val := val.(type) {
	case *Map:
		for _, entry := range val.Entries {
			entry.Pos = lexer.Position{}
			entry.Parent = nil
			normaliseValue(entry.Key)
			normaliseValue(entry.Value)
		}

	case *List:
		for _, entry := range val.List {
			normaliseValue(entry)
		}
	}
}

func list(elements ...Value) Value {
	return &List{List: elements}
}

func hmap(kv ...*MapEntry) Value {
	return &Map{Entries: kv}
}

func hkv(k string, v Value) *MapEntry {
	return &MapEntry{Key: &String{Str: k}, Value: v}
}

func hcl(entries ...Entry) *AST {
	return &AST{Entries: entries}
}

func block(name string, labels []string, entries ...Entry) Entry {
	return &Block{
		Name:   name,
		Labels: labels,
		Body:   entries,
	}
}

func attr(k string, v Value) Entry {
	return &Attribute{Key: k, Value: v}
}

func str(s string) Value {
	return &String{Str: s}
}

func num(n float64) Value {
	s := fmt.Sprintf("%g", n)
	b := &big.Float{}
	// b, _, _ := big.ParseFloat(s, 10, 64, 0)
	_, _, _ = b.Parse(s, 0)
	return &Number{Float: b}
}

func TestFunctionalOptions(t *testing.T) {
	hclContent := `
		// This is a comment
		attr = "value"
		
		block {
			// Another comment
			nested_attr = true
		}
		
		// Trailing comment
	`

	t.Run("DefaultBehavior", func(t *testing.T) {
		// Test default behavior (comments stripped)
		ast, err := ParseString(hclContent)
		assert.NoError(t, err)

		// Should have 2 entries: attribute and block (standalone comments stripped)
		assert.Equal(t, 2, len(ast.Entries))

		// First entry should be attribute with prefix comment
		attr, ok := ast.Entries[0].(*Attribute)
		assert.True(t, ok)
		assert.Equal(t, "attr", attr.Key)
		assert.Equal(t, []string{"This is a comment"}, attr.Comments)

		// Second entry should be block
		block, ok := ast.Entries[1].(*Block)
		assert.True(t, ok)
		assert.Equal(t, "block", block.Name)

		// Block should have 1 entry (nested attribute with prefix comment)
		assert.Equal(t, 1, len(block.Body))
		nestedAttr, ok := block.Body[0].(*Attribute)
		assert.True(t, ok)
		assert.Equal(t, "nested_attr", nestedAttr.Key)
		assert.Equal(t, []string{"Another comment"}, nestedAttr.Comments)

		// No standalone Comment entries should be present
		for _, entry := range ast.Entries {
			_, isComment := entry.(*Comment)
			assert.False(t, isComment, "Found unexpected standalone Comment entry")
		}
		for _, entry := range block.Body {
			_, isComment := entry.(*Comment)
			assert.False(t, isComment, "Found unexpected standalone Comment entry in block")
		}

		// Trailing comments should still be processed
		assert.Equal(t, []string{"Trailing comment"}, ast.TrailingComments)
	})

	t.Run("WithDetachedComments", func(t *testing.T) {
		// Test with detached comments enabled
		ast, err := ParseString(hclContent, WithDetachedComments(true))
		assert.NoError(t, err)

		// Should have 3 entries: attribute, block, and trailing comment
		assert.Equal(t, 3, len(ast.Entries))

		// First entry should be attribute with prefix comment
		attr, ok := ast.Entries[0].(*Attribute)
		assert.True(t, ok)
		assert.Equal(t, "attr", attr.Key)
		assert.Equal(t, []string{"This is a comment"}, attr.Comments)

		// Second entry should be block
		block, ok := ast.Entries[1].(*Block)
		assert.True(t, ok)
		assert.Equal(t, "block", block.Name)

		// Block should have 1 entry (nested attribute with prefix comment)
		assert.Equal(t, 1, len(block.Body))
		nestedAttr, ok := block.Body[0].(*Attribute)
		assert.True(t, ok)
		assert.Equal(t, "nested_attr", nestedAttr.Key)
		assert.Equal(t, []string{"Another comment"}, nestedAttr.Comments)

		// Third entry should be trailing comment
		comment, ok := ast.Entries[2].(*Comment)
		assert.True(t, ok)
		assert.Equal(t, []string{"Trailing comment"}, comment.Comments)

		// Trailing comments should still be processed
		assert.Equal(t, []string{"Trailing comment"}, ast.TrailingComments)
	})

	t.Run("WithDetachedCommentsFalse", func(t *testing.T) {
		// Test explicitly setting detached comments to false
		ast, err := ParseString(hclContent, WithDetachedComments(false))
		assert.NoError(t, err)

		// Should behave same as default (standalone comments stripped)
		assert.Equal(t, 2, len(ast.Entries))

		// No standalone Comment entries should be present
		for _, entry := range ast.Entries {
			_, isComment := entry.(*Comment)
			assert.False(t, isComment, "Found unexpected standalone Comment entry")
		}
	})
}
