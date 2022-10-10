package hcl

import (
	"strings"
	"testing"

	require "github.com/alecthomas/assert/v2"
)

type testSchema struct {
	Str   string         `hcl:"str" help:"A string field."`
	Num   int            `hcl:"num,optional"`
	Bool  bool           `hcl:"bool"`
	List  []string       `hcl:"list"`
	Map   map[string]int `hcl:"map" help:"A map."`
	Block struct {
		Name string  `hcl:"name,label"`
		Attr *string `hcl:"attr"`
	} `hcl:"block,block" help:"A block."`
	BlockSlice []struct {
		Label0 string `hcl:"label0,label"`
		Label1 string `hcl:"label1,label"`
		Attr   string `hcl:"attr"`
	} `hcl:"block_slice,block" help:"Repeated blocks."`
	DefaultStr string `hcl:"default_str" default:"def"`
	EnumStr    string `hcl:"enum_str" enum:"a,b,c"`
}

type keyValue struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

type objectRef struct {
	Name string `json:"name"`
}

type jsonTaggedSchema struct {
	Pos Position `json:"-"`

	Str     string      `json:"str"`
	Config  keyValue    `json:"config"`
	Options *keyValue   `json:"options,omitempty"`
	Refs    []objectRef `json:"refs,omitempty"`
}

const expectedSchema = `
// A string field.
str = string
num = number(optional)
bool = boolean
list = [string]
// A map.
map = {
  string: number,
}

// A block.
block "name" {
  attr = string
}

// Repeated blocks.
block_slice(repeated) "label0" "label1" {
  attr = string
}

default_str = string(optional default("def"))
enum_str = string(enum("a", "b", "c"))
`

func TestSchema(t *testing.T) {
	expected, err := Schema(&testSchema{})
	require.NoError(t, err)
	data, err := MarshalAST(expected)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(expectedSchema), strings.TrimSpace(string(data)))
	actual, err := ParseBytes(data)
	require.NoError(t, err)
	normaliseAST(actual)
	normaliseAST(expected)
	expected.Schema = false
	require.Equal(t, expected, actual)
}

func TestBlockSchema(t *testing.T) {
	type Block struct {
		Label string `hcl:"label,label"`
		Attr  string `hcl:"attr"`
	}
	schema, err := BlockSchema("block", &Block{})
	require.NoError(t, err)
	data, err := MarshalAST(schema)
	require.NoError(t, err)
	require.Equal(t,
		strings.TrimSpace(`
block "label" {
  attr = string
}
`),
		strings.TrimSpace(string(data)))
}

func TestJsonTaggedSchema(t *testing.T) {
	var val interface{}
	val = &jsonTaggedSchema{
		Str:     "testSchema",
		Config:  keyValue{"key1", "val1"},
		Options: &keyValue{},
		Refs:    []objectRef{{"ref11"}, {"ref12"}, {"ref13"}},
	}
	schema, err := Schema(val, InferHCLTags(true))
	require.NoError(t, err)
	data, err := MarshalAST(schema)
	require.NoError(t, err)
	expectedSchema := `
str = string

config {
  key = string
  value = string(optional)
}

options {
  key = string
  value = string(optional)
}

refs(repeated) {
  name = string
}
    `
	require.Equal(t, strings.TrimSpace(expectedSchema), strings.TrimSpace(string(data)))
}

type RecursiveSchema struct {
	Name      string           `hcl:"name" help:"Name of user."`
	Age       int              `hcl:"age,optional" help:"Age of user."`
	Recursive *RecursiveSchema `hcl:"recursive,block"`
}

func TestRecursiveSchema(t *testing.T) {
	ast, err := Schema(&RecursiveSchema{})
	require.NoError(t, err)
	schema, err := MarshalAST(ast)
	require.NoError(t, err)
	require.Equal(t, `// Name of user.
name = string
// Age of user.
age = number(optional)

recursive {
  // (recursive)
}
`, string(schema))
}
